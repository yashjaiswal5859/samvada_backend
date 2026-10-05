package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/services/tts"
)

type TTSService struct {
	httpClient *http.Client
}

func NewTTSService() *TTSService {
	return &TTSService{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Linear PCM to u-law compression algorithm for 8kHz telecom audio
func pcmToMulaw(pcm []int16) []byte {
	mulaw := make([]byte, len(pcm))
	for i, sample := range pcm {
		mulaw[i] = linearToMulaw(sample)
	}
	return mulaw
}

func linearToMulaw(pcm int16) byte {
	const bias = 132
	const clip = 32635

	sign := byte((pcm >> 8) & 0x80)
	if pcm < 0 {
		pcm = -pcm
	}
	if pcm > clip {
		pcm = clip
	}
	pcm = pcm + bias

	exponent := byte(7)
	for mask := int16(0x4000); (pcm&mask) == 0 && exponent > 0; mask >>= 1 {
		exponent--
	}
	mantissa := byte((pcm >> (exponent + 3)) & 0x0F)
	mulawByte := sign | (exponent << 4) | mantissa
	return ^mulawByte
}

func (t *TTSService) SynthesizeSpeechMulaw(ctx context.Context, text string) ([]byte, error) {
	return t.SynthesizeSpeechMulawWithSpeed(ctx, text, 0.85)
}

// GetPreGeneratedAudio checks for disk-cached pre-rendered audio matching provider-voice_id-tts_speed.
// On Cache HIT, returns pre-synthesized audio in 0ms. On Cache MISS, synthesizes once & stores to disk.
func (t *TTSService) GetPreGeneratedAudio(ctx context.Context, text string, speed float64) []byte {
	if t == nil || text == "" {
		return nil
	}
	provider := strings.ToLower(config.Settings.TtsProvider)
	if provider == "" {
		provider = "fishaudio"
	}
	voiceID := config.Settings.FishAudioVoice
	if provider == "cartesia" {
		voiceID = config.Settings.CartesiaVoice
	} else if provider == "openai" {
		voiceID = config.Settings.OpenaiVoice
	} else if provider == "murf" || provider == "murf_cloud" || provider == "murf_falcon2" || provider == "falcon2" || provider == "falcon-2" {
		voiceID = config.Settings.MurfVoice
	}
	if voiceID == "" {
		voiceID = "default"
	}

	cleanText := strings.TrimSpace(text)
	textSlug := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, cleanText)
	if len(textSlug) > 25 {
		textSlug = textSlug[:25]
	}

	fileName := fmt.Sprintf("%s-%s-%.2f-%s.mulaw", provider, voiceID, speed, textSlug)
	cacheDir := filepath.Join("audio", "pregenerated")
	_ = os.MkdirAll(cacheDir, 0755)
	cachePath := filepath.Join(cacheDir, fileName)

	// 1. Check Disk Cache (0ms Latency)
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		log.Printf("⚡⚡ [DISK PRE-GEN CACHE HIT] %s (%d bytes)", fileName, len(data))
		return data
	}

	// 2. Cache Miss: Synthesize & save to disk
	audio, err := t.SynthesizeSpeechMulawWithSpeed(ctx, text, speed)
	if err == nil && len(audio) > 0 {
		_ = os.WriteFile(cachePath, audio, 0644)
		log.Printf("💾 [DISK PRE-GEN CACHE SAVED] Synthesized & cached: %s (%d bytes)", fileName, len(audio))
		return audio
	}
	return nil
}

func (t *TTSService) SynthesizeSpeechStreamWithSpeed(ctx context.Context, text string, speed float64, onAudioChunk func([]byte)) ([]byte, error) {
	if speed <= 0 {
		speed = 0.9
	}
	provider := strings.ToLower(config.Settings.TtsProvider)
	voice := config.Settings.TtsVoice

	// 0b. FISH AUDIO CLOUD PROVIDER (PRIMARY WITH CHUNKED STREAMING)
	if provider == "fishaudio_cloud" || provider == "fish_cloud" || provider == "fishaudio" {
		fishVoice := config.Settings.FishAudioVoice
		if fishVoice == "" {
			fishVoice = voice
		}
		cloudTTS := tts.NewFishAudioCloudTTS()
		mulawBytes, err := cloudTTS.SynthesizeSpeechStreamWithSpeed(ctx, text, fishVoice, speed, onAudioChunk)
		if err == nil && len(mulawBytes) > 0 {
			log.Printf("[LEVEL 3: TTS ENGINE] Provider: FISHAUDIO_CLOUD STREAMING | Speed: %.2f | Text (%d chars) -> %d mulaw bytes", speed, len(text), len(mulawBytes))
			return mulawBytes, nil
		}
		log.Printf("[LEVEL 3: TTS ENGINE WARNING] FishAudio Cloud streaming failed (%v). Falling back to Cartesia TTS.", err)
	}

	// 0c. MURF AI FALCON-2 PROVIDER
	if provider == "murf" || provider == "murf_cloud" || provider == "murf_falcon2" || provider == "falcon2" || provider == "falcon-2" {
		murfVoice := config.Settings.MurfVoice
		if murfVoice == "" {
			murfVoice = voice
		}
		murfTTS := tts.NewMurfTTS()
		mulawBytes, err := murfTTS.SynthesizeSpeechStreamWithSpeed(ctx, text, murfVoice, speed, onAudioChunk)
		if err == nil && len(mulawBytes) > 0 {
			log.Printf("[LEVEL 3: TTS ENGINE] Provider: MURF_FALCON2 | Speed: %.2f | Text (%d chars) -> %d mulaw bytes", speed, len(text), len(mulawBytes))
			return mulawBytes, nil
		}
		log.Printf("[LEVEL 3: TTS ENGINE WARNING] Murf Falcon-2 failed (%v). Falling back to Cartesia/OpenAI TTS.", err)
	}

	// 1. FALLBACK TO CARTESIA PROVIDER
	mulawBytes, err := t.SynthesizeSpeechMulawWithSpeed(ctx, text, speed)
	if err == nil && len(mulawBytes) > 0 {
		if onAudioChunk != nil {
			onAudioChunk(mulawBytes)
		}
		return mulawBytes, nil
	}
	return nil, err
}

func (t *TTSService) SynthesizeSpeechMulawWithSpeed(ctx context.Context, text string, speed float64) ([]byte, error) {
	if speed <= 0 {
		speed = 0.9
	}
	provider := strings.ToLower(config.Settings.TtsProvider)
	voice := config.Settings.TtsVoice

	// 0. FISH AUDIO LOCAL PROVIDER (PRIMARY LOCAL OPEN SOURCE MODEL)
	if provider == "fishaudio_local" || provider == "fish_local" {
		localURL := config.Settings.FishAudioLocalUrl
		if localURL == "" {
			localURL = "http://localhost:8080/v1/tts"
		}
		fishVoice := config.Settings.FishAudioVoice
		if fishVoice == "" {
			fishVoice = voice
		}
		localTTS := tts.NewFishAudioLocalTTS()
		mulawBytes, err := localTTS.SynthesizeSpeechMulaw(ctx, text, fishVoice)
		if err == nil && len(mulawBytes) > 0 {
			log.Printf("[LEVEL 3: TTS ENGINE] Provider: FISHAUDIO_LOCAL | Endpoint: %s | Text (%d chars) -> %d mulaw bytes", localURL, len(text), len(mulawBytes))
			return mulawBytes, nil
		}
		log.Printf("[LEVEL 3: TTS ENGINE WARNING] FishAudio Local failed (%v). Falling back to Cartesia/OpenAI TTS.", err)
	}

	// 0b. FISH AUDIO CLOUD PROVIDER (PRIMARY CLOUD MODEL)
	if provider == "fishaudio_cloud" || provider == "fish_cloud" || provider == "fishaudio" {
		fishVoice := config.Settings.FishAudioVoice
		if fishVoice == "" {
			fishVoice = voice
		}
		cloudTTS := tts.NewFishAudioCloudTTS()
		mulawBytes, err := cloudTTS.SynthesizeSpeechMulawWithSpeed(ctx, text, fishVoice, speed)
		if err == nil && len(mulawBytes) > 0 {
			log.Printf("[LEVEL 3: TTS ENGINE] Provider: FISHAUDIO_CLOUD | Speed: %.2f | Text (%d chars) -> %d mulaw bytes", speed, len(text), len(mulawBytes))
			return mulawBytes, nil
		}
		log.Printf("[LEVEL 3: TTS ENGINE WARNING] FishAudio Cloud failed (%v). Falling back to Cartesia TTS.", err)
	}

	// 0c. MURF AI FALCON-2 PROVIDER
	if provider == "murf" || provider == "murf_cloud" || provider == "murf_falcon2" || provider == "falcon2" || provider == "falcon-2" {
		murfVoice := config.Settings.MurfVoice
		if murfVoice == "" {
			murfVoice = voice
		}
		murfTTS := tts.NewMurfTTS()
		mulawBytes, err := murfTTS.SynthesizeSpeechMulawWithSpeed(ctx, text, murfVoice, speed)
		if err == nil && len(mulawBytes) > 0 {
			log.Printf("[LEVEL 3: TTS ENGINE] Provider: MURF_FALCON2 | Speed: %.2f | Text (%d chars) -> %d mulaw bytes", speed, len(text), len(mulawBytes))
			return mulawBytes, nil
		}
		log.Printf("[LEVEL 3: TTS ENGINE WARNING] Murf Falcon-2 failed (%v). Falling back to Cartesia/OpenAI TTS.", err)
	}

	// 1. CARTESIA PROVIDER
	if provider == "cartesia" && config.Settings.CartesiaApiKey != "" {
		voiceID := config.Settings.CartesiaVoice
		if voiceID == "" {
			voiceID = voice
		}
		if voiceID == "" || voiceID == "default" || voiceID == "nova" {
			voiceID = "72656902-fb4b-4c31-af52-c3b68e2cae26" // Default Esha Indian Female Voice ID
		}

		ttsLang := config.Settings.TtsLanguage
		if ttsLang == "" {
			ttsLang = "hi"
		}

		payload := map[string]interface{}{
			"model_id":   "sonic-3.5",
			"transcript": text,
			"language":   ttsLang,
			"voice": map[string]interface{}{
				"mode": "id",
				"id":   voiceID,
			},
			"output_format": map[string]interface{}{
				"container":   "raw",
				"encoding":    "pcm_mulaw",
				"sample_rate": 8000,
			},
		}
		payloadBytes, _ := json.Marshal(payload)

		req, err := http.NewRequestWithContext(ctx, "POST", "https://api.cartesia.ai/tts/bytes", bytes.NewReader(payloadBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create Cartesia TTS request: %w", err)
		}
		req.Header.Set("X-API-Key", config.Settings.CartesiaApiKey)
		req.Header.Set("Cartesia-Version", "2024-06-10")
		req.Header.Set("Content-Type", "application/json")

		resp, err := t.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			mulawBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err == nil && len(mulawBytes) > 0 {
				log.Printf("[LEVEL 3: TTS ENGINE] Provider: CARTESIA | Model: sonic-3.5 | Voice ID: %s | Text (%d chars) -> %d mulaw bytes", voiceID, len(text), len(mulawBytes))
				return mulawBytes, nil
			}
		} else if resp != nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("[LEVEL 3: TTS ENGINE WARNING] Cartesia Status %d: %s. Falling back to OpenAI TTS.", resp.StatusCode, string(b))
		}
	}

	// 2. OPENAI PROVIDER (OR FALLBACK)
	apiKey := config.Settings.OpenaiApiKey
	openaiVoice := config.Settings.OpenaiVoice
	if openaiVoice == "" || strings.Contains(openaiVoice, "-") {
		openaiVoice = "nova" // Default OpenAI female voice if voice is empty or a UUID
	}

	if apiKey == "" {
		log.Printf("[TTS Service] Warning: OPENAI_API_KEY is empty. Generating mock audio.")
		return make([]byte, 1600), nil
	}

	payload := map[string]interface{}{
		"model":           "tts-1",
		"input":           text,
		"voice":           openaiVoice,
		"response_format": "pcm",
		"speed":           speed,
	}

	payloadBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/audio/speech", bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI TTS request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute OpenAI TTS request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI TTS API error (status %d): %s", resp.StatusCode, string(b))
	}

	rawPcm, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OpenAI TTS audio payload: %w", err)
	}

	// High-fidelity anti-aliasing downsampler: 24kHz PCM -> 8kHz PCM (Averages 3 consecutive samples)
	numSamples := len(rawPcm) / 2
	targetSamples := numSamples / 3
	pcmSamples := make([]int16, targetSamples)

	for i := 0; i < targetSamples; i++ {
		idx := i * 3 * 2
		if idx+5 < len(rawPcm) {
			s1 := int32(int16(binaryLittleEndianUint16(rawPcm[idx : idx+2])))
			s2 := int32(int16(binaryLittleEndianUint16(rawPcm[idx+2 : idx+4])))
			s3 := int32(int16(binaryLittleEndianUint16(rawPcm[idx+4 : idx+6])))
			pcmSamples[i] = int16((s1 + s2 + s3) / 3)
		}
	}

	mulawBytes := pcmToMulaw(pcmSamples)
	log.Printf("[LEVEL 3: TTS ENGINE] Provider: OPENAI | Model: tts-1 | Voice: %s | Text (%d chars) -> %d mulaw bytes", openaiVoice, len(text), len(mulawBytes))
	return mulawBytes, nil
}

func binaryLittleEndianUint16(b []byte) uint16 {
	return uint16(b[0]) | uint16(b[1])<<8
}

func (t *TTSService) SynthesizeBase64(ctx context.Context, text string) (string, error) {
	mulaw, err := t.SynthesizeSpeechMulaw(ctx, text)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(mulaw), nil
}

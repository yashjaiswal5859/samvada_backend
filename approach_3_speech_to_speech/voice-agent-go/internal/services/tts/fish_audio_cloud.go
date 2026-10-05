package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"voice-agent-go/internal/config"
)

// FishAudioCloudTTS implements TTSProvider for Fish Audio Cloud API (api.fish.audio)
type FishAudioCloudTTS struct {
	apiKey     string
	httpClient *http.Client
}

func NewFishAudioCloudTTS() *FishAudioCloudTTS {
	return &FishAudioCloudTTS{
		apiKey: config.Settings.FishAudioApiKey,
		httpClient: &http.Client{
			Timeout: 35 * time.Second, // 35s timeout for streaming long audio responses without premature deadline cancellation
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 30, // Optimized TCP Keep-Alive connection pool
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 5 * time.Second,
			},
		},
	}
}

func (f *FishAudioCloudTTS) ProviderName() string {
	return "fishaudio_cloud"
}

func (f *FishAudioCloudTTS) SynthesizeSpeechMulaw(ctx context.Context, text, voice string) ([]byte, error) {
	return f.SynthesizeSpeechMulawWithSpeed(ctx, text, voice, 0.85)
}

func (f *FishAudioCloudTTS) SynthesizeSpeechMulawWithSpeed(ctx context.Context, text, voice string, speed float64) ([]byte, error) {
	return f.SynthesizeSpeechStreamWithSpeed(ctx, text, voice, speed, nil)
}

func (f *FishAudioCloudTTS) SynthesizeSpeechStreamWithSpeed(ctx context.Context, text, voice string, speed float64, onAudioChunk func([]byte)) ([]byte, error) {
	if f.apiKey == "" {
		return nil, fmt.Errorf("FISH_AUDIO_API_KEY is not configured in .env")
	}
	if voice == "" {
		voice = "default"
	}
	if speed <= 0 {
		speed = 0.9
	}

	// Convert any emotion bracket tags anywhere in the sentence (e.g. [excited], [enthusiastic], [joyful]) into FishAudio format (e.g. (excited))
	cleanText := text
	for strings.Contains(cleanText, "[") && strings.Contains(cleanText, "]") {
		startIdx := strings.Index(cleanText, "[")
		endIdx := strings.Index(cleanText, "]")
		if startIdx >= 0 && endIdx > startIdx {
			tag := strings.ToLower(cleanText[startIdx+1 : endIdx])
			cleanText = cleanText[:startIdx] + "(" + tag + ") " + cleanText[endIdx+1:]
		} else {
			break
		}
	}
	if cleanText == "" {
		cleanText = text
	}

	// 1. Strip all newlines, carriage returns, and tabs to prevent raw '\n' pauses
	cleanText = strings.ReplaceAll(cleanText, "\n", " ")
	cleanText = strings.ReplaceAll(cleanText, "\r", " ")
	cleanText = strings.ReplaceAll(cleanText, "\t", " ")

	// 2. Automatically clean dotted degree & acronym abbreviations for seamless FishAudio pronunciation
	abbrevReplacements := map[string]string{
		"M.Tech": "MTech",
		"M.Sc":   "MSc",
		"M.E":    "ME",
		"B.Tech": "BTech",
		"B.Sc":   "BSc",
		"B.E":    "BE",
		"Ph.D":   "PhD",
		"Ph.D.":  "PhD",
		"S.O.P":  "SOP",
		"C.V":    "CV",
		"R.&.D":  "RND",
		"R&D":    "RND",
		"I.E.L.T.S": "IELTS",
		"G.R.E":     "GRE",
		"G.M.A.T":   "GMAT",
	}
	for oldAbbrev, newAbbrev := range abbrevReplacements {
		cleanText = strings.ReplaceAll(cleanText, oldAbbrev, newAbbrev)
	}

	for strings.Contains(cleanText, "  ") {
		cleanText = strings.ReplaceAll(cleanText, "  ", " ")
	}

	payload := map[string]interface{}{
		"text":        cleanText,
		"format":      "pcm",
		"sample_rate": 8000,
		"latency":     "low", // Force ultra-low latency cloud generation
		"speed":       speed, // Top-level speed conditioning for Fish Audio Cloud API v1
		"temperature": 0.75,  // Conversational sweet spot: high expressiveness with stable speaker embedding
		"top_p":       0.85,  // Balanced prosodic variation without decibel clipping
		"prosody": map[string]interface{}{
			"speed":              speed,
			"volume":             0,    // Standard baseline volume (0 dB)
			"normalize_loudness": true, // EBU R128 loudness normalization: keeps perceived volume 100% steady across all emotion tags!
		},
	}
	if voice != "" && voice != "default" {
		payload["reference_id"] = voice
	}
	payloadBytes, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return nil, marshalErr
	}

	start := time.Now()
	var resp *http.Response
	var httpErr error

	for attempt := 1; attempt <= 3; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, "POST", "https://api.fish.audio/v1/tts", bytes.NewReader(payloadBytes))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Authorization", "Bearer "+f.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("model", "s2.1-pro")

		resp, httpErr = f.httpClient.Do(req)
		if httpErr == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("[Fish Audio Cloud REST Warning] Attempt %d/3 returned HTTP %d: %s", attempt, resp.StatusCode, string(body))
		} else {
			log.Printf("[Fish Audio Cloud REST Warning] Attempt %d/3 network error: %v", attempt, httpErr)
		}

		if attempt < 3 {
			time.Sleep(300 * time.Millisecond)
		}
	}

	if httpErr != nil || resp == nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Fish Audio Cloud failed after 3 attempts: %v", httpErr)
	}
	defer resp.Body.Close()

	var allMulawBytes []byte
	var pcmBuf []byte
	readTmp := make([]byte, 2048)
	firstChunk := true

	for {
		n, readErr := resp.Body.Read(readTmp)
		if n > 0 {
			pcmBuf = append(pcmBuf, readTmp[:n]...)
			// Process complete 16-bit PCM sample pairs (2 bytes per sample)
			evenLen := (len(pcmBuf) / 2) * 2
			if evenLen > 0 {
				pcmChunk := pcmBuf[:evenLen]
				pcmBuf = pcmBuf[evenLen:]

				mulawChunk := ConvertPCMToMulaw(pcmChunk, 8000, 8000)
				if firstChunk {
					log.Printf("⚡⚡ [FishAudio REST Streaming] Time-To-First-Audio (TTFA): %v", time.Since(start))
					firstChunk = false
				}
				if onAudioChunk != nil {
					onAudioChunk(mulawChunk)
				}
				allMulawBytes = append(allMulawBytes, mulawChunk...)
			}
		}
		if readErr != nil {
			if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
				// Flush any remaining trailing sample pair
				if len(pcmBuf) >= 2 {
					evenLen := (len(pcmBuf) / 2) * 2
					pcmChunk := pcmBuf[:evenLen]
					mulawChunk := ConvertPCMToMulaw(pcmChunk, 8000, 8000)
					if onAudioChunk != nil {
						onAudioChunk(mulawChunk)
					}
					allMulawBytes = append(allMulawBytes, mulawChunk...)
				}
				break
			}
			return allMulawBytes, readErr
		}
	}

	log.Printf("⚡ [FishAudio REST SUCCESS] Speed: %.2f | TTFA: %v | Total: %v | Text (%d chars) -> %d mulaw bytes", speed, time.Since(start), time.Since(start), len(text), len(allMulawBytes))
	return allMulawBytes, nil
}

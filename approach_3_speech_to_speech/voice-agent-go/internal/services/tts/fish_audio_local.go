package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"voice-agent-go/internal/config"
)

// FishAudioLocalTTS implements TTSProvider for self-hosted local Fish Speech engine (Docker / SGLang / PyTorch server)
type FishAudioLocalTTS struct {
	localURL   string
	httpClient *http.Client
}

func NewFishAudioLocalTTS() *FishAudioLocalTTS {
	url := config.Settings.FishAudioLocalUrl
	if url == "" {
		url = "http://localhost:8080/v1/tts"
	}
	return &FishAudioLocalTTS{
		localURL: url,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (f *FishAudioLocalTTS) ProviderName() string {
	return "fishaudio_local"
}

func (f *FishAudioLocalTTS) SynthesizeSpeechMulaw(ctx context.Context, text, voice string) ([]byte, error) {
	if voice == "" {
		voice = "default"
	}

	payload := map[string]interface{}{
		"text":         text,
		"reference_id": voice,
		"format":       "pcm",
		"sample_rate":  22050,
		"streaming":    true,
		"language":     "hi",
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", f.localURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Fish Audio Local request failed at %s: %w", f.localURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Fish Audio Local API error (status %d): %s", resp.StatusCode, string(b))
	}

	pcmBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	mulawBytes := ConvertPCMToMulaw(pcmBytes, 22050, 8000)
	log.Printf("[TTS Engine] Provider: FishAudio Local | TTFA: %v | Text (%d chars) -> %d mulaw bytes", time.Since(start), len(text), len(mulawBytes))
	return mulawBytes, nil
}

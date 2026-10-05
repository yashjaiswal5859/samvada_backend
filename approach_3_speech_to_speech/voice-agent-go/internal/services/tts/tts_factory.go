package tts

import (
	"fmt"
	"strings"

	"voice-agent-go/internal/config"
)

// TTSFactory adheres to Factory Pattern and Open-Closed Principle (SOLID)
type TTSFactory struct{}

func NewTTSFactory() *TTSFactory {
	return &TTSFactory{}
}

func (f *TTSFactory) CreateTTSProvider(providerName string) (TTSProvider, error) {
	p := strings.ToLower(strings.TrimSpace(providerName))
	if p == "" {
		p = strings.ToLower(config.Settings.TtsProvider)
	}

	switch p {
	case "murf", "murf_cloud", "murf_falcon2", "falcon2", "falcon-2":
		return NewMurfTTS(), nil
	case "fishaudio_cloud", "fish_cloud", "fishaudio":
		return NewFishAudioCloudTTS(), nil
	case "fishaudio_local", "fish_local":
		return NewFishAudioLocalTTS(), nil
	default:
		return NewFishAudioCloudTTS(), fmt.Errorf("unsupported TTS provider: %s", providerName)
	}
}

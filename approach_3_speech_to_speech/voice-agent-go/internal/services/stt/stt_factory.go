package stt

import (
	"log"
	"strings"

	"voice-agent-go/internal/config"
)

// CreateSTTProvider is the Factory function (SOLID: Open/Closed Principle)
// New STT providers can be added here without modifying existing implementations
func CreateSTTProvider() STTProvider {
	provider := strings.ToLower(config.Settings.SttProvider)

	switch provider {
	case "deepgram":
		log.Printf("[STT Factory] Initialized Deepgram Nova-2 STT (multi-language, server-side streaming)")
		return NewDeepgramSTT()

	case "sarvam", "sarvamai":
		log.Printf("[STT Factory] Initialized Sarvam AI Saaras STT (multilingual Indic, server-side streaming)")
		return NewSarvamSTT()

	case "chrome", "browser", "":
		log.Printf("[STT Factory] Initialized Chrome Web Speech API STT (browser-native)")
		return NewChromeSTT()

	default:
		log.Printf("[STT Factory] Unknown STT_PROVIDER=%q, falling back to Chrome Web Speech API", provider)
		return NewChromeSTT()
	}
}

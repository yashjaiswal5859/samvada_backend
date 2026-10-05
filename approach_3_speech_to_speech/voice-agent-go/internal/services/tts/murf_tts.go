package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"voice-agent-go/internal/config"

	"github.com/gorilla/websocket"
)

// Global mutex to serialize requests and respect Murf account concurrency limits
var murfGlobalMu sync.Mutex

// MurfTTS implements TTSProvider for Murf AI Falcon-2 real-time WebSocket streaming
type MurfTTS struct {
	apiKey       string
	model        string
	defaultVoice string
}

// NewMurfTTS creates a new Murf AI Falcon-2 TTS provider instance
func NewMurfTTS() *MurfTTS {
	model := config.Settings.MurfModel
	if model == "" {
		model = "falcon-2"
	}
	voice := config.Settings.MurfVoice
	if voice == "" {
		voice = "en-US-natalie"
	}
	return &MurfTTS{
		apiKey:       config.Settings.MurfApiKey,
		model:        model,
		defaultVoice: voice,
	}
}

func (m *MurfTTS) ProviderName() string {
	return "murf"
}

func (m *MurfTTS) SynthesizeSpeechMulaw(ctx context.Context, text, voice string) ([]byte, error) {
	return m.SynthesizeSpeechStreamWithSpeed(ctx, text, voice, 1.0, nil)
}

func (m *MurfTTS) SynthesizeSpeechMulawWithSpeed(ctx context.Context, text, voice string, speed float64) ([]byte, error) {
	return m.SynthesizeSpeechStreamWithSpeed(ctx, text, voice, speed, nil)
}

func (m *MurfTTS) SynthesizeSpeechStreamWithSpeed(ctx context.Context, text, voice string, speed float64, onAudioChunk func([]byte)) ([]byte, error) {
	if m.apiKey == "" {
		return nil, fmt.Errorf("MURF_API_KEY is empty in .env")
	}

	// Serialize requests to prevent HTTP 429 concurrency limit on Murf API
	murfGlobalMu.Lock()
	defer murfGlobalMu.Unlock()

	voiceID := voice
	if voiceID == "" || voiceID == "default" || voiceID == "nova" {
		voiceID = m.defaultVoice
	}

	modelName := m.model
	if modelName == "" || strings.EqualFold(modelName, "falcon-2") {
		modelName = "falcon"
	}

	wsURL := fmt.Sprintf("wss://global.api.murf.ai/v1/speech/stream-input?api-key=%s&model=%s&sample_rate=8000&channel_type=MONO&format=ULAW", m.apiKey, modelName)

	headers := http.Header{}
	headers.Set("api-key", m.apiKey)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 6 * time.Second

	conn, httpResp, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		statusStr := "none"
		if httpResp != nil {
			statusStr = httpResp.Status
		}
		return nil, fmt.Errorf("failed to connect to Murf Falcon-2 WebSocket (%s): %w", statusStr, err)
	}
	defer conn.Close()

	// Handshake payload for Falcon streaming (aligned 100% with official Murf WebSockets specification)
	payload := map[string]interface{}{
		"voice_config": map[string]interface{}{
			"voice_id":          voiceID,
			"voiceId":           voiceID,
			"locale":            "hi-IN",
			"multiNativeLocale": "hi-IN",
			"style":             "Conversational",
			"model":             modelName,
			"rate":              0,
			"pitch":             0,
			"variation":         1,
			"format":            "ULAW",
			"sample_rate":       8000,
			"sampleRate":        8000,
			"channel_type":      "MONO",
		},
		"text":          text,
		"end_of_stream": true,
		"end":           true,
	}

	payloadBytes, _ := json.Marshal(payload)
	log.Printf("🚀 [MURF FALCON-2 WS SEND] Connecting to %s | Voice: %s | Text: %q", wsURL, voiceID, text)

	if err := conn.WriteMessage(websocket.TextMessage, payloadBytes); err != nil {
		return nil, fmt.Errorf("failed to send text to Murf WS stream: %w", err)
	}

	var accumulatedAudio []byte

	// Stream audio response chunks in real-time (with 3s read timeout per frame)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var chunkData []byte

		if messageType == websocket.TextMessage {
			var resp map[string]interface{}
			if err := json.Unmarshal(message, &resp); err == nil {
				if errMsg, ok := resp["error_message"].(string); ok && errMsg != "" {
					log.Printf("❌ [MURF WS ERROR EVENT] %s", errMsg)
					return nil, fmt.Errorf("Murf WS error: %s", errMsg)
				}
				if b64, ok := resp["audio"].(string); ok && b64 != "" {
					dec, decErr := base64.StdEncoding.DecodeString(b64)
					if decErr == nil && len(dec) > 0 {
						chunkData = dec
					}
				} else if b64, ok := resp["encodedAudio"].(string); ok && b64 != "" {
					dec, decErr := base64.StdEncoding.DecodeString(b64)
					if decErr == nil && len(dec) > 0 {
						chunkData = dec
					}
				} else if b64, ok := resp["data"].(string); ok && b64 != "" {
					dec, decErr := base64.StdEncoding.DecodeString(b64)
					if decErr == nil && len(dec) > 0 {
						chunkData = dec
					}
				}

				eos := false
				if v, ok := resp["end_of_stream"].(bool); ok {
					eos = v
				} else if v, ok := resp["end_of_stream"].(string); ok {
					eos = (v == "true" || v == "1")
				} else if v, ok := resp["final"].(bool); ok {
					eos = v
				} else if v, ok := resp["is_final"].(bool); ok {
					eos = v
				} else if v, ok := resp["end"].(bool); ok {
					eos = v
				}

				if len(chunkData) > 0 {
					accumulatedAudio = append(accumulatedAudio, chunkData...)
					if onAudioChunk != nil {
						onAudioChunk(chunkData)
					}
				}

				if eos {
					break
				}
			}
		} else if messageType == websocket.BinaryMessage {
			chunkData = message
			if len(chunkData) > 0 {
				accumulatedAudio = append(accumulatedAudio, chunkData...)
				if onAudioChunk != nil {
					onAudioChunk(chunkData)
				}
			}
		}
	}

	if len(accumulatedAudio) == 0 {
		return nil, fmt.Errorf("Murf WebSocket stream returned 0 audio bytes")
	}

	log.Printf("🎙️⚡ [MURF FALCON-2 WS STREAM] Voice ID: %s | Text (%d chars) -> %d mulaw bytes", voiceID, len(text), len(accumulatedAudio))
	return accumulatedAudio, nil
}

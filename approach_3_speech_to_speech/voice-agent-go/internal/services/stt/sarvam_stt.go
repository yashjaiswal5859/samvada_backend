package stt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/db"

	"github.com/gorilla/websocket"
)


// SarvamSTT implements STTProvider using Sarvam AI Saaras v3/v4 STT API & WebSocket
type SarvamSTT struct {
	apiKey string
}

// NewSarvamSTT creates a new Sarvam AI STT provider
func NewSarvamSTT() *SarvamSTT {
	return &SarvamSTT{
		apiKey: config.Settings.SarvamApiKey,
	}
}

func (s *SarvamSTT) Type() string                   { return "sarvam" }
func (s *SarvamSTT) RequiresServerProcessing() bool { return true }

// SarvamSession represents an active Sarvam AI real-time WebSocket streaming session
type SarvamSession struct {
	sessionID   string
	conn        *websocket.Conn
	transcripts chan TranscriptResult
	done        chan struct{}
	mu          sync.Mutex
	closed      bool
	frameCount  int
}

// sarvamWSResponse matches Sarvam v3-realtime WS response format (per official AsyncAPI spec).
// Events: session.begin, transcript.partial, transcript.final, vad.speech_start, vad.speech_end,
//         config.updated, pong, session.end, error
type sarvamWSResponse struct {
	Event              string  `json:"event"`                         // e.g. "transcript.partial", "transcript.final", "session.begin", "error"
	RequestID          string  `json:"request_id,omitempty"`          // session.begin / session.end
	UtteranceIdx       int     `json:"utterance_idx,omitempty"`       // transcript.partial / transcript.final
	Text               string  `json:"text,omitempty"`                // transcript.partial / transcript.final
	Language           string  `json:"language,omitempty"`            // auto-detected language (language_code=auto)
	LanguageConfidence float64 `json:"language_confidence,omitempty"` // detection confidence
	StartS             float64 `json:"start_s,omitempty"`             // segment start (return_timestamps=true)
	EndS               float64 `json:"end_s,omitempty"`               // segment end (return_timestamps=true)
	// Error fields
	Code      string `json:"code,omitempty"`
	IsFatal   bool   `json:"is_fatal,omitempty"`
	Message   string `json:"message,omitempty"`
	StatusCode int   `json:"status_code,omitempty"`
}

func (s *SarvamSTT) NewSession(sessionID string) (STTSession, error) {
	if s.apiKey == "" {
		log.Printf("❌ [SARVAM STT CRITICAL] SARVAM_API_KEY is EMPTY! Check your .env file!")
		return nil, fmt.Errorf("SARVAM_API_KEY not configured in .env")
	}

	langCode := "auto" // Default to "auto" for adaptive Hindi + English auto-detection
	if config.Settings.SttLanguage != "" {
		sttLang := strings.ToLower(config.Settings.SttLanguage)
		if sttLang == "en" || sttLang == "english" {
			langCode = "en-IN"
		} else if sttLang == "hi" || sttLang == "hindi" {
			langCode = "hi-IN"
		} else if sttLang == "auto" {
			langCode = "auto"
		}
	}

	if db.RedisClient != nil && sessionID != "" {
		configKey := fmt.Sprintf("call_config:%s", sessionID)
		if refID, err := db.RedisClient.HGet(context.Background(), configKey, "preliminary_call_id").Result(); err == nil && refID != "" {
			configKey = fmt.Sprintf("call_config:%s", refID)
		}
		if val, err := db.RedisClient.HGet(context.Background(), configKey, "initial_language").Result(); err == nil && val != "" {
			valLower := strings.ToLower(val)
			if valLower == "english" || valLower == "en" {
				langCode = "en-IN"
			} else if valLower == "hindi" || valLower == "hi" {
				langCode = "hi-IN"
			} else if valLower == "auto" {
				langCode = "auto"
			}
		}
	}

	// Sarvam v3-realtime WS: mode=transcribe ensures native Hindi transcription (not translated to English).
	// mode=transcribe: Standard transcription in source language script (Hindi).
	// encoding=mulaw&sample_rate=8000&endpointing=vad&threshold=0.6&min_speech_duration_ms=300&silence_duration_ms=450
	sarvamWSURL := fmt.Sprintf(
		"wss://api.sarvam.ai/speech-to-text-realtime/ws?api-subscription-key=%s&model=saaras:v3-realtime&language_code=%s&mode=transcribe&encoding=mulaw&sample_rate=8000&endpointing=vad&threshold=0.6&min_speech_duration_ms=300&silence_duration_ms=450",
		s.apiKey,
		langCode,
	)

	log.Printf("[SARVAM STT DIAL] Connecting to Sarvam WS (lang=%s, API Key len=%d)", langCode, len(s.apiKey))

	headers := http.Header{}
	headers.Set("api-subscription-key", s.apiKey)

	conn, httpResp, err := websocket.DefaultDialer.Dial(sarvamWSURL, headers)
	if err != nil {
		statusStr := "none"
		if httpResp != nil {
			statusStr = httpResp.Status
		}
		log.Printf("❌ [SARVAM STT DIAL ERROR] Failed (HTTP %s): %v", statusStr, err)
		return nil, fmt.Errorf("failed to connect to Sarvam AI WebSocket: %w", err)
	}

	log.Printf("✅ [SARVAM STT CONNECTED] Session %s: WebSocket open (lang=%s)", sessionID, langCode)

	session := &SarvamSession{
		sessionID:   sessionID,
		conn:        conn,
		transcripts: make(chan TranscriptResult, 64),
		done:        make(chan struct{}),
	}

	go session.readTranscripts()
	return session, nil
}

// SendAudio sends raw 8kHz mulaw telephony audio to Sarvam's saaras:v3-realtime WS.
// Sarvam natively supports mulaw at 8kHz (set via encoding=mulaw&sample_rate=8000 query params).
// Audio is base64-encoded and sent as JSON {"event":"audio_input","audio":"<base64_mulaw>"}.
// Zero conversion overhead — just base64 encode and send.
func (sess *SarvamSession) SendAudio(audio []byte) error {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.closed {
		return fmt.Errorf("session closed")
	}

	sess.frameCount++

	if sess.frameCount%50 == 1 {
		log.Printf("📡 [SARVAM STT AUDIO] Chunk #%d: %d bytes (8kHz mulaw) → JSON audio_input → Sarvam WS", sess.frameCount, len(audio))
	}

	// Sarvam accepts mulaw natively (encoding=mulaw in WS URL). Just base64-encode and send.
	b64Audio := base64.StdEncoding.EncodeToString(audio)
	msg := map[string]interface{}{
		"event": "audio_input",
		"audio": b64Audio,
	}
	msgBytes, _ := json.Marshal(msg)
	err := sess.conn.WriteMessage(websocket.TextMessage, msgBytes)

	if err != nil {
		log.Printf("❌ [SARVAM STT WRITE ERROR] Session %s chunk #%d: %v", sess.sessionID, sess.frameCount, err)
	}
	return err
}

func (sess *SarvamSession) Transcripts() <-chan TranscriptResult {
	return sess.transcripts
}

func (sess *SarvamSession) Close() error {
	sess.mu.Lock()
	if sess.closed {
		sess.mu.Unlock()
		return nil
	}
	sess.closed = true
	log.Printf("[SARVAM STT CLOSE] Session %s: Closing WebSocket (chunks sent: %d)", sess.sessionID, sess.frameCount)
	err := sess.conn.Close()
	close(sess.transcripts)
	sess.mu.Unlock()
	return err
}

func (sess *SarvamSession) readTranscripts() {
	defer func() {
		close(sess.done)
		sess.mu.Lock()
		if !sess.closed {
			sess.closed = true
			close(sess.transcripts)
		}
		sess.mu.Unlock()
	}()

	for {
		_, message, err := sess.conn.ReadMessage()
		if err != nil {
			sess.mu.Lock()
			wasClosed := sess.closed
			sess.mu.Unlock()
			if !wasClosed {
				log.Printf("❌ [SARVAM STT READ ERROR] Session %s: %v", sess.sessionID, err)
			}
			return
		}

		log.Printf("📩 [SARVAM STT RAW WS] Session %s: %s", sess.sessionID, string(message))

		var resp sarvamWSResponse
		if err := json.Unmarshal(message, &resp); err != nil {
			log.Printf("⚠️ [SARVAM STT PARSE ERROR] Session %s: %v", sess.sessionID, err)
			continue
		}

		switch resp.Event {
		case "session.begin":
			log.Printf("📋 [SARVAM STT SESSION BEGIN] Session %s: request_id=%s", sess.sessionID, resp.RequestID)

		case "session.end":
			log.Printf("📋 [SARVAM STT SESSION END] Session %s: request_id=%s", sess.sessionID, resp.RequestID)

		case "vad.speech_start":
			log.Printf("🎤 [SARVAM STT VAD] Session %s: Speech started (utterance #%d)", sess.sessionID, resp.UtteranceIdx)

		case "vad.speech_end":
			log.Printf("🔇 [SARVAM STT VAD] Session %s: Speech ended (utterance #%d)", sess.sessionID, resp.UtteranceIdx)

		case "transcript.partial":
			if resp.Text != "" {
				log.Printf("🎙️ [SARVAM STT PARTIAL] Session %s: [utt=#%d] %q", sess.sessionID, resp.UtteranceIdx, resp.Text)
				// Don't emit partials to the turn handler — wait for final
			}

		case "transcript.final":
			if resp.Text != "" {
				conf := resp.LanguageConfidence
				if conf <= 0 {
					conf = 0.95
				}
				log.Printf("🎙️ [SARVAM STT FINAL] Session %s: [utt=#%d, lang=%s, conf=%.2f] %q", sess.sessionID, resp.UtteranceIdx, resp.Language, conf, resp.Text)
				sess.transcripts <- TranscriptResult{
					Text:       resp.Text,
					IsFinal:    true,
					Confidence: conf,
				}
			}

		case "config.updated":
			log.Printf("📋 [SARVAM STT CONFIG UPDATED] Session %s", sess.sessionID)

		case "pong":
			// Keepalive response, ignore

		case "error":
			log.Printf("❌ [SARVAM STT ERROR] Session %s: [code=%s, fatal=%v, status=%d] %s", sess.sessionID, resp.Code, resp.IsFatal, resp.StatusCode, resp.Message)

		default:
			log.Printf("⚠️ [SARVAM STT UNKNOWN EVENT] Session %s: event=%s", sess.sessionID, resp.Event)
		}
	}
}

// TranscribeAudioPayload sends audio to Sarvam AI REST API.
// IMPORTANT: audioBytes coming from call_handler.go is ALREADY a valid 8kHz PCM WAV file
// (converted by convertMulawToWavBytes). Sarvam supports WAV natively — just send it directly!
// DO NOT re-decode WAV bytes as mulaw, that corrupts the audio completely.
func (s *SarvamSTT) TranscribeAudioPayload(audioBytes []byte) (string, error) {
	if s.apiKey == "" {
		log.Printf("❌ [SARVAM STT REST] SARVAM_API_KEY is empty!")
		return "", fmt.Errorf("SARVAM_API_KEY not configured in .env")
	}

	// audioBytes is already a valid 8kHz 16-bit PCM WAV file from convertMulawToWavBytes().
	// Sarvam REST API accepts WAV, MP3, OGG, FLAC etc. natively with auto-codec detection.
	// Just send it directly — NO further conversion needed!
	log.Printf("🚀 [SARVAM STT REST] Sending %d WAV bytes to POST https://api.sarvam.ai/speech-to-text", len(audioBytes))

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Attach the WAV file directly
	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", fmt.Errorf("failed to create form file for Sarvam STT: %w", err)
	}
	if _, err := part.Write(audioBytes); err != nil {
		return "", fmt.Errorf("failed to write audio bytes to Sarvam form: %w", err)
	}

	// Model & mode parameters
	_ = writer.WriteField("model", "saaras:v3")
	_ = writer.WriteField("mode", "codemix")
	_ = writer.WriteField("language_code", "hi-IN")
	_ = writer.Close()

	apiURL := "https://api.sarvam.ai/speech-to-text"
	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return "", fmt.Errorf("failed to create Sarvam STT HTTP request: %w", err)
	}

	req.Header.Set("api-subscription-key", s.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ [SARVAM STT REST HTTP ERROR] %v", err)
		return "", fmt.Errorf("failed to call Sarvam STT API: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	log.Printf("📩 [SARVAM STT REST RESPONSE] Status %d: %s", resp.StatusCode, string(respBytes))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Sarvam STT API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Transcript string `json:"transcript"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("failed to parse Sarvam STT JSON response: %w", err)
	}

	log.Printf("✅ [SARVAM STT REST SUCCESS] Transcribed → %q", result.Transcript)
	return strings.TrimSpace(result.Transcript), nil
}

package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/db"

	"github.com/gorilla/websocket"
)

// DeepgramSTT implements STTProvider using Deepgram Nova-2 real-time streaming & REST API
type DeepgramSTT struct {
	apiKey string
}

// NewDeepgramSTT creates a new Deepgram STT provider
func NewDeepgramSTT() *DeepgramSTT {
	return &DeepgramSTT{
		apiKey: config.Settings.DeepgramApiKey,
	}
}

func (d *DeepgramSTT) Type() string                   { return "deepgram" }
func (d *DeepgramSTT) RequiresServerProcessing() bool { return true }

// Keywords boosting parameters for Nova-3 (English) vs Nova-2 (Hindi)
const extractedRAGKeywordsEn = "keyterm=HDFC&keyterm=Swiggy&keyterm=Instamart&keyterm=Dineout&keyterm=Genie&keyterm=cashback&keyterm=credit%20card&keyterm=joining%20fee&keyterm=annual%20fee"
const extractedRAGKeywordsHi = "keywords=HDFC:2&keywords=Swiggy:2&keywords=Instamart:2&keywords=cashback:2&keywords=credit%20card:2&keywords=joining%20fee:2&keywords=annual%20fee:2"

func (d *DeepgramSTT) NewSession(sessionID string) (STTSession, error) {
	if d.apiKey == "" {
		return nil, fmt.Errorf("DEEPGRAM_API_KEY not configured in .env")
	}

	lang := config.Settings.SttLanguage
	if db.RedisClient != nil && sessionID != "" {
		configKey := fmt.Sprintf("call_config:%s", sessionID)
		if refID, err := db.RedisClient.HGet(context.Background(), configKey, "preliminary_call_id").Result(); err == nil && refID != "" {
			configKey = fmt.Sprintf("call_config:%s", refID)
		}
		if val, err := db.RedisClient.HGet(context.Background(), configKey, "initial_language").Result(); err == nil && val != "" {
			lang = strings.ToLower(val)
		}
	}

	// Use nova-2-general with Hindi/multilingual capability for Indian phone calls so both Hindi & English speech are transcribed accurately
	modelName := "nova-2-general"
	lang = "hi"
	kwParam := extractedRAGKeywordsHi

	// Deepgram streaming URL — nova-2-general for Hindi (using keywords) or nova-3 for English (using keyterm)
	dgURL := fmt.Sprintf(
		"wss://api.deepgram.com/v1/listen?model=%s&language=%s&smart_format=true&punctuate=true&interim_results=true&endpointing=200&vad_events=true&encoding=mulaw&sample_rate=8000&channels=1&%s",
		modelName,
		lang,
		kwParam,
	)

	// Connect to Deepgram with auth header
	headers := http.Header{}
	headers.Set("Authorization", "Token "+d.apiKey)

	conn, _, err := websocket.DefaultDialer.Dial(dgURL, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Deepgram WebSocket: %w", err)
	}

	log.Printf("[STT Deepgram] Session %s: Connected to Deepgram %s (lang=%s, keywords boosted)", sessionID, modelName, lang)

	session := &DeepgramSession{
		sessionID:   sessionID,
		conn:        conn,
		transcripts: make(chan TranscriptResult, 64),
		done:        make(chan struct{}),
	}

	// Start background goroutine to read Deepgram transcript responses
	go session.readTranscripts()

	// Start background KeepAlive ticker (sends KeepAlive ping every 3s so Deepgram WS never drops on idle)
	go session.startKeepAlive()

	return session, nil
}

func (s *DeepgramSession) startKeepAlive() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.closed || s.conn == nil {
				s.mu.Unlock()
				return
			}
			keepAliveMsg := map[string]string{"type": "KeepAlive"}
			msgBytes, _ := json.Marshal(keepAliveMsg)
			_ = s.conn.WriteMessage(websocket.TextMessage, msgBytes)
			s.mu.Unlock()
		}
	}
}

// DeepgramSession represents an active Deepgram streaming session
type DeepgramSession struct {
	sessionID   string
	conn        *websocket.Conn
	transcripts chan TranscriptResult
	done        chan struct{} // Signals that readTranscripts goroutine has exited
	mu          sync.Mutex
	closed      bool
}

// deepgramResponse represents the JSON response from Deepgram's streaming API
type deepgramResponse struct {
	Type    string `json:"type"`
	IsFinal bool   `json:"is_final"`
	Channel struct {
		Alternatives []struct {
			Transcript string  `json:"transcript"`
			Confidence float64 `json:"confidence"`
		} `json:"alternatives"`
	} `json:"channel"`
	SpeechFinal bool `json:"speech_final"`
}

func (s *DeepgramSession) SendAudio(audio []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("session closed")
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, audio)
}

func (s *DeepgramSession) Transcripts() <-chan TranscriptResult {
	return s.transcripts
}

// Close gracefully shuts down the Deepgram session
func (s *DeepgramSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// Step 1: Send CloseStream (tells Deepgram to process remaining audio and close)
	closeMsg := map[string]string{"type": "CloseStream"}
	closeMsgBytes, _ := json.Marshal(closeMsg)
	s.mu.Lock()
	_ = s.conn.WriteMessage(websocket.TextMessage, closeMsgBytes)
	s.mu.Unlock()

	log.Printf("[STT Deepgram] Session %s: Sent CloseStream, waiting for final transcripts...", s.sessionID)

	// Step 2: Wait for readTranscripts to finish (Deepgram closes its side after flushing)
	select {
	case <-s.done:
		log.Printf("[STT Deepgram] Session %s: Deepgram stream finished gracefully", s.sessionID)
	case <-time.After(3 * time.Second):
		log.Printf("[STT Deepgram] Session %s: Timeout waiting for Deepgram response, forcing close", s.sessionID)
	}

	// Step 3: Now actually close the WebSocket and channel
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	log.Printf("[STT Deepgram] Session %s: Closing WebSocket", s.sessionID)
	err := s.conn.Close()
	close(s.transcripts)
	return err
}

// readTranscripts reads Deepgram WebSocket responses and emits TranscriptResults
func (s *DeepgramSession) readTranscripts() {
	defer func() {
		close(s.done) // Signal Close() that we're finished
		s.mu.Lock()
		if !s.closed {
			s.closed = true
			close(s.transcripts)
		}
		s.mu.Unlock()
	}()

	for {
		_, message, err := s.conn.ReadMessage()
		if err != nil {
			s.mu.Lock()
			wasClosed := s.closed
			s.mu.Unlock()
			if !wasClosed {
				log.Printf("[STT Deepgram] Session %s: Read error: %v", s.sessionID, err)
			}
			return
		}

		log.Printf("[STT Deepgram RAW] Session %s: %s", s.sessionID, string(message))

		var resp deepgramResponse
		if err := json.Unmarshal(message, &resp); err != nil {
			continue
		}

		if resp.Type == "Error" {
			log.Printf("[STT Deepgram ERROR] Session %s: %s", s.sessionID, string(message))
			continue
		}

		if resp.Type != "Results" {
			continue
		}

		if len(resp.Channel.Alternatives) == 0 {
			continue
		}

		transcript := resp.Channel.Alternatives[0].Transcript
		if transcript == "" {
			continue
		}

		confidence := resp.Channel.Alternatives[0].Confidence
		isFinal := resp.SpeechFinal || resp.IsFinal

		log.Printf("[STT Deepgram] Session %s: [final=%v, conf=%.2f] %q", s.sessionID, isFinal, confidence, transcript)

		s.transcripts <- TranscriptResult{
			Text:       transcript,
			IsFinal:    isFinal,
			Confidence: confidence,
		}
	}
}

// TranscribeAudioPayload sends full recorded audio file bytes directly to Deepgram REST API (Approach A: Record & Send)
func (d *DeepgramSTT) TranscribeAudioPayload(audioBytes []byte) (string, error) {
	if d.apiKey == "" {
		return "", fmt.Errorf("DEEPGRAM_API_KEY not configured in .env")
	}

	lang := config.Settings.SttLanguage
	modelName := "nova-3"
	kwParam := extractedRAGKeywordsEn
	if lang == "" || lang == "multi" || lang == "hi" || lang == "hindi" {
		modelName = "nova-2-general"
		lang = "hi"
		kwParam = extractedRAGKeywordsHi
	}
	langParam := fmt.Sprintf("&language=%s", lang)

	// Deepgram REST API endpoint
	apiURL := fmt.Sprintf(
		"https://api.deepgram.com/v1/listen?model=%s%s&smart_format=true&punctuate=true&%s",
		modelName,
		langParam,
		kwParam,
	)

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(audioBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create Deepgram request: %w", err)
	}

	contentType := "audio/wav"
	if bytes.HasPrefix(audioBytes, []byte("RIFF")) {
		contentType = "audio/wav"
	} else if bytes.HasPrefix(audioBytes, []byte("\x1a\x45\xdf\xa3")) {
		contentType = "audio/webm"
	}

	req.Header.Set("Authorization", "Token "+d.apiKey)
	req.Header.Set("Content-Type", contentType)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call Deepgram API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Deepgram API error (status %d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		Results struct {
			Channels []struct {
				Alternatives []struct {
					Transcript string `json:"transcript"`
				} `json:"alternatives"`
			} `json:"channels"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("failed to parse Deepgram response: %w", err)
	}

	if len(res.Results.Channels) > 0 && len(res.Results.Channels[0].Alternatives) > 0 {
		return res.Results.Channels[0].Alternatives[0].Transcript, nil
	}

	return "", nil
}

package stt

// TranscriptResult represents a speech-to-text transcription result
type TranscriptResult struct {
	Text       string  `json:"text"`
	IsFinal    bool    `json:"is_final"`
	Confidence float64 `json:"confidence"`
}

// STTSession represents an active streaming STT session (e.g. one mic recording)
type STTSession interface {
	// SendAudio sends raw PCM audio bytes to the STT engine
	SendAudio(audio []byte) error

	// Transcripts returns a read-only channel that emits transcription results
	Transcripts() <-chan TranscriptResult

	// Close gracefully shuts down the STT session
	Close() error
}

// STTProvider is the abstract factory interface for Speech-to-Text engines (SOLID: Dependency Inversion)
// Implementations: DeepgramSTT (server-side streaming), ChromeSTT (browser-side, server no-op)
type STTProvider interface {
	// Type returns the provider name (e.g. "deepgram", "chrome")
	Type() string

	// RequiresServerProcessing returns true if audio must be relayed through Go server
	// Deepgram: true (server relays audio to Deepgram WS API)
	// Chrome: false (browser handles STT natively)
	RequiresServerProcessing() bool

	// NewSession creates a new streaming STT session for the given session ID
	// Returns nil session for providers that don't require server processing
	NewSession(sessionID string) (STTSession, error)

	// TranscribeAudioPayload transcribes a complete recorded audio payload (Approach A: Record & Send)
	TranscribeAudioPayload(audioBytes []byte) (string, error)
}

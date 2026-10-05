package stt

// ChromeSTT implements STTProvider using browser-native Web Speech API (webkitSpeechRecognition)
// Server-side is a no-op: all speech recognition happens in the browser's JavaScript runtime.
// The browser sends final transcript text directly as a regular chat message.
type ChromeSTT struct{}

// NewChromeSTT creates a new Chrome Web Speech API STT provider
func NewChromeSTT() *ChromeSTT {
	return &ChromeSTT{}
}

func (c *ChromeSTT) Type() string                    { return "chrome" }
func (c *ChromeSTT) RequiresServerProcessing() bool  { return false }

// NewSession is a no-op for Chrome STT since the browser handles speech recognition natively
func (c *ChromeSTT) NewSession(sessionID string) (STTSession, error) {
	return nil, nil
}

func (c *ChromeSTT) TranscribeAudioPayload(audioBytes []byte) (string, error) {
	return "", nil
}

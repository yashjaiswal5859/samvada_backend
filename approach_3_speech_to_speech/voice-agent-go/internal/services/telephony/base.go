package telephony

import (
	"context"
	"strings"
)

// CallStatus represents the status of an outbound/inbound call
type CallStatus string

const (
	StatusQueued     CallStatus = "queued"
	StatusInitiated  CallStatus = "initiated"
	StatusRinging    CallStatus = "ringing"
	StatusInProgress CallStatus = "in-progress"
	StatusCompleted  CallStatus = "completed"
	StatusFailed     CallStatus = "failed"
	StatusBusy       CallStatus = "busy"
	StatusNoAnswer   CallStatus = "no-answer"
)

// OutboundCallRequest contains parameters required to trigger an outbound call
type OutboundCallRequest struct {
	ToPhone       string            `json:"to_phone"`
	FromPhone     string            `json:"from_phone"`
	CallbackURL   string            `json:"callback_url"`
	StreamURL     string            `json:"stream_url"`
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// CallResponse contains standard unified response metadata returned by any telephony provider
type CallResponse struct {
	CallID      string            `json:"call_id"`
	Status      CallStatus        `json:"status"`
	Provider    string            `json:"provider"`
	RawResponse string            `json:"raw_response,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// FrameEventType represents the type of WebSocket streaming message frame
type FrameEventType string

const (
	EventStart FrameEventType = "start"
	EventMedia FrameEventType = "media"
	EventStop  FrameEventType = "stop"
	EventMark  FrameEventType = "mark"
	EventClear FrameEventType = "clear"
)

// AudioStreamFrame represents unified audio stream frame parsed from vendor WebSockets
type AudioStreamFrame struct {
	Event         FrameEventType `json:"event"`
	SessionID     string         `json:"session_id"`
	StreamSid     string         `json:"stream_sid,omitempty"`
	PayloadBase64 string         `json:"payload_base64,omitempty"` // Raw audio payload (mulaw 8khz or PCM)
	SampleRate    int            `json:"sample_rate"`
	Format        string         `json:"format"` // "mulaw", "pcm", etc.
	Track         string         `json:"track,omitempty"` // "inbound", "outbound"
}

// TelephonyProvider defines the abstract contract (interface) that all telephony providers must implement.
// Adheres to Open-Closed Principle (SOLID): new providers (e.g. Tata Communications, Airtel, Bandwidth) can be added without modifying existing caller logic.
type TelephonyProvider interface {
	// GetProviderName returns the identifier of the provider (e.g. "exotel", "twilio", "plivo", "telnyx")
	GetProviderName() string

	// MakeCall triggers an outbound call to a destination phone number
	MakeCall(ctx context.Context, req OutboundCallRequest) (*CallResponse, error)

	// HangupCall terminates an active call by ID
	HangupCall(ctx context.Context, callID string) error

	// GenerateConnectStreamPayload returns the vendor-specific XML/JSON markup (e.g. TwiML, Exoml, Plivo XML, TeXML)
	// instructing the provider's server to connect the audio call to a WebSocket stream endpoint.
	GenerateConnectStreamPayload(websocketURL string, streamName string) (string, error)

	// ParseStreamMessage converts vendor-specific raw WebSocket frames into a unified AudioStreamFrame
	ParseStreamMessage(rawData []byte) (*AudioStreamFrame, error)

	// BuildAudioOutboundPayload formats PCM/mulaw audio bytes back into vendor WebSocket wire payload format
	BuildAudioOutboundPayload(streamSid string, payloadBase64 string) ([]byte, error)
}

// NormalizeIndianPhone converts local Indian numbers (e.g. "09876543210" or "9876543210") to international E.164 format (+919876543210)
func NormalizeIndianPhone(phone string) string {
	cleaned := strings.TrimSpace(phone)
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")

	if strings.HasPrefix(cleaned, "+") {
		return cleaned
	}
	if strings.HasPrefix(cleaned, "0") && len(cleaned) == 11 {
		return "+91" + cleaned[1:]
	}
	if len(cleaned) == 10 {
		return "+91" + cleaned
	}
	return cleaned
}

package telephony

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"voice-agent-go/internal/config"
)

// ExotelProvider implements TelephonyProvider for Exotel (India's premier cloud telephony platform)
type ExotelProvider struct {
	accountSid  string
	apiKey      string
	apiToken    string
	subdomain   string
	fromPhone   string
	httpClient  *http.Client
}

func NewExotelProvider() *ExotelProvider {
	subdomain := config.Settings.ExotelSubdomain
	if subdomain == "" {
		subdomain = "api.exotel.com"
	}
	return &ExotelProvider{
		accountSid: config.Settings.ExotelAccountSid,
		apiKey:     config.Settings.ExotelApiKey,
		apiToken:   config.Settings.ExotelApiToken,
		subdomain:  subdomain,
		fromPhone:  config.Settings.ExotelPhoneNumber,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *ExotelProvider) GetProviderName() string {
	return "exotel"
}

func (e *ExotelProvider) MakeCall(ctx context.Context, req OutboundCallRequest) (*CallResponse, error) {
	if e.accountSid == "" || e.apiKey == "" || e.apiToken == "" {
		log.Printf("[Exotel Provider] Warning: Exotel credentials not configured. Returning mock outbound call.")
		callID := fmt.Sprintf("exotel_mock_%d", time.Now().UnixNano())
		return &CallResponse{
			CallID:   callID,
			Status:   StatusInitiated,
			Provider: e.GetProviderName(),
			Metadata: map[string]string{"note": "Mock Exotel Call triggered"},
		}, nil
	}

	toPhone := NormalizeIndianPhone(req.ToPhone)
	fromPhone := req.FromPhone
	if fromPhone == "" {
		fromPhone = e.fromPhone
	}

	apiURL := fmt.Sprintf("https://%s/v1/Accounts/%s/Calls/connect.json", e.subdomain, e.accountSid)

	formData := url.Values{}
	formData.Set("From", fromPhone)
	formData.Set("To", toPhone)
	formData.Set("Url", req.CallbackURL)
	formData.Set("CallType", "trans")

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create Exotel call request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.SetBasicAuth(e.apiKey, e.apiToken)

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to dispatch Exotel call: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("Exotel API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Call struct {
			Sid    string `json:"Sid"`
			Status string `json:"Status"`
		} `json:"Call"`
	}
	_ = json.Unmarshal(bodyBytes, &result)

	callID := result.Call.Sid
	if callID == "" {
		callID = fmt.Sprintf("exotel_%d", time.Now().UnixNano())
	}

	log.Printf("[Exotel Provider] Outbound call initiated successfully to %s (Sid: %s)", toPhone, callID)

	return &CallResponse{
		CallID:      callID,
		Status:      StatusInitiated,
		Provider:    e.GetProviderName(),
		RawResponse: string(bodyBytes),
	}, nil
}

func (e *ExotelProvider) HangupCall(ctx context.Context, callID string) error {
	if e.accountSid == "" || strings.HasPrefix(callID, "exotel_mock_") {
		log.Printf("[Exotel Provider] Hangup mock call: %s", callID)
		return nil
	}

	apiURL := fmt.Sprintf("https://%s/v1/Accounts/%s/Calls/%s.json", e.subdomain, e.accountSid, callID)
	formData := url.Values{}
	formData.Set("Status", "completed")

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.SetBasicAuth(e.apiKey, e.apiToken)

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (e *ExotelProvider) GenerateConnectStreamPayload(websocketURL string, streamName string) (string, error) {
	exoml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Connect>
        <Stream name="%s" url="%s" />
    </Connect>
</Response>`, streamName, websocketURL)
	return exoml, nil
}

// Exotel WebSocket Message Struct
type exotelWSMessage struct {
	Event     string `json:"event"`
	StreamSid string `json:"stream_sid"`
	CallSid   string `json:"call_sid"`
	Start     struct {
		CallSid   string `json:"call_sid"`
		StreamSid string `json:"stream_sid"`
	} `json:"start"`
	Media struct {
		Payload string `json:"payload"`
		Track   string `json:"track"`
	} `json:"media"`
}

func (e *ExotelProvider) ParseStreamMessage(rawData []byte) (*AudioStreamFrame, error) {
	var msg exotelWSMessage
	if err := json.Unmarshal(rawData, &msg); err != nil {
		return nil, fmt.Errorf("invalid Exotel WebSocket frame: %w", err)
	}

	frame := &AudioStreamFrame{
		SampleRate: 8000,
		Format:     "mulaw",
	}

	switch msg.Event {
	case "start":
		frame.Event = EventStart
		frame.SessionID = msg.Start.CallSid
		frame.StreamSid = msg.Start.StreamSid
	case "media":
		frame.Event = EventMedia
		frame.SessionID = msg.CallSid
		frame.StreamSid = msg.StreamSid
		frame.PayloadBase64 = msg.Media.Payload
		frame.Track = msg.Media.Track
	case "stop":
		frame.Event = EventStop
		frame.SessionID = msg.CallSid
		frame.StreamSid = msg.StreamSid
	default:
		frame.Event = EventMark
	}

	return frame, nil
}

func (e *ExotelProvider) BuildAudioOutboundPayload(streamSid string, payloadBase64 string) ([]byte, error) {
	payload := map[string]interface{}{
		"event":      "media",
		"stream_sid": streamSid,
		"media": map[string]string{
			"payload": payloadBase64,
		},
	}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(payload)
	return buf.Bytes(), err
}

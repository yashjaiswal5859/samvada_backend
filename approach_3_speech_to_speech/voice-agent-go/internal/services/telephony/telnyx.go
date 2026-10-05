package telephony

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"voice-agent-go/internal/config"
)

// TelnyxProvider implements TelephonyProvider for Telnyx TeXML
type TelnyxProvider struct {
	apiKey       string
	connectionId string
	fromPhone    string
	httpClient   *http.Client
}

func NewTelnyxProvider() *TelnyxProvider {
	return &TelnyxProvider{
		apiKey:       config.Settings.TelnyxApiKey,
		connectionId: config.Settings.TelnyxConnectionId,
		fromPhone:    config.Settings.TelnyxPhoneNumber,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *TelnyxProvider) GetProviderName() string {
	return "telnyx"
}

func (t *TelnyxProvider) MakeCall(ctx context.Context, req OutboundCallRequest) (*CallResponse, error) {
	if t.apiKey == "" {
		log.Printf("[Telnyx Provider] Warning: Telnyx API Key not configured. Returning mock outbound call.")
		callID := fmt.Sprintf("telnyx_mock_%d", time.Now().UnixNano())
		return &CallResponse{
			CallID:   callID,
			Status:   StatusInitiated,
			Provider: t.GetProviderName(),
			Metadata: map[string]string{"note": "Mock Telnyx Call triggered"},
		}, nil
	}

	toPhone := req.ToPhone
	fromPhone := req.FromPhone
	if fromPhone == "" {
		fromPhone = t.fromPhone
	}

	preliminaryCallID := req.Metadata["preliminary_call_id"]
	if preliminaryCallID == "" {
		preliminaryCallID = fmt.Sprintf("telnyx_%d", time.Now().UnixNano())
	}

	// Build public HTTPS answer URL with preliminary_call_id attached
	answerURL := req.CallbackURL
	if answerURL == "" {
		host := config.Settings.ServerPublicHost
		if host != "" && !strings.Contains(host, "localhost") && !strings.Contains(host, "127.0.0.1") {
			if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
				host = "https://" + host
			}
			answerURL = fmt.Sprintf("%s/v1/telephony/telnyx/answer?preliminary_call_id=%s", host, preliminaryCallID)
		}
	}

	// Telnyx TeXML Call API Endpoint
	var apiURL string
	if t.connectionId != "" {
		apiURL = fmt.Sprintf("https://api.telnyx.com/v2/texml/calls/%s", t.connectionId)
	} else {
		apiURL = "https://api.telnyx.com/v2/texml/Calls"
	}

	payload := map[string]interface{}{
		"To":   toPhone,
		"From": fromPhone,
		"Url":  answerURL,
	}

	payloadBytes, _ := json.Marshal(payload)
	log.Printf("[Telnyx TeXML Provider] Dispatching call to %s via API %s with Answer URL %s", toPhone, apiURL, answerURL)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create Telnyx call request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+t.apiKey)

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to dispatch Telnyx call: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("Telnyx API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	log.Printf("[Telnyx Provider] MakeCall response: %s", string(bodyBytes))

	var result struct {
		Sid        string `json:"sid"`
		CallSid    string `json:"call_sid"`
		CallLegID  string `json:"call_leg_id"`
		CallControl string `json:"call_control_id"`
	}
	_ = json.Unmarshal(bodyBytes, &result)

	callID := result.Sid
	if callID == "" {
		callID = result.CallSid
	}
	if callID == "" {
		callID = result.CallLegID
	}
	if callID == "" {
		callID = preliminaryCallID
	}

	log.Printf("[Telnyx Provider] Outbound call initiated to %s (CallID: %s)", toPhone, callID)

	return &CallResponse{
		CallID:      callID,
		Status:      StatusInitiated,
		Provider:    t.GetProviderName(),
		RawResponse: string(bodyBytes),
	}, nil
}

func (t *TelnyxProvider) HangupCall(ctx context.Context, callID string) error {
	if t.apiKey == "" || strings.HasPrefix(callID, "telnyx_mock_") {
		log.Printf("[Telnyx Provider] Hangup mock call: %s", callID)
		return nil
	}

	apiURL := fmt.Sprintf("https://api.telnyx.com/v2/texml/Calls/%s", callID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+t.apiKey)

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (t *TelnyxProvider) GenerateConnectStreamPayload(websocketURL string, streamName string) (string, error) {
	texml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Connect>
        <Stream name="%s" url="%s" />
    </Connect>
</Response>`, streamName, websocketURL)
	return texml, nil
}

// telnyxWSMessage handles Telnyx WebSocket media stream frames (TeXML stream)
type telnyxWSMessage struct {
	Event     string `json:"event"`
	StreamSid string `json:"streamSid"`
	StreamID  string `json:"stream_id"`
	Start     struct {
		CallSid   string `json:"callSid"`
		StreamSid string `json:"streamSid"`
	} `json:"start"`
	Media struct {
		Payload string      `json:"payload"`
		Track   string      `json:"track"`
		Chunk   interface{} `json:"chunk"`
	} `json:"media"`
}

func (t *TelnyxProvider) ParseStreamMessage(rawData []byte) (*AudioStreamFrame, error) {
	var msg telnyxWSMessage
	if err := json.Unmarshal(rawData, &msg); err != nil {
		return nil, fmt.Errorf("invalid Telnyx WebSocket frame: %w", err)
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
		if frame.StreamSid == "" {
			frame.StreamSid = msg.StreamSid
		}
		log.Printf("[Telnyx WS] Stream START — SessionID: %s, StreamSid: %s", frame.SessionID, frame.StreamSid)
	case "media":
		frame.Event = EventMedia
		frame.StreamSid = msg.StreamSid
		frame.PayloadBase64 = msg.Media.Payload
		frame.Track = msg.Media.Track
	case "stop":
		frame.Event = EventStop
		frame.StreamSid = msg.StreamSid
		log.Printf("[Telnyx WS] Stream STOP — StreamSid: %s", frame.StreamSid)
	default:
		frame.Event = EventMark
	}

	return frame, nil
}

func (t *TelnyxProvider) BuildAudioOutboundPayload(streamSid string, payloadBase64 string) ([]byte, error) {
	// Standard TeXML / TwiML WebSocket outbound media payload format
	payload := map[string]interface{}{
		"event":     "media",
		"streamSid": streamSid,
		"media": map[string]string{
			"payload": payloadBase64,
		},
	}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(payload)
	return buf.Bytes(), err
}

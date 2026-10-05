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
	"voice-agent-go/internal/db"
)

// PlivoProvider implements TelephonyProvider for Plivo
type PlivoProvider struct {
	authId     string
	authToken  string
	fromPhone  string
	httpClient *http.Client
}

func NewPlivoProvider() *PlivoProvider {
	return &PlivoProvider{
		authId:     config.Settings.PlivoAuthId,
		authToken:  config.Settings.PlivoAuthToken,
		fromPhone:  config.Settings.PlivoPhoneNumber,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *PlivoProvider) GetProviderName() string {
	return "plivo"
}

func (p *PlivoProvider) MakeCall(ctx context.Context, req OutboundCallRequest) (*CallResponse, error) {
	if p.authId == "" || p.authToken == "" {
		log.Printf("[Plivo Provider] Warning: Plivo credentials not configured. Returning mock outbound call.")
		callID := fmt.Sprintf("plivo_mock_%d", time.Now().UnixNano())
		return &CallResponse{
			CallID:   callID,
			Status:   StatusInitiated,
			Provider: p.GetProviderName(),
			Metadata: map[string]string{"note": "Mock Plivo Call triggered"},
		}, nil
	}

	toPhone := NormalizeIndianPhone(req.ToPhone)
	fromPhone := req.FromPhone
	if fromPhone == "" {
		fromPhone = p.fromPhone
	}

	apiURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Call/", p.authId)

	preliminaryCallID := req.Metadata["preliminary_call_id"]
	answerURL := req.CallbackURL
	if answerURL == "" {
		host := config.Settings.ServerPublicHost
		if host == "" || strings.Contains(host, "localhost") || strings.Contains(host, "127.0.0.1") {
			// Plivo requires a publicly resolvable HTTPS URL. If SERVER_PUBLIC_HOST is localhost, fallback to public endpoint for API validation.
			answerURL = "https://httpbin.org/post"
		} else {
			if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
				host = "https://" + host
			}
			answerURL = fmt.Sprintf("%s/v1/telephony/plivo/answer", host)
		}
	}

	if preliminaryCallID != "" {
		if strings.Contains(answerURL, "?") {
			answerURL = fmt.Sprintf("%s&preliminary_call_id=%s", answerURL, preliminaryCallID)
		} else {
			answerURL = fmt.Sprintf("%s?preliminary_call_id=%s", answerURL, preliminaryCallID)
		}
	}

	payload := map[string]string{
		"from":          fromPhone,
		"to":            toPhone,
		"answer_url":    answerURL,
		"answer_method": "POST",
	}
	payloadBytes, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create Plivo call request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.SetBasicAuth(p.authId, p.authToken)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to dispatch Plivo call: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("Plivo API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		RequestUUID string `json:"request_uuid"`
		Message     string `json:"message"`
	}
	_ = json.Unmarshal(bodyBytes, &result)

	log.Printf("[Plivo Provider] Outbound call initiated to %s (RequestUUID: %s)", toPhone, result.RequestUUID)

	if db.RedisClient != nil && preliminaryCallID != "" && result.RequestUUID != "" {
		configKey := fmt.Sprintf("call_config:%s", result.RequestUUID)
		_ = db.RedisClient.HSet(context.Background(), configKey, "preliminary_call_id", preliminaryCallID).Err()
		_ = db.RedisClient.Expire(context.Background(), configKey, 24*time.Hour).Err()
		log.Printf("🔗 [Plivo Provider] Pre-mapped Plivo RequestUUID %s -> preliminaryCallID %s in Redis", result.RequestUUID, preliminaryCallID)
	}

	return &CallResponse{
		CallID:      result.RequestUUID,
		Status:      StatusInitiated,
		Provider:    p.GetProviderName(),
		RawResponse: string(bodyBytes),
	}, nil
}

func (p *PlivoProvider) HangupCall(ctx context.Context, callID string) error {
	if p.authId == "" || strings.HasPrefix(callID, "plivo_mock_") {
		log.Printf("[Plivo Provider] Hangup mock call: %s", callID)
		return nil
	}

	apiURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Call/%s/", p.authId, callID)
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", apiURL, nil)
	if err != nil {
		return err
	}
	httpReq.SetBasicAuth(p.authId, p.authToken)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (p *PlivoProvider) GenerateConnectStreamPayload(websocketURL string, streamName string) (string, error) {
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Stream bidirectional="true" keepCallAlive="true" contentType="audio/x-mulaw;rate=8000">%s</Stream>
    <Wait length="1000"/>
</Response>`, websocketURL)
	return xml, nil
}

type plivoWSMessage struct {
	Event    string `json:"event"`
	StreamId string `json:"streamId"`
	CallUUID string `json:"callUUID"`
	Start    struct {
		StreamId string `json:"streamId"`
		CallUUID string `json:"callUUID"`
		CallID   string `json:"callId"`
	} `json:"start"`
	Media struct {
		Payload string `json:"payload"`
	} `json:"media"`
}

func (p *PlivoProvider) ParseStreamMessage(rawData []byte) (*AudioStreamFrame, error) {
	var msg plivoWSMessage
	if err := json.Unmarshal(rawData, &msg); err != nil {
		return nil, fmt.Errorf("invalid Plivo WebSocket frame: %w", err)
	}

	frame := &AudioStreamFrame{
		SampleRate: 8000,
		Format:     "mulaw",
	}

	sid := msg.StreamId
	if sid == "" {
		sid = msg.Start.StreamId
	}
	uuidStr := msg.CallUUID
	if uuidStr == "" {
		uuidStr = msg.Start.CallUUID
	}
	if uuidStr == "" {
		uuidStr = msg.Start.CallID
	}

	switch msg.Event {
	case "start", "media":
		if msg.Event == "start" {
			frame.Event = EventStart
		} else {
			frame.Event = EventMedia
		}
		frame.SessionID = uuidStr
		frame.StreamSid = sid
		frame.PayloadBase64 = msg.Media.Payload
	case "stop":
		frame.Event = EventStop
		frame.SessionID = uuidStr
		frame.StreamSid = sid
	default:
		frame.Event = EventMark
	}

	return frame, nil
}

func (p *PlivoProvider) BuildAudioOutboundPayload(streamSid string, payloadBase64 string) ([]byte, error) {
	payload := map[string]interface{}{
		"event":    "playAudio",
		"streamId": streamSid,
		"media": map[string]interface{}{
			"contentType": "audio/x-mulaw",
			"sampleRate":  8000,
			"payload":     payloadBase64,
		},
	}
	return json.Marshal(payload)
}

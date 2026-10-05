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

// TwilioProvider implements TelephonyProvider for Twilio
type TwilioProvider struct {
	accountSid string
	authToken  string
	fromPhone  string
	httpClient *http.Client
}

func NewTwilioProvider() *TwilioProvider {
	return &TwilioProvider{
		accountSid: config.Settings.TwilioAccountSid,
		authToken:  config.Settings.TwilioAuthToken,
		fromPhone:  config.Settings.TwilioPhoneNumber,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *TwilioProvider) GetProviderName() string {
	return "twilio"
}

func (t *TwilioProvider) MakeCall(ctx context.Context, req OutboundCallRequest) (*CallResponse, error) {
	if t.accountSid == "" || t.authToken == "" {
		log.Printf("[Twilio Provider] Warning: Twilio credentials not configured. Returning mock outbound call.")
		callID := fmt.Sprintf("twilio_mock_%d", time.Now().UnixNano())
		return &CallResponse{
			CallID:   callID,
			Status:   StatusInitiated,
			Provider: t.GetProviderName(),
			Metadata: map[string]string{"note": "Mock Twilio Call triggered"},
		}, nil
	}

	toPhone := req.ToPhone
	fromPhone := req.FromPhone
	if fromPhone == "" {
		fromPhone = t.fromPhone
	}

	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls.json", t.accountSid)

	answerURL := req.CallbackURL
	if answerURL == "" {
		host := config.Settings.ServerPublicHost
		if host == "" || strings.Contains(host, "localhost") || strings.Contains(host, "127.0.0.1") {
			answerURL = "https://demo.twilio.com/welcome/voice/"
		} else {
			if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
				host = "https://" + host
			}
			answerURL = fmt.Sprintf("%s/v1/telephony/twilio/answer", host)
		}
	}

	formData := url.Values{}
	formData.Set("To", toPhone)
	formData.Set("From", fromPhone)
	formData.Set("Url", answerURL)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create Twilio call request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.SetBasicAuth(t.accountSid, t.authToken)

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to dispatch Twilio call: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("Twilio API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Sid    string `json:"sid"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(bodyBytes, &result)

	log.Printf("[Twilio Provider] Outbound call initiated to %s (Sid: %s)", toPhone, result.Sid)

	return &CallResponse{
		CallID:      result.Sid,
		Status:      StatusInitiated,
		Provider:    t.GetProviderName(),
		RawResponse: string(bodyBytes),
	}, nil
}

func (t *TwilioProvider) HangupCall(ctx context.Context, callID string) error {
	if t.accountSid == "" || strings.HasPrefix(callID, "twilio_mock_") {
		log.Printf("[Twilio Provider] Hangup mock call: %s", callID)
		return nil
	}

	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls/%s.json", t.accountSid, callID)
	formData := url.Values{}
	formData.Set("Status", "completed")

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.SetBasicAuth(t.accountSid, t.authToken)

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (t *TwilioProvider) GenerateConnectStreamPayload(websocketURL string, streamName string) (string, error) {
	twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Connect>
        <Stream name="%s" url="%s" />
    </Connect>
</Response>`, streamName, websocketURL)
	return twiml, nil
}

type twilioWSMessage struct {
	Event     string `json:"event"`
	StreamSid string `json:"streamSid"`
	Start     struct {
		CallSid   string `json:"callSid"`
		StreamSid string `json:"streamSid"`
	} `json:"start"`
	Media struct {
		Payload string `json:"payload"`
		Track   string `json:"track"`
	} `json:"media"`
}

func (t *TwilioProvider) ParseStreamMessage(rawData []byte) (*AudioStreamFrame, error) {
	var msg twilioWSMessage
	if err := json.Unmarshal(rawData, &msg); err != nil {
		return nil, fmt.Errorf("invalid Twilio WebSocket frame: %w", err)
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
		frame.StreamSid = msg.StreamSid
		frame.PayloadBase64 = msg.Media.Payload
		frame.Track = msg.Media.Track
	case "stop":
		frame.Event = EventStop
		frame.StreamSid = msg.StreamSid
	default:
		frame.Event = EventMark
	}

	return frame, nil
}

func (t *TwilioProvider) BuildAudioOutboundPayload(streamSid string, payloadBase64 string) ([]byte, error) {
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

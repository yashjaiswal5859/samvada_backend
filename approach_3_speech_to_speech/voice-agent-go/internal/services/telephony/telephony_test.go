package telephony

import (
	"context"
	"strings"
	"testing"
)

func TestPhoneNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"9876543210", "+919876543210"},
		{"09876543210", "+919876543210"},
		{"+919876543210", "+919876543210"},
		{" 98765-43210 ", "+919876543210"},
	}

	for _, tt := range tests {
		result := NormalizeIndianPhone(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeIndianPhone(%q) = %q; want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTelephonyFactoryProviders(t *testing.T) {
	factory := GetFactory()

	providers := []string{"exotel", "twilio", "plivo", "telnyx"}
	for _, name := range providers {
		p, err := factory.GetProvider(name)
		if err != nil {
			t.Fatalf("Failed to get provider %q: %v", name, err)
		}
		if strings.ToLower(p.GetProviderName()) != name {
			t.Errorf("GetProviderName() = %q; want %q", p.GetProviderName(), name)
		}

		// Test Stream Payload Generation
		payload, err := p.GenerateConnectStreamPayload("wss://test.domain.com/v1/stream", "audio_stream")
		if err != nil {
			t.Fatalf("Failed generating connect stream payload for %s: %v", name, err)
		}
		if !strings.Contains(payload, "wss://test.domain.com/v1/stream") {
			t.Errorf("Stream payload for %s missing websocket URL: %s", name, payload)
		}

		// Test Outbound Call Request (Mock)
		resp, err := p.MakeCall(context.Background(), OutboundCallRequest{
			ToPhone:   "+919876543210",
			FromPhone: "+918047110000",
		})
		if err != nil {
			t.Fatalf("MakeCall failed for provider %s: %v", name, err)
		}
		if resp.CallID == "" {
			t.Errorf("MakeCall returned empty CallID for provider %s", name)
		}
	}
}

func TestTwilioStreamParsing(t *testing.T) {
	twilioProvider := NewTwilioProvider()
	jsonMsg := []byte(`{
		"event": "media",
		"streamSid": "MZ123456789",
		"media": {
			"payload": "AAAA////AAAA"
		}
	}`)

	frame, err := twilioProvider.ParseStreamMessage(jsonMsg)
	if err != nil {
		t.Fatalf("ParseStreamMessage failed for Twilio: %v", err)
	}
	if frame.Event != EventMedia {
		t.Errorf("Expected event %q, got %q", EventMedia, frame.Event)
	}
	if frame.PayloadBase64 != "AAAA////AAAA" {
		t.Errorf("Expected payload %q, got %q", "AAAA////AAAA", frame.PayloadBase64)
	}
}

func TestExotelStreamParsing(t *testing.T) {
	exotelProvider := NewExotelProvider()
	jsonMsg := []byte(`{
		"event": "media",
		"stream_sid": "exotel_stream_999",
		"call_sid": "exotel_call_111",
		"media": {
			"payload": "BBBB////BBBB"
		}
	}`)

	frame, err := exotelProvider.ParseStreamMessage(jsonMsg)
	if err != nil {
		t.Fatalf("ParseStreamMessage failed for Exotel: %v", err)
	}
	if frame.Event != EventMedia {
		t.Errorf("Expected event %q, got %q", EventMedia, frame.Event)
	}
	if frame.PayloadBase64 != "BBBB////BBBB" {
		t.Errorf("Expected payload %q, got %q", "BBBB////BBBB", frame.PayloadBase64)
	}
}

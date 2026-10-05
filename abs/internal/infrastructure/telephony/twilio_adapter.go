// =============================================================================
// Samvada Platform — ABS Twilio Telephony Adapter
// =============================================================================

package telephony

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type TwilioAdapter struct {
	accountSID string
	authToken  string
	httpClient *http.Client
	log        *zap.Logger
}

func NewTwilioAdapter(accountSID, authToken string, log *zap.Logger) CarrierAdapter {
	return &TwilioAdapter{
		accountSID: strings.TrimSpace(accountSID),
		authToken:  strings.TrimSpace(authToken),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		log:        log.Named("twilio_adapter"),
	}
}

func (a *TwilioAdapter) GetProviderName() string {
	return "twilio"
}

func (a *TwilioAdapter) isConfigured() bool {
	return a.accountSID != "" && a.authToken != "" && !strings.HasPrefix(a.accountSID, "mock")
}

func (a *TwilioAdapter) SearchAvailableNumbers(
	ctx context.Context,
	country, areaCode, numberType string,
	limit int,
) ([]AvailableNumber, error) {
	if country == "" {
		country = "US"
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	if !a.isConfigured() {
		// Realistic sandbox mock numbers for local development and integration tests
		results := make([]AvailableNumber, 0, limit)
		prefix := "+1415"
		if areaCode != "" {
			prefix = "+1" + areaCode
		}
		for i := 1; i <= limit; i++ {
			num := fmt.Sprintf("%s555%04d", prefix, 1000+i)
			cost := 1.15
			if numberType == "toll_free" {
				cost = 2.15
			}
			results = append(results, AvailableNumber{
				PhoneNumber:    num,
				FriendlyName:   fmt.Sprintf("(%s) 555-%04d", prefix[len(prefix)-3:], 1000+i),
				CountryCode:    country,
				Locality:       "San Francisco",
				Region:         "CA",
				Type:           numberType,
				MonthlyCostUSD: cost,
				Provider:       "twilio",
			})
		}
		return results, nil
	}

	endpointType := "Local"
	if strings.ToLower(numberType) == "toll_free" {
		endpointType = "TollFree"
	}

	apiURL := fmt.Sprintf(
		"https://api.twilio.com/2010-04-01/Accounts/%s/AvailablePhoneNumbers/%s/%s.json?PageSize=%d",
		a.accountSID, country, endpointType, limit,
	)
	if areaCode != "" {
		apiURL += fmt.Sprintf("&AreaCode=%s", areaCode)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create twilio request: %w", err)
	}
	req.SetBasicAuth(a.accountSID, a.authToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twilio api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("twilio search error %d: %s", resp.StatusCode, string(body))
	}

	var twilioResp struct {
		AvailablePhoneNumbers []struct {
			FriendlyName string `json:"friendly_name"`
			PhoneNumber  string `json:"phone_number"`
			Locality     string `json:"locality"`
			Region       string `json:"region"`
			IsoCountry   string `json:"iso_country"`
		} `json:"available_phone_numbers"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&twilioResp); err != nil {
		return nil, fmt.Errorf("decode twilio response: %w", err)
	}

	results := make([]AvailableNumber, 0, len(twilioResp.AvailablePhoneNumbers))
	for _, n := range twilioResp.AvailablePhoneNumbers {
		results = append(results, AvailableNumber{
			PhoneNumber:    n.PhoneNumber,
			FriendlyName:   n.FriendlyName,
			CountryCode:    n.IsoCountry,
			Locality:       n.Locality,
			Region:         n.Region,
			Type:           numberType,
			MonthlyCostUSD: 1.15,
			Provider:       "twilio",
		})
	}
	return results, nil
}

func (a *TwilioAdapter) PurchaseNumber(
	ctx context.Context,
	phoneNumber, webhookURL string,
) (*domain.PhoneNumber, error) {
	if !a.isConfigured() {
		sid := "PN" + uuid.New().String()[:32]
		return &domain.PhoneNumber{
			ID:             uuid.New(),
			PhoneNumber:    phoneNumber,
			Type:           "local",
			Provider:       "twilio",
			CarrierSID:     &sid,
			Status:         "active",
			MonthlyCostUSD: 1.15,
			CreatedAt:      time.Now().UTC(),
		}, nil
	}

	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/IncomingPhoneNumbers.json", a.accountSID)
	data := url.Values{}
	data.Set("PhoneNumber", phoneNumber)
	if webhookURL != "" {
		data.Set("VoiceUrl", webhookURL)
		data.Set("VoiceMethod", "POST")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create purchase request: %w", err)
	}
	req.SetBasicAuth(a.accountSID, a.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twilio purchase call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("twilio purchase error %d: %s", resp.StatusCode, string(body))
	}

	var twilioNum struct {
		Sid         string `json:"sid"`
		PhoneNumber string `json:"phone_number"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&twilioNum); err != nil {
		return nil, fmt.Errorf("decode twilio purchase response: %w", err)
	}

	return &domain.PhoneNumber{
		ID:             uuid.New(),
		PhoneNumber:    twilioNum.PhoneNumber,
		Type:           "local",
		Provider:       "twilio",
		CarrierSID:     &twilioNum.Sid,
		Status:         "active",
		MonthlyCostUSD: 1.15,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

func (a *TwilioAdapter) ReleaseNumber(ctx context.Context, carrierSID string) error {
	if !a.isConfigured() || carrierSID == "" {
		a.log.Info("Sandbox/mock DID release", zap.String("carrier_sid", carrierSID))
		return nil
	}

	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/IncomingPhoneNumbers/%s.json", a.accountSID, carrierSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}
	req.SetBasicAuth(a.accountSID, a.authToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("twilio release call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twilio release error %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (a *TwilioAdapter) ConfigureInboundWebhook(ctx context.Context, carrierSID, webhookURL string) error {
	if !a.isConfigured() || carrierSID == "" {
		a.log.Info("Sandbox/mock webhook configured", zap.String("carrier_sid", carrierSID), zap.String("webhook", webhookURL))
		return nil
	}

	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/IncomingPhoneNumbers/%s.json", a.accountSID, carrierSID)
	data := url.Values{}
	data.Set("VoiceUrl", webhookURL)
	data.Set("VoiceMethod", "POST")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create update webhook request: %w", err)
	}
	req.SetBasicAuth(a.accountSID, a.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("twilio update webhook call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twilio webhook config error %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

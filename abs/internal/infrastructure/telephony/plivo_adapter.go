// =============================================================================
// Samvada Platform — ABS Plivo Telephony Adapter
// =============================================================================

package telephony

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type PlivoAdapter struct {
	authID     string
	authToken  string
	httpClient *http.Client
	log        *zap.Logger
}

func NewPlivoAdapter(authID, authToken string, log *zap.Logger) CarrierAdapter {
	return &PlivoAdapter{
		authID:     strings.TrimSpace(authID),
		authToken:  strings.TrimSpace(authToken),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		log:        log.Named("plivo_adapter"),
	}
}

func (a *PlivoAdapter) GetProviderName() string {
	return "plivo"
}

func (a *PlivoAdapter) isConfigured() bool {
	return a.authID != "" && a.authToken != "" && !strings.HasPrefix(a.authID, "mock")
}

func (a *PlivoAdapter) SearchAvailableNumbers(
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
		results := make([]AvailableNumber, 0, limit)
		prefix := "+1212"
		if areaCode != "" {
			prefix = "+1" + areaCode
		}
		for i := 1; i <= limit; i++ {
			num := fmt.Sprintf("%s777%04d", prefix, 2000+i)
			cost := 0.85
			if numberType == "toll_free" {
				cost = 1.85
			}
			results = append(results, AvailableNumber{
				PhoneNumber:    num,
				FriendlyName:   fmt.Sprintf("(%s) 777-%04d", prefix[len(prefix)-3:], 2000+i),
				CountryCode:    country,
				Locality:       "New York",
				Region:         "NY",
				Type:           numberType,
				MonthlyCostUSD: cost,
				Provider:       "plivo",
			})
		}
		return results, nil
	}

	apiURL := fmt.Sprintf(
		"https://api.plivo.com/v1/Account/%s/PhoneNumber/?country_iso=%s&limit=%d",
		a.authID, country, limit,
	)
	if areaCode != "" {
		apiURL += fmt.Sprintf("&pattern=%s", areaCode)
	}
	if numberType != "" {
		apiURL += fmt.Sprintf("&type=%s", numberType)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create plivo request: %w", err)
	}
	req.SetBasicAuth(a.authID, a.authToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plivo api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("plivo search error %d: %s", resp.StatusCode, string(body))
	}

	var plivoResp struct {
		Objects []struct {
			Number      string `json:"number"`
			Prefix      string `json:"prefix"`
			Region      string `json:"region"`
			Country     string `json:"country"`
			MonthlyRate string `json:"monthly_rental_rate"`
			Type        string `json:"type"`
		} `json:"objects"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&plivoResp); err != nil {
		return nil, fmt.Errorf("decode plivo response: %w", err)
	}

	results := make([]AvailableNumber, 0, len(plivoResp.Objects))
	for _, n := range plivoResp.Objects {
		results = append(results, AvailableNumber{
			PhoneNumber:    "+" + n.Number,
			FriendlyName:   "+" + n.Number,
			CountryCode:    n.Country,
			Locality:       n.Region,
			Region:         n.Region,
			Type:           n.Type,
			MonthlyCostUSD: 0.85,
			Provider:       "plivo",
		})
	}
	return results, nil
}

func (a *PlivoAdapter) PurchaseNumber(
	ctx context.Context,
	phoneNumber, webhookURL string,
) (*domain.PhoneNumber, error) {
	cleanNum := strings.TrimPrefix(phoneNumber, "+")

	if !a.isConfigured() {
		sid := "PL" + uuid.New().String()[:32]
		return &domain.PhoneNumber{
			ID:             uuid.New(),
			PhoneNumber:    phoneNumber,
			Type:           "local",
			Provider:       "plivo",
			CarrierSID:     &sid,
			Status:         "active",
			MonthlyCostUSD: 0.85,
			CreatedAt:      time.Now().UTC(),
		}, nil
	}

	apiURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/PhoneNumber/%s/", a.authID, cleanNum)
	bodyData := map[string]string{}
	if webhookURL != "" {
		bodyData["app_id"] = webhookURL
	}
	payload, _ := json.Marshal(bodyData)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create purchase request: %w", err)
	}
	req.SetBasicAuth(a.authID, a.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plivo purchase call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("plivo purchase error %d: %s", resp.StatusCode, string(body))
	}

	sid := "PL" + cleanNum
	return &domain.PhoneNumber{
		ID:             uuid.New(),
		PhoneNumber:    phoneNumber,
		Type:           "local",
		Provider:       "plivo",
		CarrierSID:     &sid,
		Status:         "active",
		MonthlyCostUSD: 0.85,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

func (a *PlivoAdapter) ReleaseNumber(ctx context.Context, carrierSID string) error {
	cleanNum := strings.TrimPrefix(carrierSID, "PL")
	cleanNum = strings.TrimPrefix(cleanNum, "+")

	if !a.isConfigured() || carrierSID == "" {
		a.log.Info("Sandbox/mock Plivo number release", zap.String("carrier_sid", carrierSID))
		return nil
	}

	apiURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Number/%s/", a.authID, cleanNum)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}
	req.SetBasicAuth(a.authID, a.authToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("plivo release call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plivo release error %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (a *PlivoAdapter) ConfigureInboundWebhook(ctx context.Context, carrierSID, webhookURL string) error {
	cleanNum := strings.TrimPrefix(carrierSID, "PL")
	cleanNum = strings.TrimPrefix(cleanNum, "+")

	if !a.isConfigured() || carrierSID == "" {
		a.log.Info("Sandbox/mock Plivo webhook configured", zap.String("carrier_sid", carrierSID), zap.String("webhook", webhookURL))
		return nil
	}

	apiURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Number/%s/", a.authID, cleanNum)
	bodyData := map[string]string{
		"alias": "Samvada S2S DID",
	}
	payload, _ := json.Marshal(bodyData)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create update webhook request: %w", err)
	}
	req.SetBasicAuth(a.authID, a.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("plivo update webhook call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plivo webhook config error %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

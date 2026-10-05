// =============================================================================
// Samvada Platform — ABS Telephony Carrier Interface & Unified Types
// =============================================================================

package telephony

import (
	"context"

	"github.com/samvada/abs/internal/domain"
)

// AvailableNumber represents a purchasable DID returned by a carrier search.
type AvailableNumber struct {
	PhoneNumber    string  `json:"phone_number"`
	FriendlyName   string  `json:"friendly_name"`
	CountryCode    string  `json:"country_code"`
	Locality       string  `json:"locality"`
	Region         string  `json:"region"`
	Type           string  `json:"type"` // toll_free, local, mobile
	MonthlyCostUSD float64 `json:"monthly_cost_usd"`
	Provider       string  `json:"provider"`
}

// CarrierAdapter defines the unified carrier contract for searching, purchasing, and releasing DIDs.
type CarrierAdapter interface {
	GetProviderName() string
	SearchAvailableNumbers(ctx context.Context, country, areaCode, numberType string, limit int) ([]AvailableNumber, error)
	PurchaseNumber(ctx context.Context, phoneNumber, webhookURL string) (*domain.PhoneNumber, error)
	ReleaseNumber(ctx context.Context, carrierSID string) error
	ConfigureInboundWebhook(ctx context.Context, carrierSID, webhookURL string) error
}

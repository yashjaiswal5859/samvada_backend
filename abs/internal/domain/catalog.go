// =============================================================================
// Samvada Platform — ABS AI Catalog & Resource Registry Domain Entity
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type AIModel struct {
	ID            string                 `gorm:"type:varchar(100);primaryKey" json:"id"`
	Category      string                 `gorm:"type:varchar(50);not null;index:idx_ai_models_category" json:"category"` // stt, tts, llm
	Name          string                 `gorm:"type:varchar(255);not null" json:"name"`
	Provider      string                 `gorm:"type:varchar(100);not null" json:"provider"`
	Note          *string                `gorm:"type:text" json:"note,omitempty"`
	IsRecommended bool                   `gorm:"default:false" json:"is_recommended"`
	IsActive      bool                   `gorm:"default:true;index:idx_ai_models_active" json:"is_active"`
	Metadata      map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"metadata,omitempty"`
	CreatedAt     time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (AIModel) TableName() string {
	return "ai_models"
}

// PhoneNumberInput is the DTO used by admins to bulk upload numbers into the inventory
type PhoneNumberInput struct {
	PhoneNumber    string   `json:"phone_number" binding:"required"`
	Provider       string   `json:"provider" binding:"required"` // twilio, plivo, sip_trunk, telnyx, bandwidth
	Type           string   `json:"type"`                        // toll_free, local, mobile (default: local)
	MonthlyCostUSD *float64 `json:"monthly_cost_usd"`
	CarrierSID     *string  `json:"carrier_sid"`
	Country        *string  `json:"country"`
	Region         *string  `json:"region"`
}

// AIModelInput is the DTO used by admins to bulk upload models (STT, TTS, LLM)
type AIModelInput struct {
	ID            string                 `json:"id" binding:"required"`
	Category      string                 `json:"category"` // stt, tts, llm (inferred if hitting specific endpoint)
	Name          string                 `json:"name" binding:"required"`
	Provider      string                 `json:"provider" binding:"required"`
	Note          *string                `json:"note"`
	IsRecommended *bool                  `json:"is_recommended"`
	IsActive      *bool                  `json:"is_active"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// BulkUploadResult contains summaries of bulk upload operations
type BulkUploadResult struct {
	TotalProcessed int      `json:"total_processed"`
	Inserted       int      `json:"inserted"`
	Updated        int      `json:"updated"`
	Failed         int      `json:"failed"`
	Errors         []string `json:"errors,omitempty"`
}

// CatalogRepository defines data access methods for AI models and phone inventory
type CatalogRepository interface {
	UpsertAIModels(ctx context.Context, models []*AIModel) ([]*AIModel, error)
	ListAIModels(ctx context.Context, category string, activeOnly bool) ([]*AIModel, error)
	GetAIModel(ctx context.Context, id string) (*AIModel, error)
	DeleteAIModel(ctx context.Context, id string) error
	BulkUpsertPhoneNumbers(ctx context.Context, numbers []*PhoneNumber) (inserted int, updated int, err error)
	ListAvailablePhoneNumbers(ctx context.Context, search string, limit int) ([]*PhoneNumber, error)
	DeletePhoneNumber(ctx context.Context, id uuid.UUID) error
}

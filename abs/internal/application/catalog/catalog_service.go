// =============================================================================
// Samvada Platform — ABS AI Catalog & Phone Inventory Service
// =============================================================================

package catalog

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

var (
	validCategories = map[string]bool{"stt": true, "tts": true, "llm": true}
	validProviders  = map[string]bool{
		"twilio": true, "plivo": true, "sip_trunk": true, "telnyx": true, "bandwidth": true,
	}
	validNumberTypes = map[string]bool{"toll_free": true, "local": true, "mobile": true}
	phoneCleanRegex  = regexp.MustCompile(`[^\d+]`)
)

type CatalogService interface {
	BulkUploadPhoneNumbers(ctx context.Context, inputs []domain.PhoneNumberInput) (*domain.BulkUploadResult, error)
	BulkUploadModels(ctx context.Context, categoryOverride string, inputs []domain.AIModelInput) ([]*domain.AIModel, error)
	ListModelsGrouped(ctx context.Context, category string, activeOnly bool) (map[string][]*domain.AIModel, error)
	ListModelsFlat(ctx context.Context, category string, activeOnly bool) ([]*domain.AIModel, error)
	ListAvailablePhoneNumbers(ctx context.Context, search string, limit int) ([]*domain.PhoneNumber, error)
	DeleteModel(ctx context.Context, id string) error
	DeletePhoneNumber(ctx context.Context, id uuid.UUID) error
}

type catalogService struct {
	repo domain.CatalogRepository
	log  *zap.Logger
}

func NewCatalogService(repo domain.CatalogRepository, log *zap.Logger) CatalogService {
	return &catalogService{
		repo: repo,
		log:  log.Named("catalog_service"),
	}
}

func (s *catalogService) BulkUploadPhoneNumbers(ctx context.Context, inputs []domain.PhoneNumberInput) (*domain.BulkUploadResult, error) {
	if len(inputs) == 0 {
		return &domain.BulkUploadResult{}, nil
	}

	result := &domain.BulkUploadResult{
		TotalProcessed: len(inputs),
	}

	validNumbers := make([]*domain.PhoneNumber, 0, len(inputs))

	for idx, item := range inputs {
		cleanPhone := phoneCleanRegex.ReplaceAllString(strings.TrimSpace(item.PhoneNumber), "")
		if cleanPhone == "" {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: phone number is empty", idx+1))
			continue
		}

		// Ensure leading +
		if !strings.HasPrefix(cleanPhone, "+") {
			cleanPhone = "+" + cleanPhone
		}

		provider := strings.ToLower(strings.TrimSpace(item.Provider))
		if !validProviders[provider] {
			provider = "sip_trunk"
		}

		numType := strings.ToLower(strings.TrimSpace(item.Type))
		if !validNumberTypes[numType] {
			numType = "local"
		}

		monthlyCost := 1.00
		if item.MonthlyCostUSD != nil && *item.MonthlyCostUSD >= 0 {
			monthlyCost = *item.MonthlyCostUSD
		}

		pn := &domain.PhoneNumber{
			PhoneNumber:    cleanPhone,
			Provider:       provider,
			Type:           numType,
			CarrierSID:     item.CarrierSID,
			Status:         "available",
			MonthlyCostUSD: monthlyCost,
		}
		validNumbers = append(validNumbers, pn)
	}

	if len(validNumbers) == 0 {
		return result, fmt.Errorf("no valid phone numbers in payload")
	}

	inserted, updated, err := s.repo.BulkUpsertPhoneNumbers(ctx, validNumbers)
	if err != nil {
		s.log.Error("Failed to bulk upsert phone numbers", zap.Error(err))
		return nil, fmt.Errorf("database bulk upsert failed: %w", err)
	}

	result.Inserted = inserted
	result.Updated = updated

	s.log.Info("Bulk uploaded phone numbers successfully",
		zap.Int("total", result.TotalProcessed),
		zap.Int("inserted", inserted),
		zap.Int("updated", updated),
	)

	return result, nil
}

func (s *catalogService) BulkUploadModels(ctx context.Context, categoryOverride string, inputs []domain.AIModelInput) ([]*domain.AIModel, error) {
	if len(inputs) == 0 {
		return []*domain.AIModel{}, nil
	}

	override := strings.ToLower(strings.TrimSpace(categoryOverride))
	models := make([]*domain.AIModel, 0, len(inputs))

	for idx, item := range inputs {
		id := strings.ToLower(strings.TrimSpace(item.ID))
		if id == "" {
			return nil, fmt.Errorf("row %d: model id is required", idx+1)
		}

		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = id
		}

		category := override
		if category == "" {
			category = strings.ToLower(strings.TrimSpace(item.Category))
		}
		if !validCategories[category] {
			return nil, fmt.Errorf("row %d (id=%s): invalid category '%s' (must be stt, tts, or llm)", idx+1, id, category)
		}

		provider := strings.ToLower(strings.TrimSpace(item.Provider))
		if provider == "" {
			provider = "generic"
		}

		isRecommended := false
		if item.IsRecommended != nil {
			isRecommended = *item.IsRecommended
		}

		isActive := true
		if item.IsActive != nil {
			isActive = *item.IsActive
		}

		meta := item.Metadata
		if meta == nil {
			meta = make(map[string]interface{})
		}

		m := &domain.AIModel{
			ID:            id,
			Category:      category,
			Name:          name,
			Provider:      provider,
			Note:          item.Note,
			IsRecommended: isRecommended,
			IsActive:      isActive,
			Metadata:      meta,
		}
		models = append(models, m)
	}

	upserted, err := s.repo.UpsertAIModels(ctx, models)
	if err != nil {
		s.log.Error("Failed to upsert AI models", zap.Error(err))
		return nil, fmt.Errorf("database upsert failed: %w", err)
	}

	s.log.Info("Bulk uploaded AI models successfully", zap.Int("count", len(upserted)))
	return upserted, nil
}

func (s *catalogService) ListModelsGrouped(ctx context.Context, category string, activeOnly bool) (map[string][]*domain.AIModel, error) {
	list, err := s.repo.ListAIModels(ctx, category, activeOnly)
	if err != nil {
		return nil, err
	}

	grouped := map[string][]*domain.AIModel{
		"stt": {},
		"tts": {},
		"llm": {},
	}

	for _, m := range list {
		grouped[m.Category] = append(grouped[m.Category], m)
	}

	return grouped, nil
}

func (s *catalogService) ListModelsFlat(ctx context.Context, category string, activeOnly bool) ([]*domain.AIModel, error) {
	return s.repo.ListAIModels(ctx, category, activeOnly)
}

func (s *catalogService) ListAvailablePhoneNumbers(ctx context.Context, search string, limit int) ([]*domain.PhoneNumber, error) {
	return s.repo.ListAvailablePhoneNumbers(ctx, search, limit)
}

func (s *catalogService) DeleteModel(ctx context.Context, id string) error {
	return s.repo.DeleteAIModel(ctx, id)
}

func (s *catalogService) DeletePhoneNumber(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeletePhoneNumber(ctx, id)
}

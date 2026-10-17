// =============================================================================
// Samvada Platform — ABS AI Catalog & Phone Inventory Service Unit Tests
// =============================================================================

package catalog_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	appCatalog "github.com/samvada/abs/internal/application/catalog"
	"github.com/samvada/abs/internal/domain"
)

type mockCatalogRepo struct {
	mu      sync.Mutex
	models  map[string]*domain.AIModel
	numbers map[string]*domain.PhoneNumber
}

func newMockCatalogRepo() *mockCatalogRepo {
	return &mockCatalogRepo{
		models:  make(map[string]*domain.AIModel),
		numbers: make(map[string]*domain.PhoneNumber),
	}
}

func (m *mockCatalogRepo) UpsertAIModels(ctx context.Context, models []*domain.AIModel) ([]*domain.AIModel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, model := range models {
		m.models[model.ID] = model
	}
	return models, nil
}

func (m *mockCatalogRepo) ListAIModels(ctx context.Context, category string, activeOnly bool) ([]*domain.AIModel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*domain.AIModel
	cat := strings.ToLower(strings.TrimSpace(category))
	for _, model := range m.models {
		if cat != "" && model.Category != cat {
			continue
		}
		if activeOnly && !model.IsActive {
			continue
		}
		list = append(list, model)
	}
	return list, nil
}

func (m *mockCatalogRepo) GetAIModel(ctx context.Context, id string) (*domain.AIModel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	model, exists := m.models[id]
	if !exists {
		return nil, domain.ErrKBNotFound
	}
	return model, nil
}

func (m *mockCatalogRepo) DeleteAIModel(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.models, id)
	return nil
}

func (m *mockCatalogRepo) BulkUpsertPhoneNumbers(ctx context.Context, numbers []*domain.PhoneNumber) (inserted int, updated int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range numbers {
		if _, exists := m.numbers[n.PhoneNumber]; exists {
			updated++
		} else {
			inserted++
		}
		m.numbers[n.PhoneNumber] = n
	}
	return inserted, updated, nil
}

func (m *mockCatalogRepo) ListAvailablePhoneNumbers(ctx context.Context, search string, limit int) ([]*domain.PhoneNumber, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*domain.PhoneNumber
	for _, n := range m.numbers {
		if n.Status == "available" || n.Status == "unassigned" {
			list = append(list, n)
		}
	}
	return list, nil
}

func (m *mockCatalogRepo) DeletePhoneNumber(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.numbers {
		if v.ID == id {
			delete(m.numbers, k)
			break
		}
	}
	return nil
}

func TestCatalogService_BulkUploadPhoneNumbers(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockCatalogRepo()
	svc := appCatalog.NewCatalogService(repo, logger)
	ctx := context.Background()

	cost := 2.50
	carrierSID := "CR123"

	// 1. Bulk upload a list of phone numbers
	payload := []domain.PhoneNumberInput{
		{
			PhoneNumber:    "+1 (202) 555-0143",
			Provider:       "twilio",
			Type:           "local",
			MonthlyCostUSD: &cost,
			CarrierSID:     &carrierSID,
		},
		{
			PhoneNumber: "918045678901", // without +
			Provider:    "plivo",
			Type:        "toll_free",
		},
	}

	res, err := svc.BulkUploadPhoneNumbers(ctx, payload)
	require.NoError(t, err)
	assert.Equal(t, 2, res.TotalProcessed)
	assert.Equal(t, 2, res.Inserted)
	assert.Equal(t, 0, res.Updated)

	// 2. Verify normalization
	numbers, err := svc.ListAvailablePhoneNumbers(ctx, "", 10)
	require.NoError(t, err)
	assert.Len(t, numbers, 2)

	// Check "+12025550143" was cleaned
	foundClean1 := false
	foundClean2 := false
	for _, n := range numbers {
		if n.PhoneNumber == "+12025550143" {
			foundClean1 = true
			assert.Equal(t, "twilio", n.Provider)
			assert.Equal(t, "local", n.Type)
			assert.Equal(t, 2.50, n.MonthlyCostUSD)
		}
		if n.PhoneNumber == "+918045678901" {
			foundClean2 = true
			assert.Equal(t, "plivo", n.Provider)
			assert.Equal(t, "toll_free", n.Type)
		}
	}
	assert.True(t, foundClean1)
	assert.True(t, foundClean2)

	// 3. Repeated upload updates existing
	res2, err := svc.BulkUploadPhoneNumbers(ctx, payload)
	require.NoError(t, err)
	assert.Equal(t, 0, res2.Inserted)
	assert.Equal(t, 2, res2.Updated)
}

func TestCatalogService_BulkUploadModels(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockCatalogRepo()
	svc := appCatalog.NewCatalogService(repo, logger)
	ctx := context.Background()

	isRec := true
	note := "Best speech model"

	// 1. Bulk upload STT models
	sttPayload := []domain.AIModelInput{
		{
			ID:            "deepgram",
			Name:          "Deepgram Nova-2",
			Provider:      "deepgram",
			Note:          &note,
			IsRecommended: &isRec,
		},
		{
			ID:       "whisper",
			Name:     "Whisper Large",
			Provider: "openai",
		},
	}

	sttUpserted, err := svc.BulkUploadModels(ctx, "stt", sttPayload)
	require.NoError(t, err)
	assert.Len(t, sttUpserted, 2)
	assert.Equal(t, "stt", sttUpserted[0].Category)

	// 2. Bulk upload mixed models with inline categories (LLM and TTS)
	mixedPayload := []domain.AIModelInput{
		{
			ID:       "gpt-4o-mini",
			Category: "llm",
			Name:     "GPT-4o Mini",
			Provider: "openai",
		},
		{
			ID:       "fish-audio",
			Category: "tts",
			Name:     "Fish Audio",
			Provider: "fish-audio",
		},
	}

	mixedUpserted, err := svc.BulkUploadModels(ctx, "", mixedPayload)
	require.NoError(t, err)
	assert.Len(t, mixedUpserted, 2)

	// 3. Verify grouped retrieval
	grouped, err := svc.ListModelsGrouped(ctx, "", true)
	require.NoError(t, err)
	assert.Len(t, grouped["stt"], 2)
	assert.Len(t, grouped["llm"], 1)
	assert.Len(t, grouped["tts"], 1)

	// 4. Invalid category rejected
	invalidPayload := []domain.AIModelInput{
		{
			ID:       "unknown-model",
			Category: "invalid_category",
			Name:     "Test",
			Provider: "test",
		},
	}
	_, err = svc.BulkUploadModels(ctx, "", invalidPayload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid category")
}

// =============================================================================
// Samvada Platform — ABS AI Catalog & Phone Inventory Repository (GORM PostgreSQL)
// =============================================================================

package repos

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/samvada/abs/internal/domain"
)

type catalogRepo struct {
	db *gorm.DB
}

func NewCatalogRepository(db *gorm.DB) domain.CatalogRepository {
	return &catalogRepo{db: db}
}

func (r *catalogRepo) UpsertAIModels(ctx context.Context, models []*domain.AIModel) ([]*domain.AIModel, error) {
	if len(models) == 0 {
		return []*domain.AIModel{}, nil
	}

	now := time.Now().UTC()
	for _, m := range models {
		m.UpdatedAt = now
		if m.CreatedAt.IsZero() {
			m.CreatedAt = now
		}
	}

	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"category",
				"name",
				"provider",
				"note",
				"is_recommended",
				"is_active",
				"metadata",
				"updated_at",
			}),
		}).
		Create(&models).Error

	if err != nil {
		return nil, err
	}

	return models, nil
}

func (r *catalogRepo) ListAIModels(ctx context.Context, category string, activeOnly bool) ([]*domain.AIModel, error) {
	var models []*domain.AIModel
	q := r.db.WithContext(ctx)

	if strings.TrimSpace(category) != "" {
		q = q.Where("category = ?", strings.ToLower(strings.TrimSpace(category)))
	}

	if activeOnly {
		q = q.Where("is_active = ?", true)
	}

	q = q.Order("category ASC, is_recommended DESC, name ASC")
	err := q.Find(&models).Error
	return models, err
}

func (r *catalogRepo) GetAIModel(ctx context.Context, id string) (*domain.AIModel, error) {
	var m domain.AIModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *catalogRepo) DeleteAIModel(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&domain.AIModel{}, "id = ?", id).Error
}

func (r *catalogRepo) BulkUpsertPhoneNumbers(ctx context.Context, numbers []*domain.PhoneNumber) (inserted int, updated int, err error) {
	if len(numbers) == 0 {
		return 0, 0, nil
	}

	now := time.Now().UTC()
	for _, n := range numbers {
		if n.ID == uuid.Nil {
			n.ID = uuid.New()
		}
		if n.CreatedAt.IsZero() {
			n.CreatedAt = now
		}
		if n.Status == "" {
			n.Status = "available"
		}
	}

	// Count existing phone numbers before upsert to determine inserted vs updated
	var existingNumbers []string
	phoneStrs := make([]string, len(numbers))
	for i, n := range numbers {
		phoneStrs[i] = n.PhoneNumber
	}

	_ = r.db.WithContext(ctx).
		Model(&domain.PhoneNumber{}).
		Where("phone_number IN ?", phoneStrs).
		Pluck("phone_number", &existingNumbers).Error

	existingMap := make(map[string]bool)
	for _, p := range existingNumbers {
		existingMap[p] = true
	}

	// Idempotent upsert via ON CONFLICT (phone_number)
	err = r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "phone_number"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"type",
				"provider",
				"carrier_sid",
				"status",
				"monthly_cost_usd",
			}),
		}).
		Create(&numbers).Error

	if err != nil {
		return 0, 0, err
	}

	for _, n := range numbers {
		if existingMap[n.PhoneNumber] {
			updated++
		} else {
			inserted++
		}
	}

	return inserted, updated, nil
}

func (r *catalogRepo) ListAvailablePhoneNumbers(ctx context.Context, search string, limit int) ([]*domain.PhoneNumber, error) {
	var numbers []*domain.PhoneNumber
	q := r.db.WithContext(ctx).
		Where("assigned_agent_id IS NULL AND status IN ('available', 'unassigned')")

	if strings.TrimSpace(search) != "" {
		term := "%" + strings.TrimSpace(search) + "%"
		q = q.Where("phone_number ILIKE ? OR provider ILIKE ? OR type ILIKE ?", term, term, term)
	}

	if limit <= 0 || limit > 200 {
		limit = 100
	}

	q = q.Order("created_at DESC, phone_number ASC").Limit(limit)
	err := q.Find(&numbers).Error
	return numbers, err
}

func (r *catalogRepo) DeletePhoneNumber(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&domain.PhoneNumber{}, "id = ?", id).Error
}

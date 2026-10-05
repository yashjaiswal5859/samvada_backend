// =============================================================================
// Samvada Platform — ABS Campaign Repository (GORM PostgreSQL)
// =============================================================================

package repos

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/samvada/abs/internal/domain"
)

type campaignRepo struct {
	db *gorm.DB
}

func NewCampaignRepository(db *gorm.DB) domain.CampaignRepository {
	return &campaignRepo{db: db}
}

func (r *campaignRepo) CreateAudience(ctx context.Context, aud *domain.Audience) error {
	if aud.ID == uuid.Nil {
		aud.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(aud).Error
}

func (r *campaignRepo) GetAudienceByID(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Audience, error) {
	var aud domain.Audience
	if err := r.db.WithContext(ctx).First(&aud, "id = ? AND workspace_id = ?", id, workspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrAudienceNotFound
		}
		return nil, err
	}
	return &aud, nil
}

func (r *campaignRepo) ListAudiences(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Audience, error) {
	var auds []*domain.Audience
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&auds).Error
	return auds, err
}

func (r *campaignRepo) AddContacts(ctx context.Context, contacts []*domain.Contact) error {
	for _, c := range contacts {
		if c.ID == uuid.Nil {
			c.ID = uuid.New()
		}
	}
	return r.db.WithContext(ctx).Create(&contacts).Error
}


func (r *campaignRepo) ListContacts(ctx context.Context, audienceID uuid.UUID, limit, offset int) ([]*domain.Contact, int64, error) {
	if limit <= 0 {
		limit = 50
	}

	var count int64
	var contacts []*domain.Contact

	baseQuery := r.db.WithContext(ctx).Model(&domain.Contact{}).Where("audience_id = ?", audienceID)
	if err := baseQuery.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	err := baseQuery.
		Order("created_at ASC").
		Limit(limit).
		Offset(offset).
		Find(&contacts).Error

	return contacts, count, err
}

func (r *campaignRepo) CreateCampaign(ctx context.Context, cmp *domain.Campaign) error {
	if cmp.ID == uuid.Nil {
		cmp.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(cmp).Error
}

func (r *campaignRepo) GetCampaignByID(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Campaign, error) {
	var cmp domain.Campaign
	if err := r.db.WithContext(ctx).
		Preload("Agent").
		Preload("Audience").
		First(&cmp, "id = ? AND workspace_id = ?", id, workspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrCampaignNotFound
		}
		return nil, err
	}
	return &cmp, nil
}

func (r *campaignRepo) ListCampaigns(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Campaign, error) {
	var campaigns []*domain.Campaign
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Preload("Agent").
		Preload("Audience").
		Order("created_at DESC").
		Find(&campaigns).Error
	return campaigns, err
}

func (r *campaignRepo) UpdateCampaignStatus(ctx context.Context, workspaceID, id uuid.UUID, status string) error {
	res := r.db.WithContext(ctx).
		Model(&domain.Campaign{}).
		Where("id = ? AND workspace_id = ?", id, workspaceID).
		Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrCampaignNotFound
	}
	return nil
}

func (r *campaignRepo) DeleteAudience(ctx context.Context, workspaceID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&domain.Audience{}, "id = ? AND workspace_id = ?", id, workspaceID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrAudienceNotFound
	}
	return nil
}

func (r *campaignRepo) DeleteContact(ctx context.Context, audienceID, contactID uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&domain.Contact{}, "id = ? AND audience_id = ?", contactID, audienceID).Error
}

func (r *campaignRepo) GetNextPendingContacts(ctx context.Context, audienceID uuid.UUID, limit int) ([]*domain.Contact, error) {
	if limit <= 0 {
		limit = 10
	}
	var contacts []*domain.Contact
	err := r.db.WithContext(ctx).
		Where("audience_id = ? AND status = ? AND last_called_at IS NULL", audienceID, "active").
		Order("created_at ASC").
		Limit(limit).
		Find(&contacts).Error
	return contacts, err
}

func (r *campaignRepo) UpdateContactLastCalled(ctx context.Context, contactID uuid.UUID, status string) error {
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"last_called_at": now,
	}
	if status != "" {
		updates["status"] = status
	}
	return r.db.WithContext(ctx).Model(&domain.Contact{}).Where("id = ?", contactID).Updates(updates).Error
}

func (r *campaignRepo) ListRunningCampaigns(ctx context.Context) ([]*domain.Campaign, error) {
	var campaigns []*domain.Campaign
	err := r.db.WithContext(ctx).
		Where("status = ? AND (scheduled_at IS NULL OR scheduled_at <= ?)", "running", time.Now().UTC()).
		Preload("Agent").
		Preload("Audience").
		Find(&campaigns).Error
	return campaigns, err
}

func (r *campaignRepo) IncrementCampaignStats(ctx context.Context, campaignID uuid.UUID, completed, successful, failed int) error {
	updates := map[string]interface{}{}
	if completed > 0 {
		updates["completed_calls"] = gorm.Expr("completed_calls + ?", completed)
	}
	if successful > 0 {
		updates["successful_calls"] = gorm.Expr("successful_calls + ?", successful)
	}
	if failed > 0 {
		updates["failed_calls"] = gorm.Expr("failed_calls + ?", failed)
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&domain.Campaign{}).Where("id = ?", campaignID).Updates(updates).Error
}

func (r *campaignRepo) DeleteCampaign(ctx context.Context, workspaceID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Delete(&domain.Campaign{}, "id = ? AND workspace_id = ?", id, workspaceID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrCampaignNotFound
	}
	return nil
}

// =============================================================================
// Samvada Platform — ABS Telephony Repository (GORM PostgreSQL)
// =============================================================================

package repos

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/samvada/abs/internal/domain"
)

type telephonyRepo struct {
	db *gorm.DB
}

func NewTelephonyRepository(db *gorm.DB) domain.TelephonyRepository {
	return &telephonyRepo{db: db}
}

func (r *telephonyRepo) Create(ctx context.Context, pn *domain.PhoneNumber) error {
	if pn.ID == uuid.Nil {
		pn.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(pn).Error
}

func (r *telephonyRepo) GetByID(ctx context.Context, workspaceID, id uuid.UUID) (*domain.PhoneNumber, error) {
	var pn domain.PhoneNumber
	if err := r.db.WithContext(ctx).
		Preload("AssignedAgent").
		First(&pn, "id = ? AND workspace_id = ?", id, workspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPhoneNumberNotFound
		}
		return nil, err
	}
	return &pn, nil
}

func (r *telephonyRepo) GetByNumber(ctx context.Context, number string) (*domain.PhoneNumber, error) {
	var pn domain.PhoneNumber
	if err := r.db.WithContext(ctx).
		Preload("AssignedAgent").
		First(&pn, "phone_number = ?", number).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPhoneNumberNotFound
		}
		return nil, err
	}
	return &pn, nil
}

func (r *telephonyRepo) ListByWorkspace(ctx context.Context, workspaceID uuid.UUID, search string) ([]*domain.PhoneNumber, error) {
	var numbers []*domain.PhoneNumber
	q := r.db.WithContext(ctx)
	if workspaceID != uuid.Nil {
		q = q.Where("workspace_id = ?", workspaceID)
	}
	q = q.Preload("AssignedAgent").Order("created_at DESC")

	if strings.TrimSpace(search) != "" {
		term := "%" + strings.TrimSpace(search) + "%"
		q = q.Where(
			"phone_number ILIKE ? OR provider ILIKE ? OR type ILIKE ? OR status ILIKE ? OR EXISTS (SELECT 1 FROM agents WHERE agents.id = phone_numbers.assigned_agent_id AND agents.name ILIKE ?)",
			term, term, term, term, term,
		)
	}

	err := q.Find(&numbers).Error
	return numbers, err
}

func (r *telephonyRepo) ListAvailable(ctx context.Context, workspaceID *uuid.UUID, search string) ([]*domain.PhoneNumber, error) {
	var numbers []*domain.PhoneNumber
	q := r.db.WithContext(ctx).
		Where("assigned_agent_id IS NULL AND status IN ('available', 'unassigned')")

	if workspaceID != nil && *workspaceID != uuid.Nil {
		q = q.Where("workspace_id IS NULL OR workspace_id = ?", *workspaceID)
	} else {
		q = q.Where("workspace_id IS NULL")
	}

	if strings.TrimSpace(search) != "" {
		term := "%" + strings.TrimSpace(search) + "%"
		q = q.Where("phone_number ILIKE ? OR provider ILIKE ? OR type ILIKE ?", term, term, term)
	}

	q = q.Order("phone_number ASC")
	err := q.Find(&numbers).Error
	return numbers, err
}

func (r *telephonyRepo) AllocateNumber(ctx context.Context, workspaceID, numberID uuid.UUID, agentID *uuid.UUID) (*domain.PhoneNumber, error) {
	var pn domain.PhoneNumber
	if err := r.db.WithContext(ctx).First(&pn, "id = ?", numberID).Error; err != nil {
		return nil, err
	}

	updates := map[string]interface{}{
		"workspace_id": workspaceID,
		"status":       "active",
	}
	if agentID != nil && *agentID != uuid.Nil {
		updates["assigned_agent_id"] = *agentID
	} else {
		updates["assigned_agent_id"] = nil
	}

	if err := r.db.WithContext(ctx).Model(&pn).Updates(updates).Error; err != nil {
		return nil, err
	}

	if err := r.db.WithContext(ctx).Preload("AssignedAgent").First(&pn, "id = ?", numberID).Error; err != nil {
		return nil, err
	}
	return &pn, nil
}

func (r *telephonyRepo) AssignAgent(ctx context.Context, workspaceID, id, agentID uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&domain.PhoneNumber{}).
		Where("id = ? AND workspace_id = ?", id, workspaceID).
		Update("assigned_agent_id", agentID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrPhoneNumberNotFound
	}
	return nil
}

func (r *telephonyRepo) UnassignAgent(ctx context.Context, workspaceID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&domain.PhoneNumber{}).
		Where("id = ? AND workspace_id = ?", id, workspaceID).
		Update("assigned_agent_id", nil)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrPhoneNumberNotFound
	}
	return nil
}

func (r *telephonyRepo) Delete(ctx context.Context, workspaceID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Delete(&domain.PhoneNumber{}, "id = ? AND workspace_id = ?", id, workspaceID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrPhoneNumberNotFound
	}
	return nil
}

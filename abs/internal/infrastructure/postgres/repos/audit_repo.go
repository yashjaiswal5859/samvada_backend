// =============================================================================
// Samvada Platform — PostgreSQL Audit Log Repository Implementation
// =============================================================================

package repos

import (
	"context"

	"gorm.io/gorm"

	"github.com/samvada/abs/internal/domain"
)

type AuditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Record(ctx context.Context, entry *domain.AuditLog) error {
	return r.db.WithContext(ctx).Create(entry).Error
}

func (r *AuditRepository) List(ctx context.Context, filter domain.AuditLogFilter) ([]*domain.AuditLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&domain.AuditLog{})

	if filter.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", *filter.WorkspaceID)
	}
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.ResourceType != "" {
		query = query.Where("resource_type = ?", filter.ResourceType)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	var logs []*domain.AuditLog
	err := query.Order("created_at DESC").
		Limit(limit).
		Offset(filter.Offset).
		Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

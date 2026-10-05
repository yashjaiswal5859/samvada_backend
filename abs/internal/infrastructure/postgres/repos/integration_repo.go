// =============================================================================
// Samvada Platform — PostgreSQL Connector Repository Implementation
// =============================================================================

package repos

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/samvada/abs/internal/domain"
)

type ConnectorRepository struct {
	db *gorm.DB
}

func NewConnectorRepository(db *gorm.DB) *ConnectorRepository {
	return &ConnectorRepository{db: db}
}

func (r *ConnectorRepository) ListByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]domain.ConnectorIntegration, error) {
	var list []domain.ConnectorIntegration
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("created_at ASC").
		Find(&list).Error
	return list, err
}

func (r *ConnectorRepository) GetByProvider(ctx context.Context, workspaceID uuid.UUID, provider string) (*domain.ConnectorIntegration, error) {
	var item domain.ConnectorIntegration
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND provider = ?", workspaceID, provider).
		First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *ConnectorRepository) GetByID(ctx context.Context, workspaceID, id uuid.UUID) (*domain.ConnectorIntegration, error) {
	var item domain.ConnectorIntegration
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND id = ?", workspaceID, id).
		First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *ConnectorRepository) Upsert(ctx context.Context, item *domain.ConnectorIntegration) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "workspace_id"}, {Name: "provider"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "is_connected", "credentials_json", "config_json", "updated_at",
		}),
	}).Create(item).Error
}

func (r *ConnectorRepository) Toggle(ctx context.Context, workspaceID, id uuid.UUID, connected bool) error {
	return r.db.WithContext(ctx).Model(&domain.ConnectorIntegration{}).
		Where("workspace_id = ? AND id = ?", workspaceID, id).
		Updates(map[string]interface{}{
			"is_connected": connected,
			"updated_at":   time.Now().UTC(),
		}).Error
}

func (r *ConnectorRepository) UpdateCredentials(ctx context.Context, workspaceID, id uuid.UUID, credsJSON string) error {
	return r.db.WithContext(ctx).Model(&domain.ConnectorIntegration{}).
		Where("workspace_id = ? AND id = ?", workspaceID, id).
		Updates(map[string]interface{}{
			"credentials_json": credsJSON,
			"is_connected":    true,
			"updated_at":      time.Now().UTC(),
		}).Error
}

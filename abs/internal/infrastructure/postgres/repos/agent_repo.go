// =============================================================================
// Samvada Platform — ABS Agent Repository (GORM PostgreSQL)
// =============================================================================

package repos

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/samvada/abs/internal/domain"
)

type agentRepo struct {
	db *gorm.DB
}

func NewAgentRepository(db *gorm.DB) domain.AgentRepository {
	return &agentRepo{db: db}
}

func (r *agentRepo) Create(ctx context.Context, agent *domain.Agent) error {
	if agent.ID == uuid.Nil {
		agent.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(agent).Error
}

func (r *agentRepo) GetByID(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Agent, error) {
	var agent domain.Agent
	if err := r.db.WithContext(ctx).
		Preload("Versions", func(db *gorm.DB) *gorm.DB {
			return db.Order("version_num DESC")
		}).
		First(&agent, "id = ? AND workspace_id = ?", id, workspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrAgentNotFound
		}
		return nil, err
	}
	return &agent, nil
}

func (r *agentRepo) ListByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Agent, error) {
	var agents []*domain.Agent
	err := r.db.WithContext(ctx).
		Preload("Versions", func(db *gorm.DB) *gorm.DB {
			return db.Order("version_num DESC")
		}).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&agents).Error
	return agents, err
}

func (r *agentRepo) Update(ctx context.Context, agent *domain.Agent) error {
	res := r.db.WithContext(ctx).
		Where("id = ? AND workspace_id = ?", agent.ID, agent.WorkspaceID).
		Save(agent)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrAgentNotFound
	}
	return nil
}

func (r *agentRepo) Delete(ctx context.Context, workspaceID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Delete(&domain.Agent{}, "id = ? AND workspace_id = ?", id, workspaceID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrAgentNotFound
	}
	return nil
}

func (r *agentRepo) CountByWorkspace(ctx context.Context, workspaceID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.Agent{}).
		Where("workspace_id = ?", workspaceID).
		Count(&count).Error
	return count, err
}

func (r *agentRepo) CreateVersion(ctx context.Context, version *domain.AgentVersion) error {
	if version.ID == uuid.Nil {
		version.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(version).Error
}

func (r *agentRepo) UpdateVersion(ctx context.Context, version *domain.AgentVersion) error {
	return r.db.WithContext(ctx).Save(version).Error
}

func (r *agentRepo) GetVersion(ctx context.Context, agentID uuid.UUID, versionNumber int) (*domain.AgentVersion, error) {
	var v domain.AgentVersion
	if err := r.db.WithContext(ctx).
		First(&v, "agent_id = ? AND version_num = ?", agentID, versionNumber).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrAgentVersionNotFound
		}
		return nil, err
	}
	return &v, nil
}

func (r *agentRepo) ListVersions(ctx context.Context, agentID uuid.UUID) ([]*domain.AgentVersion, error) {
	var versions []*domain.AgentVersion
	err := r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Order("version_num DESC").
		Find(&versions).Error
	return versions, err
}

func (r *agentRepo) GetLatestVersion(ctx context.Context, agentID uuid.UUID) (*domain.AgentVersion, error) {
	var v domain.AgentVersion
	if err := r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Order("version_num DESC").
		First(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrAgentVersionNotFound
		}
		return nil, err
	}
	return &v, nil
}

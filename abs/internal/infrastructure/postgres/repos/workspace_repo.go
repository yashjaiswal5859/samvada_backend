// =============================================================================
// Samvada Platform — ABS Workspace Repository (GORM PostgreSQL)
// =============================================================================

package repos

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/samvada/abs/internal/domain"
)

type workspaceRepo struct {
	db *gorm.DB
}

func NewWorkspaceRepository(db *gorm.DB) domain.WorkspaceRepository {
	return &workspaceRepo{db: db}
}

func (r *workspaceRepo) Create(ctx context.Context, ws *domain.Workspace) error {
	if ws.ID == uuid.Nil {
		ws.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(ws).Error
}

func (r *workspaceRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Workspace, error) {
	var ws domain.Workspace
	if err := r.db.WithContext(ctx).First(&ws, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrWorkspaceNotFound
		}
		return nil, err
	}
	return &ws, nil
}

func (r *workspaceRepo) GetBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*domain.Workspace, error) {
	var ws domain.Workspace
	if err := r.db.WithContext(ctx).First(&ws, "org_id = ? AND slug = ?", orgID, slug).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrWorkspaceNotFound
		}
		return nil, err
	}
	return &ws, nil
}

func (r *workspaceRepo) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]*domain.Workspace, error) {
	var workspaces []*domain.Workspace
	err := r.db.WithContext(ctx).Find(&workspaces, "org_id = ?", orgID).Error
	return workspaces, err
}

func (r *workspaceRepo) Update(ctx context.Context, ws *domain.Workspace) error {
	return r.db.WithContext(ctx).Save(ws).Error
}

func (r *workspaceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&domain.Workspace{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrWorkspaceNotFound
	}
	return nil
}

func (r *workspaceRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]*domain.Workspace, error) {
	var workspaces []*domain.Workspace
	err := r.db.WithContext(ctx).
		Table("workspaces").
		Joins("JOIN workspace_members ON workspace_members.workspace_id = workspaces.id").
		Where("workspace_members.user_id = ?", userID).
		Find(&workspaces).Error
	return workspaces, err
}

func (r *workspaceRepo) AddMember(ctx context.Context, member *domain.WorkspaceMember) error {
	if member.ID == uuid.Nil {
		member.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(member).Error
}

func (r *workspaceRepo) GetMember(ctx context.Context, workspaceID, userID uuid.UUID) (*domain.WorkspaceMember, error) {
	var member domain.WorkspaceMember
	if err := r.db.WithContext(ctx).First(&member, "workspace_id = ? AND user_id = ?", workspaceID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrWorkspaceMemberNotFound
		}
		return nil, err
	}
	return &member, nil
}

func (r *workspaceRepo) ListMembers(ctx context.Context, workspaceID uuid.UUID) ([]*domain.WorkspaceMember, error) {
	var members []*domain.WorkspaceMember
	err := r.db.WithContext(ctx).Find(&members, "workspace_id = ?", workspaceID).Error
	return members, err
}

func (r *workspaceRepo) UpdateMemberRole(ctx context.Context, workspaceID, userID uuid.UUID, role string) error {
	res := r.db.WithContext(ctx).
		Model(&domain.WorkspaceMember{}).
		Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		Update("role", role)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrWorkspaceMemberNotFound
	}
	return nil
}

func (r *workspaceRepo) RemoveMember(ctx context.Context, workspaceID, userID uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Delete(&domain.WorkspaceMember{}, "workspace_id = ? AND user_id = ?", workspaceID, userID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrWorkspaceMemberNotFound
	}
	return nil
}

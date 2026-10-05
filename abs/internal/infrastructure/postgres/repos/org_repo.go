// =============================================================================
// Samvada Platform — ABS Organization Repository (GORM PostgreSQL)
// =============================================================================

package repos

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/samvada/abs/internal/domain"
)

type orgRepo struct {
	db *gorm.DB
}

func NewOrgRepository(db *gorm.DB) domain.OrgRepository {
	return &orgRepo{db: db}
}

func (r *orgRepo) Create(ctx context.Context, org *domain.Organization) error {
	if org.ID == uuid.Nil {
		org.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(org).Error
}

func (r *orgRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Organization, error) {
	var org domain.Organization
	if err := r.db.WithContext(ctx).Preload("Workspaces").First(&org, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrOrgNotFound
		}
		return nil, err
	}
	return &org, nil
}

func (r *orgRepo) GetBySlug(ctx context.Context, slug string) (*domain.Organization, error) {
	var org domain.Organization
	if err := r.db.WithContext(ctx).First(&org, "slug = ?", slug).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrOrgNotFound
		}
		return nil, err
	}
	return &org, nil
}

func (r *orgRepo) Update(ctx context.Context, org *domain.Organization) error {
	return r.db.WithContext(ctx).Save(org).Error
}

func (r *orgRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&domain.Organization{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrOrgNotFound
	}
	return nil
}

func (r *orgRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]*domain.Organization, error) {
	var orgs []*domain.Organization
	err := r.db.WithContext(ctx).
		Table("organizations").
		Joins("JOIN org_members ON org_members.org_id = organizations.id").
		Where("org_members.user_id = ?", userID).
		Preload("Workspaces").
		Find(&orgs).Error
	return orgs, err
}

func (r *orgRepo) AddMember(ctx context.Context, member *domain.OrgMember) error {
	if member.ID == uuid.Nil {
		member.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(member).Error
}

func (r *orgRepo) GetMember(ctx context.Context, orgID, userID uuid.UUID) (*domain.OrgMember, error) {
	var member domain.OrgMember
	if err := r.db.WithContext(ctx).First(&member, "org_id = ? AND user_id = ?", orgID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrOrgMemberNotFound
		}
		return nil, err
	}
	return &member, nil
}

func (r *orgRepo) ListMembers(ctx context.Context, orgID uuid.UUID) ([]*domain.OrgMember, error) {
	var members []*domain.OrgMember
	err := r.db.WithContext(ctx).Find(&members, "org_id = ?", orgID).Error
	return members, err
}

func (r *orgRepo) UpdateMemberRole(ctx context.Context, orgID, userID uuid.UUID, newRole string) error {
	res := r.db.WithContext(ctx).
		Model(&domain.OrgMember{}).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Update("role", newRole)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrOrgMemberNotFound
	}
	return nil
}

func (r *orgRepo) RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Delete(&domain.OrgMember{}, "org_id = ? AND user_id = ?", orgID, userID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrOrgMemberNotFound
	}
	return nil
}

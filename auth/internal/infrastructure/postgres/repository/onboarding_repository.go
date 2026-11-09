package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/ridesafety/auth-service/internal/domain/onboarding"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres/models"
	"github.com/ridesafety/auth-service/internal/platform/eventbus"
	"gorm.io/gorm"
)

type onboardingRepository struct {
	db *gorm.DB
}

func NewOnboardingRepository(db *gorm.DB) onboarding.Repository {
	return &onboardingRepository{db: db}
}

func (r *onboardingRepository) getTx(ctx context.Context) *gorm.DB {
	if tx, ok := eventbus.GetTx(ctx); ok {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func (r *onboardingRepository) CreateProfile(ctx context.Context, p *onboarding.UserProfile) error {
	dbModel := models.UserProfile{
		ID:                  p.ID,
		UserID:              p.UserID,
		FirstName:           p.FirstName,
		LastName:            p.LastName,
		AccountType:         p.AccountType,
		Industries:          models.StringArray(p.Industries),
		OnboardingCompleted: p.OnboardingCompleted,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
	}
	return r.getTx(ctx).Create(&dbModel).Error
}

func (r *onboardingRepository) GetProfileByUserID(ctx context.Context, userID uuid.UUID) (*onboarding.UserProfile, error) {
	var dbModel models.UserProfile
	err := r.getTx(ctx).First(&dbModel, "user_id = ?", userID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &onboarding.UserProfile{
		ID:                  dbModel.ID,
		UserID:              dbModel.UserID,
		FirstName:           dbModel.FirstName,
		LastName:            dbModel.LastName,
		AccountType:         dbModel.AccountType,
		Industries:          []string(dbModel.Industries),
		OnboardingCompleted: dbModel.OnboardingCompleted,
		CreatedAt:           dbModel.CreatedAt,
		UpdatedAt:           dbModel.UpdatedAt,
	}, nil
}

func (r *onboardingRepository) UpdateProfile(ctx context.Context, p *onboarding.UserProfile) error {
	dbModel := models.UserProfile{
		ID:                  p.ID,
		UserID:              p.UserID,
		FirstName:           p.FirstName,
		LastName:            p.LastName,
		AccountType:         p.AccountType,
		Industries:          models.StringArray(p.Industries),
		OnboardingCompleted: p.OnboardingCompleted,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
	}
	return r.getTx(ctx).Save(&dbModel).Error
}

func (r *onboardingRepository) CreateOrganization(ctx context.Context, org *onboarding.Organization) error {
	dbModel := models.Organization{
		ID:        org.ID,
		Name:      org.Name,
		OwnerID:   org.OwnerID,
		CreatedAt: org.CreatedAt,
		UpdatedAt: org.UpdatedAt,
	}
	return r.getTx(ctx).Create(&dbModel).Error
}

func (r *onboardingRepository) GetOrganizationByOwnerID(ctx context.Context, ownerID uuid.UUID) (*onboarding.Organization, error) {
	var dbModel models.Organization
	err := r.getTx(ctx).First(&dbModel, "owner_id = ?", ownerID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &onboarding.Organization{
		ID:        dbModel.ID,
		Name:      dbModel.Name,
		OwnerID:   dbModel.OwnerID,
		CreatedAt: dbModel.CreatedAt,
		UpdatedAt: dbModel.UpdatedAt,
	}, nil
}

func (r *onboardingRepository) CreateWorkspace(ctx context.Context, ws *onboarding.Workspace) error {
	dbModel := models.Workspace{
		ID:        ws.ID,
		Name:      ws.Name,
		OrgID:     ws.OrgID,
		OwnerID:   ws.OwnerID,
		IsDefault: ws.IsDefault,
		CreatedAt: ws.CreatedAt,
		UpdatedAt: ws.UpdatedAt,
	}
	return r.getTx(ctx).Create(&dbModel).Error
}

func (r *onboardingRepository) GetDefaultWorkspaceByOwnerID(ctx context.Context, ownerID uuid.UUID) (*onboarding.Workspace, error) {
	var dbModel models.Workspace
	err := r.getTx(ctx).First(&dbModel, "owner_id = ? AND is_default = ?", ownerID, true).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &onboarding.Workspace{
		ID:        dbModel.ID,
		Name:      dbModel.Name,
		OrgID:     dbModel.OrgID,
		OwnerID:   dbModel.OwnerID,
		IsDefault: dbModel.IsDefault,
		CreatedAt: dbModel.CreatedAt,
		UpdatedAt: dbModel.UpdatedAt,
	}, nil
}

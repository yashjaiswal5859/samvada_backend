// =============================================================================
// Samvada Platform — PostgreSQL User Profile Repository Implementation
// =============================================================================

package repos

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/samvada/abs/internal/domain"
)

type ProfileRepository struct {
	db *gorm.DB
}

func NewProfileRepository(db *gorm.DB) *ProfileRepository {
	return &ProfileRepository{db: db}
}

func (r *ProfileRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	var profile domain.UserProfile
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *ProfileRepository) Upsert(ctx context.Context, profile *domain.UserProfile) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"first_name", "last_name", "email", "phone", "company",
			"job_title", "timezone", "language", "bio", "onboarding_completed", "updated_at",
		}),
	}).Create(profile).Error
}

func (r *ProfileRepository) UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error {
	return r.db.WithContext(ctx).Model(&domain.UserProfile{}).
		Where("user_id = ?", userID).
		Update("avatar_url", avatarURL).Error
}

func (r *ProfileRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&domain.UserProfile{}).Error
}

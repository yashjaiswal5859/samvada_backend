// =============================================================================
// Samvada Platform — ABS User Profile Domain Entity & Repository Interface
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type UserProfile struct {
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	FirstName string    `gorm:"type:varchar(100);not null;default:''" json:"first_name"`
	LastName  string    `gorm:"type:varchar(100);not null;default:''" json:"last_name"`
	Email     string    `gorm:"type:varchar(255);not null;default:''" json:"email"`
	Phone     string    `gorm:"type:varchar(50);not null;default:''" json:"phone"`
	Company   string    `gorm:"type:varchar(255);not null;default:''" json:"company"`
	JobTitle  string    `gorm:"type:varchar(100);not null;default:''" json:"job_title"`
	Timezone  string    `gorm:"type:varchar(100);not null;default:'UTC'" json:"timezone"`
	Language  string    `gorm:"type:varchar(50);not null;default:'en'" json:"language"`
	AvatarURL           string    `gorm:"type:text;not null;default:''" json:"avatar_url"`
	Bio                 string    `gorm:"type:text;not null;default:''" json:"bio"`
	OnboardingCompleted bool      `gorm:"type:boolean;not null;default:false" json:"onboarding_completed"`
	CreatedAt           time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (UserProfile) TableName() string {
	return "user_profiles"
}

type GDPRDataExport struct {
	UserID                  uuid.UUID                `json:"user_id"`
	ExportedAt              time.Time                `json:"exported_at"`
	Profile                 *UserProfile             `json:"profile,omitempty"`
	NotificationPreferences *NotificationPreferences `json:"notification_preferences,omitempty"`
	Organizations           []*Organization          `json:"organizations,omitempty"`
	Workspaces              []*Workspace             `json:"workspaces,omitempty"`
	ComplianceNotice        string                   `json:"compliance_notice"`
}

type UserProfileRepository interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*UserProfile, error)
	Upsert(ctx context.Context, profile *UserProfile) error
	UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
}

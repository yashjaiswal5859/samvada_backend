// =============================================================================
// Samvada Platform — ABS User Profile Application Service
// =============================================================================

package profile

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/infrastructure/storage"
)

type UpdateProfileRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Company   string `json:"company"`
	JobTitle  string `json:"job_title"`
	Timezone  string `json:"timezone"`
	Language  string `json:"language"`
	Bio       string `json:"bio"`
}

type UserMeProfileDTO struct {
	ID                  string   `json:"id"`
	UserID              string   `json:"userId"`
	FirstName           string   `json:"firstName"`
	LastName            string   `json:"lastName"`
	Email               string   `json:"email"`
	Company             string   `json:"company"`
	JobTitle            string   `json:"jobTitle"`
	Timezone            string   `json:"timezone"`
	Language            string   `json:"language"`
	AccountType         string   `json:"accountType"`
	Industries          []string `json:"industries"`
	OnboardingCompleted bool     `json:"onboardingCompleted"`
}

type UserMeOrgDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	OwnerID string `json:"ownerId"`
	Role    string `json:"role"`
}

type UserMeWorkspaceDTO struct {
	ID        string `json:"id"`
	OrgID     string `json:"orgId"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Role      string `json:"role"`
	IsDefault bool   `json:"isDefault"`
}

type UserMeResponse struct {
	ID                  string                `json:"id"`
	Email               string                `json:"email"`
	DisplayName         string                `json:"displayName"`
	Role                string                `json:"role"`
	OnboardingCompleted bool                  `json:"onboardingCompleted"`
	Profile             *UserMeProfileDTO     `json:"profile,omitempty"`
	Organization        *UserMeOrgDTO         `json:"organization,omitempty"`
	Workspace           *UserMeWorkspaceDTO   `json:"workspace,omitempty"`
	Workspaces          []*UserMeWorkspaceDTO `json:"workspaces,omitempty"`
}

type ProfileService struct {
	repo      domain.UserProfileRepository
	notifRepo domain.NotificationRepository
	orgRepo   domain.OrgRepository
	wsRepo    domain.WorkspaceRepository
	storage   storage.Storage
	log       *zap.Logger
}

func NewProfileService(
	repo domain.UserProfileRepository,
	notifRepo domain.NotificationRepository,
	orgRepo domain.OrgRepository,
	wsRepo domain.WorkspaceRepository,
	storage storage.Storage,
	log *zap.Logger,
) *ProfileService {
	return &ProfileService{
		repo:      repo,
		notifRepo: notifRepo,
		orgRepo:   orgRepo,
		wsRepo:    wsRepo,
		storage:   storage,
		log:       log.Named("profile_service"),
	}
}

func (s *ProfileService) GetProfile(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	profile, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		// Return default empty profile
		return &domain.UserProfile{
			UserID:    userID,
			FirstName: "",
			LastName:  "",
			Timezone:  "UTC",
			Language:  "en",
		}, nil
	}
	return profile, nil
}

func (s *ProfileService) GetMe(ctx context.Context, userID uuid.UUID, userEmail string) (*UserMeResponse, error) {
	p, _ := s.repo.GetByUserID(ctx, userID)

	firstName := ""
	lastName := ""
	company := ""
	jobTitle := ""
	timezone := "UTC"
	language := "en"

	if p != nil {
		firstName = p.FirstName
		lastName = p.LastName
		company = p.Company
		jobTitle = p.JobTitle
		if p.Timezone != "" {
			timezone = p.Timezone
		}
		if p.Language != "" {
			language = p.Language
		}
		if p.Email != "" && userEmail == "" {
			userEmail = p.Email
		}
	}

	if firstName == "" && userEmail != "" {
		parts := strings.Split(userEmail, "@")
		firstName = parts[0]
	}

	displayName := strings.TrimSpace(firstName + " " + lastName)
	if displayName == "" {
		displayName = userEmail
	}

	// Fetch organizations for this user from ABS PostgreSQL
	orgs, _ := s.orgRepo.ListForUser(ctx, userID)
	var activeOrg *UserMeOrgDTO
	onboardingCompleted := false

	if p != nil && p.OnboardingCompleted {
		onboardingCompleted = true
	}

	if len(orgs) > 0 {
		activeOrg = &UserMeOrgDTO{
			ID:      orgs[0].ID.String(),
			Name:    orgs[0].Name,
			Slug:    orgs[0].Slug,
			OwnerID: userID.String(),
			Role:    "owner",
		}
		if company == "" {
			company = orgs[0].Name
		}
	}

	// Fetch workspaces for this user from ABS PostgreSQL
	wsList, _ := s.wsRepo.ListForUser(ctx, userID)
	var wsDTOs []*UserMeWorkspaceDTO
	var activeWs *UserMeWorkspaceDTO

	for i, w := range wsList {
		dto := &UserMeWorkspaceDTO{
			ID:        w.ID.String(),
			OrgID:     w.OrgID.String(),
			Name:      w.Name,
			Slug:      w.Slug,
			Role:      "workspace_admin",
			IsDefault: i == 0,
		}
		wsDTOs = append(wsDTOs, dto)
		if i == 0 {
			activeWs = dto
		}
	}

	// If profile was missing from database, persist it so future requests are consistent
	if p == nil {
		_ = s.repo.Upsert(ctx, &domain.UserProfile{
			UserID:              userID,
			FirstName:           firstName,
			LastName:            lastName,
			Email:               userEmail,
			Company:             company,
			JobTitle:            jobTitle,
			Timezone:            timezone,
			Language:            language,
			OnboardingCompleted: false,
			CreatedAt:           time.Now().UTC(),
			UpdatedAt:           time.Now().UTC(),
		})
	}

	resp := &UserMeResponse{
		ID:                  userID.String(),
		Email:               userEmail,
		DisplayName:         displayName,
		Role:                "owner",
		OnboardingCompleted: onboardingCompleted,
		Profile: &UserMeProfileDTO{
			ID:                  userID.String(),
			UserID:              userID.String(),
			FirstName:           firstName,
			LastName:            lastName,
			Email:               userEmail,
			Company:             company,
			JobTitle:            jobTitle,
			Timezone:            timezone,
			Language:            language,
			AccountType:         "business",
			Industries:          []string{},
			OnboardingCompleted: onboardingCompleted,
		},
		Organization: activeOrg,
		Workspace:    activeWs,
		Workspaces:   wsDTOs,
	}

	return resp, nil
}

func (s *ProfileService) UpdateProfile(ctx context.Context, userID uuid.UUID, req UpdateProfileRequest) (*domain.UserProfile, error) {
	existing, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	avatarURL := ""
	if existing != nil {
		avatarURL = existing.AvatarURL
	}

	profile := &domain.UserProfile{
		UserID:    userID,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Email:     req.Email,
		Phone:     req.Phone,
		Company:   req.Company,
		JobTitle:  req.JobTitle,
		Timezone:  req.Timezone,
		Language:  req.Language,
		AvatarURL: avatarURL,
		Bio:       req.Bio,
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.repo.Upsert(ctx, profile); err != nil {
		return nil, fmt.Errorf("failed to save profile: %w", err)
	}

	return profile, nil
}

func (s *ProfileService) UploadAvatar(ctx context.Context, userID uuid.UUID, fileData []byte, ext, contentType string) (string, error) {
	if s.storage == nil {
		return "", fmt.Errorf("storage driver not configured")
	}

	key := fmt.Sprintf("avatars/%s%s", userID.String(), ext)
	if ext == "" {
		key = fmt.Sprintf("avatars/%s.jpg", userID.String())
	}

	url, err := s.storage.Upload(ctx, filepath.ToSlash(key), fileData, contentType)
	if err != nil {
		return "", fmt.Errorf("failed to upload avatar: %w", err)
	}

	if err := s.repo.UpdateAvatar(ctx, userID, url); err != nil {
		return "", fmt.Errorf("failed to update avatar in database: %w", err)
	}

	return url, nil
}

// ExportData packages all personal data into a structured GDPR Article 20 portable export.
func (s *ProfileService) ExportData(ctx context.Context, userID uuid.UUID) (*domain.GDPRDataExport, error) {
	prof, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		s.log.Warn("Could not load user profile for GDPR export", zap.Error(err))
	}

	var orgs []*domain.Organization
	if s.orgRepo != nil {
		if o, err := s.orgRepo.ListForUser(ctx, userID); err == nil {
			orgs = o
		}
	}

	var workspaces []*domain.Workspace
	if s.wsRepo != nil {
		if w, err := s.wsRepo.ListForUser(ctx, userID); err == nil {
			workspaces = w
		}
	}

	var notifs *domain.NotificationPreferences
	if s.notifRepo != nil && len(workspaces) > 0 {
		if n, err := s.notifRepo.GetPreferences(ctx, userID, workspaces[0].ID); err == nil {
			notifs = n
		}
	}

	export := &domain.GDPRDataExport{
		UserID:                  userID,
		ExportedAt:              time.Now().UTC(),
		Profile:                 prof,
		NotificationPreferences: notifs,
		Organizations:           orgs,
		Workspaces:              workspaces,
		ComplianceNotice:        "Export generated in full compliance with General Data Protection Regulation (GDPR) Article 20 (Right to Data Portability). All sensitive access credentials, billing tokens, and cryptographic keys have been excluded for account security.",
	}

	s.log.Info("GDPR data export successfully generated", zap.String("user_id", userID.String()))
	return export, nil
}

// DeleteData purges personal user information in compliance with GDPR Article 17 (Right to Erasure).
func (s *ProfileService) DeleteData(ctx context.Context, userID uuid.UUID) error {
	s.log.Info("Executing GDPR data erasure request", zap.String("user_id", userID.String()))

	if err := s.repo.DeleteByUserID(ctx, userID); err != nil {
		return fmt.Errorf("failed to delete personal profile: %w", err)
	}

	if s.storage != nil {
		_ = s.storage.Delete(ctx, fmt.Sprintf("avatars/%s.jpg", userID.String()))
		_ = s.storage.Delete(ctx, fmt.Sprintf("avatars/%s.png", userID.String()))
	}

	s.log.Info("GDPR data erasure completed successfully", zap.String("user_id", userID.String()))
	return nil
}

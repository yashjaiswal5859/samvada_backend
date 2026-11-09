package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/ridesafety/auth-service/internal/domain/events"
	domOnboarding "github.com/ridesafety/auth-service/internal/domain/onboarding"
	"github.com/ridesafety/auth-service/internal/infrastructure/kafka"
	"github.com/ridesafety/auth-service/internal/platform/eventbus"
)

type ProfileRequest struct {
	FirstName     string  `json:"firstName"`
	LastName      *string `json:"lastName"`
	AccountType   string  `json:"accountType"` // "business" | "individual"
	OrgName       string  `json:"orgName"`
	WorkspaceName string  `json:"workspaceName"`
}

type IndustriesRequest struct {
	Industries []string `json:"industries"`
}

type OnboardingState struct {
	Profile             *domOnboarding.UserProfile  `json:"profile"`
	Organization        *domOnboarding.Organization `json:"organization,omitempty"`
	Workspace           *domOnboarding.Workspace    `json:"workspace,omitempty"`
	OnboardingCompleted bool                        `json:"onboardingCompleted"`
	WalletBalanceUSD    float64                     `json:"walletBalanceUSD,omitempty"`
	DemoCreditsUSD      float64                     `json:"demoCreditsUSD,omitempty"`
	RemainingMinutes    int                         `json:"remainingMinutes,omitempty"`
}

type Service interface {
	SaveProfile(ctx context.Context, userID uuid.UUID, userEmail string, req ProfileRequest) (*OnboardingState, error)
	SaveIndustries(ctx context.Context, userID uuid.UUID, userEmail string, req IndustriesRequest) (*OnboardingState, error)
	GetOnboardingState(ctx context.Context, userID uuid.UUID) (*OnboardingState, error)
}

type service struct {
	repo                 domOnboarding.Repository
	eventBus             eventbus.EventBus
	kafkaProducer        kafka.Producer
	topicTenantProvision string
}

func NewService(
	repo domOnboarding.Repository,
	eventBus eventbus.EventBus,
	kafkaProducer kafka.Producer,
	topicTenantProvision string,
) Service {
	if topicTenantProvision == "" {
		topicTenantProvision = "auth.onboarding.completed"
	}
	return &service{
		repo:                 repo,
		eventBus:             eventBus,
		kafkaProducer:        kafkaProducer,
		topicTenantProvision: topicTenantProvision,
	}
}

func (s *service) SaveProfile(ctx context.Context, userID uuid.UUID, userEmail string, req ProfileRequest) (*OnboardingState, error) {
	now := time.Now().UTC()

	// 1. Save or Update UserProfile in samvada_auth
	profile, err := s.repo.GetProfileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if profile == nil {
		profile = &domOnboarding.UserProfile{
			ID:                  uuid.New(),
			UserID:              userID,
			FirstName:           req.FirstName,
			LastName:            req.LastName,
			AccountType:         req.AccountType,
			Industries:          []string{},
			OnboardingCompleted: false,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := s.repo.CreateProfile(ctx, profile); err != nil {
			return nil, err
		}
	} else {
		profile.FirstName = req.FirstName
		profile.LastName = req.LastName
		profile.AccountType = req.AccountType
		profile.UpdatedAt = now
		if err := s.repo.UpdateProfile(ctx, profile); err != nil {
			return nil, err
		}
	}

	// 2. Save or Retrieve Organization in samvada_auth
	orgName := req.OrgName
	if orgName == "" {
		if req.FirstName != "" {
			orgName = req.FirstName + "'s Organization"
		} else {
			orgName = "Personal Organization"
		}
	}

	org, _ := s.repo.GetOrganizationByOwnerID(ctx, userID)
	if org == nil {
		org = &domOnboarding.Organization{
			ID:        uuid.New(),
			Name:      orgName,
			OwnerID:   userID,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.repo.CreateOrganization(ctx, org); err != nil {
			return nil, err
		}
	}

	// 3. Save or Retrieve Default Workspace in samvada_auth
	wsName := req.WorkspaceName
	if wsName == "" {
		wsName = "Default workspace"
	}

	ws, _ := s.repo.GetDefaultWorkspaceByOwnerID(ctx, userID)
	if ws == nil {
		var orgID *uuid.UUID
		if org != nil {
			orgID = &org.ID
		}
		ws = &domOnboarding.Workspace{
			ID:        uuid.New(),
			Name:      wsName,
			OrgID:     orgID,
			OwnerID:   userID,
			IsDefault: true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.repo.CreateWorkspace(ctx, ws); err != nil {
			return nil, err
		}
	}

	// 4. Push Event to Kafka for ABS worker to store org, workspace, and owner/admin in samvada_abs
	evPayload := map[string]interface{}{
		"event_type":     "TenantProvisionRequested",
		"user_id":        userID.String(),
		"email":          userEmail,
		"first_name":     req.FirstName,
		"last_name":      req.LastName,
		"account_type":   req.AccountType,
		"org_id":         org.ID.String(),
		"org_name":       org.Name,
		"workspace_id":   ws.ID.String(),
		"workspace_name": ws.Name,
	}

	// 4.1 Persist in outbox for guaranteed transactional delivery
	ev, err := events.NewDomainEvent(
		events.TenantProvisionRequested,
		userID.String(),
		"USER",
		1,
		"",
		evPayload,
	)
	if err == nil && s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, ev)
	}

	// 4.2 Also publish directly to Kafka for zero-latency execution
	if s.kafkaProducer != nil && s.topicTenantProvision != "" {
		payloadBytes, _ := json.Marshal(evPayload)
		_, _, _ = s.kafkaProducer.SendMessage(ctx, s.topicTenantProvision, userID.String(), payloadBytes)
	}

	return &OnboardingState{
		Profile:             profile,
		Organization:        org,
		Workspace:           ws,
		OnboardingCompleted: profile.OnboardingCompleted,
		WalletBalanceUSD:    5.00,
		DemoCreditsUSD:      5.00,
		RemainingMinutes:    83,
	}, nil
}

func (s *service) SaveIndustries(ctx context.Context, userID uuid.UUID, userEmail string, req IndustriesRequest) (*OnboardingState, error) {
	if len(req.Industries) > 3 {
		return nil, errors.New("maximum 3 industries allowed")
	}

	profile, err := s.repo.GetProfileByUserID(ctx, userID)
	if err != nil || profile == nil {
		return nil, errors.New("profile not found, please set up profile first")
	}

	profile.Industries = req.Industries
	profile.OnboardingCompleted = true
	profile.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateProfile(ctx, profile); err != nil {
		return nil, err
	}

	org, _ := s.repo.GetOrganizationByOwnerID(ctx, userID)
	ws, _ := s.repo.GetDefaultWorkspaceByOwnerID(ctx, userID)

	// Publish UserOnboarded event
	evPayload := map[string]interface{}{
		"event_type":           "UserOnboarded",
		"user_id":              userID.String(),
		"email":                userEmail,
		"industries":           req.Industries,
		"onboarding_completed": true,
	}
	ev, err := events.NewDomainEvent(
		events.UserOnboarded,
		userID.String(),
		"USER",
		1,
		"",
		evPayload,
	)
	if err == nil && s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, ev)
	}
	if s.kafkaProducer != nil && s.topicTenantProvision != "" {
		payloadBytes, _ := json.Marshal(evPayload)
		_, _, _ = s.kafkaProducer.SendMessage(ctx, s.topicTenantProvision, userID.String(), payloadBytes)
	}

	return &OnboardingState{
		Profile:             profile,
		Organization:        org,
		Workspace:           ws,
		OnboardingCompleted: true,
		WalletBalanceUSD:    5.00,
		DemoCreditsUSD:      5.00,
		RemainingMinutes:    83,
	}, nil
}

func (s *service) GetOnboardingState(ctx context.Context, userID uuid.UUID) (*OnboardingState, error) {
	profile, err := s.repo.GetProfileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return &OnboardingState{
			OnboardingCompleted: false,
		}, nil
	}

	org, _ := s.repo.GetOrganizationByOwnerID(ctx, userID)
	ws, _ := s.repo.GetDefaultWorkspaceByOwnerID(ctx, userID)

	return &OnboardingState{
		Profile:             profile,
		Organization:        org,
		Workspace:           ws,
		OnboardingCompleted: profile.OnboardingCompleted,
		WalletBalanceUSD:    5.00,
		DemoCreditsUSD:      5.00,
		RemainingMinutes:    83,
	}, nil
}

// =============================================================================
// Samvada Platform — ABS Organization Application Service
// =============================================================================

package organization

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type CreateOrgRequest struct {
	Name          string     `json:"name" binding:"required,min=2,max=100"`
	Slug          string     `json:"slug"`
	WorkspaceName string     `json:"workspace_name"`
	BillingEmail  string     `json:"billing_email" binding:"required,email"`
	UserID        uuid.UUID  `json:"-"`
	UserEmail     string     `json:"-"`
	OrgID         *uuid.UUID `json:"org_id,omitempty"`
	WorkspaceID   *uuid.UUID `json:"workspace_id,omitempty"`
}

type InviteMemberRequest struct {
	OrgID uuid.UUID `json:"org_id"`
	Email string    `json:"email" binding:"required,email"`
	Role  string    `json:"role" binding:"required,oneof=admin member"`
}

type OrgService interface {
	CreateOrganization(ctx context.Context, req CreateOrgRequest) (*domain.Organization, *domain.Workspace, *domain.OrganizationWallet, error)
	GetOrganization(ctx context.Context, id uuid.UUID) (*domain.Organization, error)
	GetOrganizationBySlug(ctx context.Context, slug string) (*domain.Organization, error)
	ListUserOrganizations(ctx context.Context, userID uuid.UUID) ([]*domain.Organization, error)
	UpdateOrganization(ctx context.Context, id uuid.UUID, name string, logoURL *string) (*domain.Organization, error)

	InviteMember(ctx context.Context, req InviteMemberRequest) (*domain.OrgMember, error)
	UpdateMemberRole(ctx context.Context, orgID, userID uuid.UUID, newRole string) error
	RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error
	ListMembers(ctx context.Context, orgID uuid.UUID) ([]*domain.OrgMember, error)
}

type orgService struct {
	orgRepo     domain.OrgRepository
	wsRepo      domain.WorkspaceRepository
	billingRepo domain.BillingRepository
	log         *zap.Logger
}

func NewOrgService(
	orgRepo domain.OrgRepository,
	wsRepo domain.WorkspaceRepository,
	billingRepo domain.BillingRepository,
	log *zap.Logger,
) OrgService {
	return &orgService{
		orgRepo:     orgRepo,
		wsRepo:      wsRepo,
		billingRepo: billingRepo,
		log:         log.Named("org_service"),
	}
}

func (s *orgService) CreateOrganization(
	ctx context.Context,
	req CreateOrgRequest,
) (*domain.Organization, *domain.Workspace, *domain.OrganizationWallet, error) {
	// 1 User = 1 Organization Constraint
	existingOrgs, _ := s.orgRepo.ListForUser(ctx, req.UserID)
	if len(existingOrgs) > 0 {
		targetOrg := existingOrgs[0]
		if req.Name != "" && req.Name != targetOrg.Name {
			targetOrg.Name = req.Name
			targetOrg.UpdatedAt = time.Now().UTC()
			_ = s.orgRepo.Update(ctx, targetOrg)
		}
		existingWs, _ := s.wsRepo.ListByOrg(ctx, targetOrg.ID)
		var ws *domain.Workspace
		if len(existingWs) > 0 {
			ws = existingWs[0]
			if req.WorkspaceName != "" && req.WorkspaceName != ws.Name {
				ws.Name = req.WorkspaceName
				ws.UpdatedAt = time.Now().UTC()
				_ = s.wsRepo.Update(ctx, ws)
			}
		} else {
			wsName := req.WorkspaceName
			if wsName == "" {
				wsName = "Default Workspace"
			}
			ws = &domain.Workspace{
				ID:                 uuid.New(),
				OrgID:              targetOrg.ID,
				Name:               wsName,
				Slug:               generateSlug(wsName),
				MonthlyMinuteLimit: 10000,
				CreatedAt:          time.Now().UTC(),
				UpdatedAt:          time.Now().UTC(),
			}
			_ = s.wsRepo.Create(ctx, ws)
		}
		wallet, _ := s.billingRepo.GetWalletByOrgID(ctx, targetOrg.ID)
		return targetOrg, ws, wallet, nil
	}

	slug := req.Slug
	if slug == "" {
		slug = generateSlug(req.Name)
	}

	// 1. Check if slug exists
	if _, err := s.orgRepo.GetBySlug(ctx, slug); err == nil {
		slug = fmt.Sprintf("%s-%d", slug, time.Now().Unix()%10000)
	}

	orgID := uuid.New()
	if req.OrgID != nil && *req.OrgID != uuid.Nil {
		orgID = *req.OrgID
	}
	org := &domain.Organization{
		ID:              orgID,
		Name:            req.Name,
		Slug:            slug,
		BillingCurrency: "USD",
		BillingEmail:    req.BillingEmail,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if err := s.orgRepo.Create(ctx, org); err != nil {
		s.log.Error("Failed to create organization", zap.Error(err))
		return nil, nil, nil, err
	}

	// 2. Add creator as Org Owner
	ownerMember := &domain.OrgMember{
		ID:        uuid.New(),
		OrgID:     orgID,
		UserID:    req.UserID,
		Email:     req.UserEmail,
		Role:      "owner",
		Status:    "active",
		InvitedAt: time.Now().UTC(),
	}
	if err := s.orgRepo.AddMember(ctx, ownerMember); err != nil {
		s.log.Error("Failed to add owner member to organization", zap.Error(err))
		return nil, nil, nil, err
	}

	// 3. Create Default Workspace
	wsName := req.WorkspaceName
	if wsName == "" {
		wsName = "Default Workspace"
	}
	wsID := uuid.New()
	if req.WorkspaceID != nil && *req.WorkspaceID != uuid.Nil {
		wsID = *req.WorkspaceID
	}
	ws := &domain.Workspace{
		ID:                 wsID,
		OrgID:              orgID,
		Name:               wsName,
		Slug:               "default",
		Icon:               "💼",
		IsDefault:          true,
		MonthlyMinuteLimit: 10000,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	if err := s.wsRepo.Create(ctx, ws); err != nil {
		s.log.Error("Failed to create default workspace for organization", zap.Error(err))
		return nil, nil, nil, err
	}

	// 4. Add creator as Workspace Admin
	wsMember := &domain.WorkspaceMember{
		ID:            uuid.New(),
		WorkspaceID:   wsID,
		UserID:        req.UserID,
		WorkspaceRole: "workspace_admin",
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.wsRepo.AddMember(ctx, wsMember); err != nil {
		s.log.Error("Failed to add member to default workspace", zap.Error(err))
		return nil, nil, nil, err
	}

	// 5. Auto-provision Organization Wallet with $5.00 promotional demo credits
	wallet := &domain.OrganizationWallet{
		ID:                     uuid.New(),
		OrgID:                  orgID,
		BalanceUSD:             5.0000,
		PromotionalCreditUSD:   5.0000,
		LifetimeCreditAddedUSD: 5.0000,
		LifetimeSpendUSD:       0.0000,
		Currency:               "USD",
		IsFrozen:               false,
		UpdatedAt:              time.Now().UTC(),
	}
	if err := s.billingRepo.CreateWallet(ctx, wallet); err != nil {
		s.log.Error("Failed to auto-provision wallet with initial demo credits", zap.Error(err))
		return nil, nil, nil, err
	}

	// 6. Auto-provision Free Trial Subscription if 'free' plan exists
	freePlan, err := s.billingRepo.GetPlanByTier(ctx, "free")
	if err == nil && freePlan != nil {
		sub := &domain.Subscription{
			ID:                 uuid.New(),
			OrgID:              orgID,
			PlanID:             freePlan.ID,
			Status:             "active",
			BillingCycle:       "monthly",
			PaymentGateway:     "stripe",
			GatewayCustomerID:  "demo_cust_" + orgID.String()[:8],
			CurrentPeriodStart: time.Now().UTC(),
			CurrentPeriodEnd:   time.Now().UTC().AddDate(0, 1, 0),
			CancelAtPeriodEnd:  false,
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		_ = s.billingRepo.CreateSubscription(ctx, sub)
	}

	s.log.Info("Organization successfully provisioned with wallet and workspace",
		zap.String("org_id", orgID.String()),
		zap.String("org_name", req.Name),
		zap.Float64("demo_credits_usd", wallet.BalanceUSD),
	)

	return org, ws, wallet, nil
}

func (s *orgService) GetOrganization(ctx context.Context, id uuid.UUID) (*domain.Organization, error) {
	return s.orgRepo.GetByID(ctx, id)
}

func (s *orgService) GetOrganizationBySlug(ctx context.Context, slug string) (*domain.Organization, error) {
	return s.orgRepo.GetBySlug(ctx, slug)
}

func (s *orgService) ListUserOrganizations(ctx context.Context, userID uuid.UUID) ([]*domain.Organization, error) {
	return s.orgRepo.ListForUser(ctx, userID)
}

func (s *orgService) UpdateOrganization(ctx context.Context, id uuid.UUID, name string, logoURL *string) (*domain.Organization, error) {
	org, err := s.orgRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if name != "" {
		org.Name = name
	}
	if logoURL != nil {
		org.LogoURL = logoURL
	}
	org.UpdatedAt = time.Now().UTC()
	if err := s.orgRepo.Update(ctx, org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *orgService) InviteMember(ctx context.Context, req InviteMemberRequest) (*domain.OrgMember, error) {
	member := &domain.OrgMember{
		ID:        uuid.New(),
		OrgID:     req.OrgID,
		UserID:    uuid.New(), // Placeholder until user signs up/accepts
		Email:     req.Email,
		Role:      req.Role,
		Status:    "invited",
		InvitedAt: time.Now().UTC(),
	}
	if err := s.orgRepo.AddMember(ctx, member); err != nil {
		return nil, err
	}
	return member, nil
}

func (s *orgService) UpdateMemberRole(ctx context.Context, orgID, userID uuid.UUID, newRole string) error {
	return s.orgRepo.UpdateMemberRole(ctx, orgID, userID, newRole)
}

func (s *orgService) RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error {
	return s.orgRepo.RemoveMember(ctx, orgID, userID)
}

func (s *orgService) ListMembers(ctx context.Context, orgID uuid.UUID) ([]*domain.OrgMember, error) {
	return s.orgRepo.ListMembers(ctx, orgID)
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9]+`)

func generateSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonAlphanumericRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "org"
	}
	return slug
}

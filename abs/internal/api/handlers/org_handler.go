// =============================================================================
// Samvada Platform — ABS Organization & Onboarding HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/api/dto"
	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/application/organization"
	"github.com/samvada/abs/internal/domain"
)

type OrgHandler struct {
	orgService    organization.OrgService
	walletService billing.WalletService
	profileRepo   domain.UserProfileRepository
}

func NewOrgHandler(orgService organization.OrgService, walletService billing.WalletService, profileRepo ...domain.UserProfileRepository) *OrgHandler {
	var pr domain.UserProfileRepository
	if len(profileRepo) > 0 {
		pr = profileRepo[0]
	}
	return &OrgHandler{
		orgService:    orgService,
		walletService: walletService,
		profileRepo:   pr,
	}
}

// Onboarding initializes a new tenant organization with default workspace and $5.00 demo credits
func (h *OrgHandler) Onboarding(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextUserIDKey)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authenticated user session required")
		return
	}
	userEmail := c.GetString(middleware.ContextUserEmailKey)

	var req dto.OnboardingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	org, ws, wallet, err := h.orgService.CreateOrganization(c.Request.Context(), organization.CreateOrgRequest{
		Name:         req.CompanyName,
		Slug:         req.Slug,
		BillingEmail: req.BillingEmail,
		UserID:       userID,
		UserEmail:    userEmail,
	})
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "ONBOARDING_FAILED", err.Error())
		return
	}

	authResult, _ := h.walletService.CheckCallAuthorization(c.Request.Context(), org.ID)
	remainingMin := 83
	if authResult != nil {
		remainingMin = authResult.EstimatedRemainingMinutes
	}

	resp := dto.OnboardingResponse{
		OrganizationID:   org.ID.String(),
		OrganizationName: org.Name,
		OrganizationSlug: org.Slug,
		WorkspaceID:      ws.ID.String(),
		WorkspaceSlug:    ws.Slug,
		Role:             "owner",
		WalletBalanceUSD: wallet.BalanceUSD,
		DemoCreditsUSD:   wallet.PromotionalCreditUSD,
		RemainingMinutes: remainingMin,
		SubscriptionTier: "free",
		CreatedAt:        org.CreatedAt,
	}

	RespondSuccess(c, http.StatusCreated, resp)
}

// SaveProfile handles POST /api/v1/onboarding/profile
func (h *OrgHandler) SaveProfile(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextUserIDKey)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authenticated user session required")
		return
	}
	userEmail := c.GetString(middleware.ContextUserEmailKey)

	var req dto.ProfileSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusUnprocessableEntity, "INVALID_REQUEST", err.Error())
		return
	}

	companyName := req.WorkspaceName
	if req.AccountType == "business" && req.OrgName != nil && *req.OrgName != "" {
		companyName = *req.OrgName
	} else if req.FirstName != "" {
		companyName = req.FirstName + "'s Organization"
	}

	org, ws, wallet, err := h.orgService.CreateOrganization(c.Request.Context(), organization.CreateOrgRequest{
		Name:          companyName,
		WorkspaceName: req.WorkspaceName,
		BillingEmail:  userEmail,
		UserID:        userID,
		UserEmail:     userEmail,
	})
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "ONBOARDING_FAILED", err.Error())
		return
	}

	lastName := ""
	if req.LastName != nil {
		lastName = *req.LastName
	}
	if h.profileRepo != nil {
		_ = h.profileRepo.Upsert(c.Request.Context(), &domain.UserProfile{
			UserID:    userID,
			FirstName: req.FirstName,
			LastName:  lastName,
			Email:     userEmail,
			Company:   companyName,
			UpdatedAt: time.Now().UTC(),
		})
	}

	authResult, _ := h.walletService.CheckCallAuthorization(c.Request.Context(), org.ID)
	remainingMin := 83
	if authResult != nil {
		remainingMin = authResult.EstimatedRemainingMinutes
	}

	resp := dto.OnboardingStateResponse{
		Profile: &dto.UserProfileDTO{
			ID:                  uuid.New().String(),
			UserID:              userID.String(),
			FirstName:           req.FirstName,
			LastName:            req.LastName,
			AccountType:         req.AccountType,
			Industries:          []string{},
			OnboardingCompleted: false,
		},
		Organization: &dto.OrganizationDTO{
			ID:      org.ID.String(),
			Name:    org.Name,
			OwnerID: userID.String(),
		},
		Workspace: &dto.WorkspaceDTO{
			ID:        ws.ID.String(),
			Name:      ws.Name,
			OrgID:     org.ID.String(),
			OwnerID:   userID.String(),
			IsDefault: ws.IsDefault,
		},
		OnboardingCompleted: false,
		WalletBalanceUSD:    wallet.BalanceUSD,
		DemoCreditsUSD:      wallet.PromotionalCreditUSD,
		RemainingMinutes:    remainingMin,
	}

	RespondSuccess(c, http.StatusOK, resp)
}

// SaveIndustries handles POST /api/v1/onboarding/industries
func (h *OrgHandler) SaveIndustries(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextUserIDKey)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authenticated user session required")
		return
	}

	var req dto.IndustrySelectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusUnprocessableEntity, "INVALID_REQUEST", err.Error())
		return
	}

	orgs, err := h.orgService.ListUserOrganizations(c.Request.Context(), userID)
	if err != nil || len(orgs) == 0 {
		RespondError(c, http.StatusBadRequest, "ORG_NOT_FOUND", "Profile setup must be completed before industry selection")
		return
	}

	targetOrg := orgs[0]
	wallet, _ := h.walletService.GetWallet(c.Request.Context(), targetOrg.ID)
	walletBal := 5.0
	promoBal := 5.0
	if wallet != nil {
		walletBal = wallet.BalanceUSD
		promoBal = wallet.PromotionalCreditUSD
	}

	firstName := "Member"
	var lastName *string
	if h.profileRepo != nil {
		if p, _ := h.profileRepo.GetByUserID(c.Request.Context(), userID); p != nil {
			p.OnboardingCompleted = true
			p.UpdatedAt = time.Now().UTC()
			_ = h.profileRepo.Upsert(c.Request.Context(), p)
			if p.FirstName != "" {
				firstName = p.FirstName
			}
			if p.LastName != "" {
				lastName = &p.LastName
			}
		}
	}

	resp := dto.OnboardingStateResponse{
		Profile: &dto.UserProfileDTO{
			ID:                  uuid.New().String(),
			UserID:              userID.String(),
			FirstName:           firstName,
			LastName:            lastName,
			AccountType:         "business",
			Industries:          req.Industries,
			OnboardingCompleted: true,
		},
		Organization: &dto.OrganizationDTO{
			ID:      targetOrg.ID.String(),
			Name:    targetOrg.Name,
			OwnerID: userID.String(),
		},
		OnboardingCompleted: true,
		WalletBalanceUSD:    walletBal,
		DemoCreditsUSD:      promoBal,
		RemainingMinutes:    83,
	}

	RespondSuccess(c, http.StatusOK, resp)
}

// GetState handles GET /api/v1/onboarding/state
func (h *OrgHandler) GetState(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextUserIDKey)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authenticated user session required")
		return
	}

	orgs, err := h.orgService.ListUserOrganizations(c.Request.Context(), userID)
	if err != nil || len(orgs) == 0 {
		RespondSuccess(c, http.StatusOK, dto.OnboardingStateResponse{
			OnboardingCompleted: false,
		})
		return
	}

	targetOrg := orgs[0]
	wallet, _ := h.walletService.GetWallet(c.Request.Context(), targetOrg.ID)
	walletBal := 5.0
	promoBal := 5.0
	if wallet != nil {
		walletBal = wallet.BalanceUSD
		promoBal = wallet.PromotionalCreditUSD
	}

	firstName := "Member"
	var lastName *string
	onboardingCompleted := false
	if h.profileRepo != nil {
		if p, _ := h.profileRepo.GetByUserID(c.Request.Context(), userID); p != nil {
			if p.FirstName != "" {
				firstName = p.FirstName
			}
			if p.LastName != "" {
				lastName = &p.LastName
			}
			onboardingCompleted = p.OnboardingCompleted
		}
	}

	resp := dto.OnboardingStateResponse{
		Profile: &dto.UserProfileDTO{
			ID:                  uuid.New().String(),
			UserID:              userID.String(),
			FirstName:           firstName,
			LastName:            lastName,
			AccountType:         "business",
			Industries:          []string{"Technology & SaaS"},
			OnboardingCompleted: onboardingCompleted,
		},
		Organization: &dto.OrganizationDTO{
			ID:      targetOrg.ID.String(),
			Name:    targetOrg.Name,
			OwnerID: userID.String(),
		},
		OnboardingCompleted: onboardingCompleted,
		WalletBalanceUSD:    walletBal,
		DemoCreditsUSD:      promoBal,
		RemainingMinutes:    83,
	}

	RespondSuccess(c, http.StatusOK, resp)
}

func (h *OrgHandler) ListMyOrgs(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextUserIDKey)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid user session required")
		return
	}

	orgs, err := h.orgService.ListUserOrganizations(c.Request.Context(), userID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondSuccess(c, http.StatusOK, orgs)
}

func (h *OrgHandler) GetOrg(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("org_id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ORG_ID", "Invalid organization UUID format")
		return
	}

	org, err := h.orgService.GetOrganization(c.Request.Context(), orgID)
	if err != nil {
		RespondError(c, http.StatusNotFound, "ORG_NOT_FOUND", err.Error())
		return
	}
	RespondSuccess(c, http.StatusOK, org)
}

func (h *OrgHandler) ListMembers(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("org_id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ORG_ID", "Invalid organization UUID format")
		return
	}

	members, err := h.orgService.ListMembers(c.Request.Context(), orgID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondSuccess(c, http.StatusOK, members)
}

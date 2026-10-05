// =============================================================================
// Samvada Platform — ABS API Data Transfer Objects (DTO)
// =============================================================================

package dto

import "time"

type APIResponse struct {
	Success       bool        `json:"success"`
	Data          interface{} `json:"data,omitempty"`
	Error         *APIError   `json:"error,omitempty"`
	CorrelationID string      `json:"correlation_id,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Onboarding & Organization DTOs
type OnboardingRequest struct {
	CompanyName  string   `json:"company_name" binding:"required,min=2,max=100"`
	Slug         string   `json:"slug"`
	BillingEmail string   `json:"billing_email" binding:"required,email"`
	Industries   []string `json:"industries"`
	UseCases     []string `json:"use_cases"`
}

type OnboardingResponse struct {
	OrganizationID    string    `json:"organization_id"`
	OrganizationName  string    `json:"organization_name"`
	OrganizationSlug  string    `json:"organization_slug"`
	WorkspaceID       string    `json:"workspace_id"`
	WorkspaceSlug     string    `json:"workspace_slug"`
	Role              string    `json:"role"`
	WalletBalanceUSD  float64   `json:"wallet_balance_usd"`
	DemoCreditsUSD    float64   `json:"demo_credits_usd"`
	RemainingMinutes  int       `json:"remaining_minutes"`
	SubscriptionTier  string    `json:"subscription_tier"`
	CreatedAt         time.Time `json:"created_at"`
}

type CreateWorkspaceRequest struct {
	Name               string  `json:"name" binding:"required,min=2,max=100"`
	Slug               string  `json:"slug"`
	Icon               string  `json:"icon"`
	Description        *string `json:"description,omitempty"`
	MonthlyMinuteLimit int     `json:"monthly_minute_limit"`
}

type CreateAgentRequest struct {
	Name          string  `json:"name" binding:"required,min=2,max=100"`
	Role          string  `json:"role" binding:"required"`
	Description   *string `json:"description,omitempty"`
	SystemPrompt  string  `json:"system_prompt" binding:"required"`
	VoiceID       string  `json:"voice_id" binding:"required"`
	VoiceProvider string  `json:"voice_provider"`
	VoiceName     string  `json:"voice_name"`
	Language      string  `json:"language"`
	Model         string  `json:"model"`
	AssignedPhone *string `json:"assigned_phone,omitempty"`
}

type TopUpRequest struct {
	AmountUSD float64 `json:"amount_usd" binding:"required,gt=0"`
	Reference string  `json:"reference" binding:"required"`
}

// Frontend-Compatible Onboarding Wizard DTOs
type ProfileSetupRequest struct {
	FirstName     string  `json:"firstName" binding:"required"`
	LastName      *string `json:"lastName,omitempty"`
	AccountType   string  `json:"accountType" binding:"required,oneof=business individual"`
	OrgName       *string `json:"orgName,omitempty"`
	WorkspaceName string  `json:"workspaceName" binding:"required"`
}

type IndustrySelectionRequest struct {
	Industries []string `json:"industries" binding:"required,min=1,max=3"`
}

type UserProfileDTO struct {
	ID                  string   `json:"id"`
	UserID              string   `json:"userId"`
	FirstName           string   `json:"firstName"`
	LastName            *string  `json:"lastName,omitempty"`
	AccountType         string   `json:"accountType"`
	Industries          []string `json:"industries"`
	OnboardingCompleted bool     `json:"onboardingCompleted"`
}

type OrganizationDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OwnerID string `json:"ownerId"`
}

type WorkspaceDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	OrgID     string `json:"orgId"`
	OwnerID   string `json:"ownerId"`
	IsDefault bool   `json:"isDefault"`
}

type OnboardingStateResponse struct {
	Profile             *UserProfileDTO  `json:"profile,omitempty"`
	Organization        *OrganizationDTO `json:"organization,omitempty"`
	Workspace           *WorkspaceDTO    `json:"workspace,omitempty"`
	OnboardingCompleted bool             `json:"onboardingCompleted"`
	WalletBalanceUSD    float64          `json:"wallet_balance_usd,omitempty"`
	DemoCreditsUSD      float64          `json:"demo_credits_usd,omitempty"`
	RemainingMinutes    int              `json:"remaining_minutes,omitempty"`
}

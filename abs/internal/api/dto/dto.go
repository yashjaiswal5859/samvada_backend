// =============================================================================
// Samvada Platform — ABS API Data Transfer Objects (DTO)
// =============================================================================

package dto

import (
	"time"

	"github.com/samvada/abs/internal/domain"
)

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
	Name                  string                 `json:"name"`
	Role                  string                 `json:"role"`
	Description           *string                `json:"description,omitempty"`
	SystemPrompt          string                 `json:"system_prompt"`
	SystemPromptAlt       string                 `json:"systemPrompt,omitempty"`
	VoiceID               string                 `json:"voice_id"`
	VoiceIDAlt            string                 `json:"voiceId,omitempty"`
	VoiceProvider         string                 `json:"voice_provider"`
	VoiceProviderAlt      string                 `json:"voiceProvider,omitempty"`
	VoiceName             string                 `json:"voice_name"`
	VoiceNameAlt          string                 `json:"voiceName,omitempty"`
	Language              string                 `json:"language"`
	Model                 string                 `json:"model"`
	Status                string                 `json:"status,omitempty"`
	AssignedPhone         *string                `json:"assigned_phone,omitempty"`
	PhoneNumberID         *string                `json:"phoneNumberId,omitempty"`
	PhoneNumberIDAlt      *string                `json:"phone_number_id,omitempty"`
	KnowledgeBaseIDs      []string               `json:"knowledgeBaseIds,omitempty"`
	KnowledgeBaseIDsAlt   []string               `json:"knowledge_base_ids,omitempty"`
	ToolAllowlist         []string               `json:"toolAllowlist,omitempty"`
	EscalationPhoneNumber *string                `json:"escalationPhoneNumber,omitempty"`
	GreetingText          *string                `json:"greetingText,omitempty"`
	InboundGreeting       *string                `json:"inboundGreeting,omitempty"`
	OutboundGreeting      *string                `json:"outboundGreeting,omitempty"`
	GreetingMode          *string                `json:"greetingMode,omitempty"`
	CallDirection         *string                `json:"callDirection,omitempty"`
	PrimaryLanguage       *string                `json:"primaryLanguage,omitempty"`
	AdditionalLanguages   []string               `json:"additionalLanguages,omitempty"`
	SupportedLanguages    []string               `json:"supportedLanguages,omitempty"`
	TelephonyProvider     *string                `json:"telephonyProvider,omitempty"`
	STTProvider           *string                `json:"sttProvider,omitempty"`
	TTSProvider           *string                `json:"ttsProvider,omitempty"`
	STTKeyterms           []string               `json:"sttKeyterms,omitempty"`
	SpeakingRate          *float64               `json:"speakingRate,omitempty"`
	PauseBeforeSpeakingMs *int                   `json:"pauseBeforeSpeakingMs,omitempty"`
	AllowInterruptions    *bool                  `json:"allowInterruptions,omitempty"`
	SilenceBehavior       *string                `json:"silenceBehavior,omitempty"`
	SilenceTimeoutSeconds *int                   `json:"silenceTimeoutSeconds,omitempty"`
	VoiceBehavior         interface{}            `json:"voiceBehavior,omitempty"`
	Guardrails            interface{}            `json:"guardrails,omitempty"`
	ComplianceFlags       interface{}            `json:"complianceFlags,omitempty"`
}

type UpdateAgentRequest struct {
	Name                  *string     `json:"name,omitempty"`
	Role                  *string     `json:"role,omitempty"`
	Description           *string     `json:"description,omitempty"`
	AvatarURL             *string     `json:"avatar_url,omitempty"`
	AvatarURLAlt          *string     `json:"avatar,omitempty"`
	SystemPrompt          *string     `json:"system_prompt,omitempty"`
	SystemPromptAlt       *string     `json:"systemPrompt,omitempty"`
	VoiceID               *string     `json:"voice_id,omitempty"`
	VoiceIDAlt            *string     `json:"voiceId,omitempty"`
	VoiceProvider         *string     `json:"voice_provider,omitempty"`
	VoiceProviderAlt      *string     `json:"voiceProvider,omitempty"`
	VoiceName             *string     `json:"voice_name,omitempty"`
	VoiceNameAlt          *string     `json:"voiceName,omitempty"`
	Language              *string     `json:"language,omitempty"`
	Model                 *string     `json:"model,omitempty"`
	Status                *string     `json:"status,omitempty"`
	AssignedPhone         *string     `json:"assigned_phone,omitempty"`
	PhoneNumber           *string     `json:"phoneNumber,omitempty"`
	PhoneNumberID         *string     `json:"phoneNumberId,omitempty"`
	PhoneNumberIDAlt      *string     `json:"phone_number_id,omitempty"`
	KnowledgeBaseIDs      []string    `json:"knowledgeBaseIds,omitempty"`
	KnowledgeBaseIDsAlt   []string    `json:"knowledge_base_ids,omitempty"`
	ToolAllowlist         []string    `json:"toolAllowlist,omitempty"`
	EscalationPhoneNumber *string     `json:"escalationPhoneNumber,omitempty"`
	GreetingText          *string     `json:"greetingText,omitempty"`
	InboundGreeting       *string     `json:"inboundGreeting,omitempty"`
	OutboundGreeting      *string     `json:"outboundGreeting,omitempty"`
	GreetingMode          *string     `json:"greetingMode,omitempty"`
	CallDirection         *string     `json:"callDirection,omitempty"`
	PrimaryLanguage       *string     `json:"primaryLanguage,omitempty"`
	AdditionalLanguages   []string    `json:"additionalLanguages,omitempty"`
	SupportedLanguages    []string    `json:"supportedLanguages,omitempty"`
	TelephonyProvider     *string     `json:"telephonyProvider,omitempty"`
	STTProvider           *string     `json:"sttProvider,omitempty"`
	TTSProvider           *string     `json:"ttsProvider,omitempty"`
	STTKeyterms           []string    `json:"sttKeyterms,omitempty"`
	SpeakingRate          *float64    `json:"speakingRate,omitempty"`
	PauseBeforeSpeakingMs *int        `json:"pauseBeforeSpeakingMs,omitempty"`
	AllowInterruptions    *bool       `json:"allowInterruptions,omitempty"`
	SilenceBehavior       *string     `json:"silenceBehavior,omitempty"`
	SilenceTimeoutSeconds *int        `json:"silenceTimeoutSeconds,omitempty"`
	VoiceBehavior         interface{} `json:"voiceBehavior,omitempty"`
	Guardrails            interface{} `json:"guardrails,omitempty"`
	ComplianceFlags       interface{} `json:"complianceFlags,omitempty"`
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

// AgentBuilderPhoneNumbers groups available and assigned numbers for agent allocation
type AgentBuilderPhoneNumbers struct {
	Available []*domain.PhoneNumber `json:"available"`
	Assigned  []*domain.PhoneNumber `json:"assigned"`
}

// AgentBuilderResourcesResponse consolidates all resources needed to build or configure an agent
type AgentBuilderResourcesResponse struct {
	PhoneNumbers   AgentBuilderPhoneNumbers      `json:"phone_numbers"`
	STT            []*domain.AIModel             `json:"stt"`
	TTS            []*domain.AIModel             `json:"tts"`
	LLM            []*domain.AIModel             `json:"llm"`
	KnowledgeBases []*domain.KnowledgeBase       `json:"knowledge_bases"`
	Integrations   []domain.ConnectorIntegration `json:"integrations"`
}



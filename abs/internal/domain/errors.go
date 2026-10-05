// =============================================================================
// Samvada Platform — ABS Domain Errors
// =============================================================================

package domain

import (
	"errors"
	"fmt"
)

var (
	// Organization & Workspace
	ErrOrgNotFound          = errors.New("organization not found")
	ErrOrgSlugExists        = errors.New("organization slug already exists")
	ErrWorkspaceNotFound    = errors.New("workspace not found")
	ErrWorkspaceSlugExists  = errors.New("workspace slug already exists in this organization")
	ErrWorkspaceForbidden   = errors.New("access denied for this workspace")
	ErrOrgMemberNotFound    = errors.New("organization member not found")
	ErrWorkspaceMemberNotFound = errors.New("workspace member not found")

	// Agent & Versions
	ErrAgentNotFound        = errors.New("agent not found")
	ErrAgentVersionNotFound = errors.New("agent version not found")
	ErrAgentLimitExceeded   = errors.New("agent creation limit exceeded for subscription plan")

	// Billing & Credits
	ErrWalletNotFound          = errors.New("organization wallet not found")
	ErrWalletFrozen            = errors.New("wallet is frozen due to administrative hold or compliance lock")
	ErrInsufficientCredits     = errors.New("insufficient wallet balance for this operation")
	ErrDemoCreditsExhausted    = errors.New("initial demo credits of $5.00 have been exhausted, recharge required")
	ErrSubscriptionNotFound    = errors.New("subscription not found")
	ErrPlanNotFound            = errors.New("plan not found")
	ErrPaymentMethodNotFound   = errors.New("payment method not found")
	ErrInvoiceNotFound         = errors.New("invoice not found")

	// Knowledge Base & Docs
	ErrKBNotFound       = errors.New("knowledge base not found")
	ErrKBDocNotFound    = errors.New("knowledge base document not found")

	// Campaigns & Telephony
	ErrCampaignNotFound    = errors.New("campaign not found")
	ErrAudienceNotFound    = errors.New("audience not found")
	ErrPhoneNumberNotFound = errors.New("phone number not found")
	ErrPhoneNumberAssigned = errors.New("phone number is already assigned to another agent")

	// API Keys & Webhooks
	ErrAPIKeyNotFound    = errors.New("api key not found")
	ErrWebhookNotFound   = errors.New("webhook not found")

	// Concurrency & Quotas
	ErrConcurrencyLimitExceeded = errors.New("maximum concurrent calls exceeded for workspace")
)

// DomainError represents an enterprise domain validation error with canonical code
type DomainError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Err
}

func NewDomainError(code, message string, err error) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

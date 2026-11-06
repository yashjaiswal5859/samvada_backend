// =============================================================================
// Samvada Voice AI Platform — Canonical Kafka Topics & Event Contracts
// =============================================================================

package contracts

// Canonical Kafka Topic Names
const (
	// Speech-to-Speech Realtime Engine Topics
	TopicCallStarted   = "s2s.call.started"
	TopicCallTurns     = "s2s.call.turns"
	TopicCallCompleted = "s2s.call.completed"

	// Core ABS Application & Billing Topics
	TopicCampaignDispatch = "abs.campaign.dispatch"
	TopicWebhookEvent     = "abs.webhook.event"
	TopicBillingDeduction = "abs.billing.deducted"

	// Auth & Identity Topics
	TopicEmailSend       = "auth.email.send"
	TopicTenantProvision = "auth.onboarding.completed"
	TopicUserRegistered  = "auth.user.registered"
	TopicUserLoggedIn    = "auth.user.logged_in"
	TopicUserLoggedOut   = "auth.user.logged_out"

	// Knowledge Base & RAG Ingestion Topics
	TopicKBDocIngest = "kb.doc.ingest"
)

// Standard HTTP / gRPC Context Header Keys
const (
	HeaderWorkspaceID   = "X-Workspace-ID"
	HeaderOrgID         = "X-Org-ID"
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderRequestID     = "X-Request-ID"
	HeaderWebhookSig    = "X-Samvada-Signature"
)

// Pagination Bounds
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)


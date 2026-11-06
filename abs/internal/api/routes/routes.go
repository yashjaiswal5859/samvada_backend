// =============================================================================
// Samvada Platform — ABS REST Route Registration
// =============================================================================

package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/samvada/abs/internal/api/handlers"
	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/infrastructure/redis"
)

type RouterDependencies struct {
	AuthMW              *middleware.AuthMiddleware
	WorkspaceMW         *middleware.WorkspaceMiddleware
	ReBACMW             *middleware.ReBACMiddleware
	OrgHandler          *handlers.OrgHandler
	WorkspaceHandler    *handlers.WorkspaceHandler
	AgentHandler        *handlers.AgentHandler
	BillingHandler      *handlers.BillingHandler
	KBHandler           *handlers.KBHandler
	CampaignHandler     *handlers.CampaignHandler
	TelephonyHandler    *handlers.TelephonyHandler
	WebhookHandler      *handlers.WebhookHandler
	ProfileHandler      *handlers.ProfileHandler
	NotificationHandler *handlers.NotificationHandler
	ConnectorHandler    *handlers.ConnectorHandler
	AuditHandler        *handlers.AuditHandler
	CatalogHandler      *handlers.CatalogHandler
	AdminHandler        *handlers.AdminHandler
	AdminAuthMW         *middleware.AdminAuthMiddleware
	RDB                 *redis.Client
}

func RegisterAPIRoutes(r *gin.Engine, deps RouterDependencies) {
	if deps.ReBACMW != nil {
		middleware.SetDefaultReBACMiddleware(deps.ReBACMW)
	}

	// Global Observability & Rate Limiting (Phase 6)
	r.Use(middleware.PrometheusMetrics())
	r.GET("/metrics", middleware.MetricsHandler())
	r.Use(middleware.RateLimit(deps.RDB, 120))
	r.Use(middleware.RequestBodyLimit(middleware.DefaultBodyLimit))

	// Live Swagger UI & OpenAPI 3.0 Documentation
	r.StaticFile("/swagger", "./docs/swagger-ui.html")
	r.StaticFile("/docs", "./docs/swagger-ui.html")
	r.StaticFile("/docs/openapi.yaml", "./docs/openapi.yaml")

	v1 := r.Group("/api/v1")
	{
		// ---------------------------------------------------------------------
		// 0. AI Model Catalog & Phone Number Inventory (Public / Tenant Discovery)
		// ---------------------------------------------------------------------
		if deps.CatalogHandler != nil {
			catalogGroup := v1.Group("/catalog")
			{
				catalogGroup.GET("/models", deps.CatalogHandler.ListModels)
				catalogGroup.GET("/phone-numbers", deps.CatalogHandler.ListPhoneNumbers)
			}
		}

		// ---------------------------------------------------------------------
		// 0.1 Administrator Operations (API Only, Dual-Token & Ory Keto ReBAC)
		// ---------------------------------------------------------------------
		admin := v1.Group("/admin")
		{
			// Admin Login (API POST curl with username & password) -> returns JWT + Refresh Token
			if deps.AdminHandler != nil {
				admin.POST("/login", deps.AdminHandler.Login)
			}

			// Protected Admin Endpoints: Require both Bearer JWT + X-Refresh-Token and Keto ReBAC Admin Role
			var adminProtected *gin.RouterGroup
			if deps.AdminAuthMW != nil {
				adminProtected = admin.Group("", deps.AdminAuthMW.RequireAdminKeto())
			} else {
				adminProtected = admin.Group("", deps.AuthMW.RequireAuth())
			}

			if deps.AdminHandler != nil {
				adminProtected.GET("/me", deps.AdminHandler.Me)
			}

			if deps.CatalogHandler != nil {
				adminCatalog := adminProtected.Group("/catalog")
				{
					// Bulk upload phone numbers in list format
					adminCatalog.POST("/phone-numbers", deps.CatalogHandler.UploadPhoneNumbers)
					adminCatalog.GET("/phone-numbers", deps.CatalogHandler.ListPhoneNumbers)
					adminCatalog.DELETE("/phone-numbers/:id", deps.CatalogHandler.DeletePhoneNumber)

					// Bulk upload AI models in list format
					adminCatalog.POST("/models", deps.CatalogHandler.UploadModels)
					adminCatalog.POST("/stt", deps.CatalogHandler.UploadSTTModels)
					adminCatalog.POST("/tts", deps.CatalogHandler.UploadTTSModels)
					adminCatalog.POST("/llm", deps.CatalogHandler.UploadLLMModels)
					adminCatalog.GET("/models", deps.CatalogHandler.ListModels)
					adminCatalog.DELETE("/models/:id", deps.CatalogHandler.DeleteModel)
				}
			}
		}

		// ---------------------------------------------------------------------
		// 1. Onboarding Pipeline (Authenticated)
		// ---------------------------------------------------------------------
		onboarding := v1.Group("/onboarding")
		onboarding.Use(deps.AuthMW.RequireAuth())
		{
			onboarding.POST("", deps.OrgHandler.Onboarding)
			onboarding.POST("/profile", deps.OrgHandler.SaveProfile)
			onboarding.POST("/industries", deps.OrgHandler.SaveIndustries)
			onboarding.GET("/state", deps.OrgHandler.GetState)
		}

		// ---------------------------------------------------------------------
		// 2. Organizations
		// ---------------------------------------------------------------------
		orgs := v1.Group("/orgs")
		orgs.Use(deps.AuthMW.RequireAuth())
		{
			orgs.GET("", deps.OrgHandler.ListMyOrgs)
			orgs.GET("/:org_id", deps.OrgHandler.GetOrg)
			orgs.GET("/:org_id/members", deps.OrgHandler.ListMembers)
		}

		// ---------------------------------------------------------------------
		// 3. Workspaces Management
		// ---------------------------------------------------------------------
		workspaces := v1.Group("/workspaces")
		workspaces.Use(deps.AuthMW.RequireAuth())
		{
			workspaces.POST("", deps.WorkspaceHandler.CreateWorkspace)
			workspaces.GET("", deps.WorkspaceHandler.ListMyWorkspaces)
			workspaces.GET("/:workspace_id", deps.WorkspaceHandler.GetWorkspace)
			workspaces.GET("/:workspace_id/members", deps.WorkspaceHandler.ListMembers)
		}

		// ---------------------------------------------------------------------
		// 4. User Profile, Account Settings & GDPR (Category 18 & Phase 6.4)
		// ---------------------------------------------------------------------
		if deps.ProfileHandler != nil {
			user := v1.Group("/user")
			user.Use(deps.AuthMW.RequireAuth())
			{
				user.GET("/me", deps.ProfileHandler.GetMe)
				user.GET("/profile", deps.ProfileHandler.GetProfile)
				user.PATCH("/profile", deps.ProfileHandler.UpdateProfile)
				user.POST("/profile/avatar", middleware.RequestBodyLimit(middleware.UploadBodyLimit), deps.ProfileHandler.UploadAvatar)
				user.GET("/data-export", deps.ProfileHandler.ExportData)
				user.DELETE("/data-deletion", deps.ProfileHandler.DeleteData)
			}
		}

		// ---------------------------------------------------------------------
		// 5. Tenant-Scoped Workspace Resources
		//    Both Hierarchical (/workspaces/:workspace_id/*) and
		//    Direct Header-based (/agents, /billing, /kb, etc. via X-Workspace-ID)
		// ---------------------------------------------------------------------
		scopedGroups := []*gin.RouterGroup{
			v1.Group("/workspaces/:workspace_id", deps.AuthMW.RequireAuth(), deps.WorkspaceMW.RequireWorkspace()),
			v1.Group("", deps.AuthMW.RequireAuth(), deps.WorkspaceMW.RequireWorkspace()),
		}

		for _, g := range scopedGroups {
			// 5.1 Agents
			agents := g.Group("/agents")
			{
				agents.POST("", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.AgentHandler.CreateAgent)
				agents.GET("", deps.AgentHandler.ListAgents)
				agents.GET("/builder-resources", deps.AgentHandler.GetBuilderResources)
				agents.GET("/:id", deps.AgentHandler.GetAgent)
				agents.PATCH("/:id", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.AgentHandler.UpdateAgent)
				agents.PUT("/:id", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.AgentHandler.UpdateAgent)
				agents.POST("/:id/publish", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.AgentHandler.PublishVersion)
				agents.GET("/:id/versions", deps.AgentHandler.ListVersions)
				agents.POST("/:id/rollback", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.AgentHandler.RollbackVersion)
				agents.POST("/:id/test-token", deps.AgentHandler.MintTestToken)
			}

			// 5.2 Billing & Wallet
			billing := g.Group("/billing")
			{
				billing.GET("/wallet", deps.BillingHandler.GetWallet)
				billing.POST("/wallet/topup", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.BillingHandler.TopUp)
				billing.GET("/wallet/transactions", deps.BillingHandler.ListTransactions)
				billing.GET("/plans", deps.BillingHandler.ListPlans)
				billing.GET("/subscription", deps.BillingHandler.GetSubscription)
				billing.POST("/subscribe", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.BillingHandler.Subscribe)
				billing.POST("/subscription/cancel", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.BillingHandler.CancelSubscription)
				billing.GET("/payment-methods", deps.BillingHandler.ListPaymentMethods)
				billing.POST("/payment-methods", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.BillingHandler.AddPaymentMethod)
				billing.DELETE("/payment-methods/:id", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.BillingHandler.DeletePaymentMethod)
				billing.POST("/payment-methods/:id/default", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.BillingHandler.SetDefaultPaymentMethod)
				billing.GET("/usage", deps.BillingHandler.GetUsage)
				billing.GET("/invoices", deps.BillingHandler.ListInvoices)
				billing.POST("/invoices/:id/charge", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.BillingHandler.ChargeInvoice)
			}

			// 5.3 Knowledge Base
			kb := g.Group("/kb")
			{
				kb.POST("", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.KBHandler.CreateKB)
				kb.GET("", deps.KBHandler.ListKBs)
				kb.GET("/:id", deps.KBHandler.GetKB)
				kb.DELETE("/:id", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.KBHandler.DeleteKB)
				kb.POST("/:id/documents", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), middleware.RequestBodyLimit(middleware.UploadBodyLimit), deps.KBHandler.AddDocument)
				kb.POST("/:id/documents/upload", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), middleware.RequestBodyLimit(middleware.UploadBodyLimit), deps.KBHandler.UploadDocument)
				kb.POST("/:id/documents/presign-upload", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.KBHandler.PresignUpload)
				kb.POST("/:id/documents/complete", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.KBHandler.CompleteUpload)
				kb.PUT("/:id/note", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.KBHandler.SaveNote)
				kb.GET("/:id/note", deps.KBHandler.GetNote)
				kb.GET("/:id/documents", deps.KBHandler.ListDocuments)
				kb.DELETE("/:id/documents/:doc_id", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.KBHandler.DeleteDocument)
				kb.POST("/search", deps.KBHandler.SearchKB)
				kb.POST("/:id/sync", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.KBHandler.SyncKB)
			}

			// 5.4 Campaigns & Audiences
			audiences := g.Group("/audiences")
			{
				audiences.POST("", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.CampaignHandler.CreateAudience)
				audiences.GET("", deps.CampaignHandler.ListAudiences)
				audiences.DELETE("/:id", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.CampaignHandler.DeleteAudience)
				audiences.POST("/:id/contacts", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.CampaignHandler.ImportContacts)
				audiences.POST("/:id/batch-import", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), middleware.RequestBodyLimit(middleware.UploadBodyLimit), deps.CampaignHandler.BatchImportCSV)
				audiences.GET("/:id/contacts", deps.CampaignHandler.ListContacts)
				audiences.DELETE("/:id/contacts/:contact_id", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.CampaignHandler.DeleteContact)
			}

			campaigns := g.Group("/campaigns")
			{
				campaigns.POST("", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.CampaignHandler.CreateCampaign)
				campaigns.GET("", deps.CampaignHandler.ListCampaigns)
				campaigns.POST("/:id/status", middleware.RequireMinWorkspaceRole(middleware.RoleOperator), deps.CampaignHandler.UpdateStatus)
			}

			// 5.5 Telephony & Phone Numbers
			telephony := g.Group("/phone-numbers")
			{
				telephony.GET("", deps.TelephonyHandler.ListNumbers)
				telephony.GET("/available", deps.TelephonyHandler.SearchAvailableNumbers)
				telephony.POST("/purchase", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.TelephonyHandler.PurchaseNumber)
				telephony.POST("", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.TelephonyHandler.ProvisionNumber)
				telephony.POST("/:id/assign", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.TelephonyHandler.AssignAgent)
				telephony.POST("/:id/unassign", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.TelephonyHandler.UnassignAgent)
				telephony.DELETE("/:id", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.TelephonyHandler.ReleaseNumber)
			}

			// 5.6 Developer API Keys
			apiKeys := g.Group("/api-keys", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin))
			{
				apiKeys.POST("", deps.WebhookHandler.CreateAPIKey)
				apiKeys.GET("", deps.WebhookHandler.ListAPIKeys)
				apiKeys.DELETE("/:id", deps.WebhookHandler.RevokeAPIKey)
			}

			// 5.7 Webhooks
			webhooks := g.Group("/webhooks", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin))
			{
				webhooks.POST("", deps.WebhookHandler.CreateWebhook)
				webhooks.GET("", deps.WebhookHandler.ListWebhooks)
				webhooks.DELETE("/:id", deps.WebhookHandler.DeleteWebhook)
			}

			// 5.8 Notification Preferences (Category 17)
			if deps.NotificationHandler != nil {
				notifs := g.Group("/notifications")
				{
					notifs.GET("/preferences", deps.NotificationHandler.GetPreferences)
					notifs.PUT("/preferences", deps.NotificationHandler.UpdatePreferences)
					notifs.POST("/test", deps.NotificationHandler.SendTest)
				}
			}

			// 5.9 CRM / Automation Integrations (Category 14)
			if deps.ConnectorHandler != nil {
				integrations := g.Group("/integrations")
				{
					integrations.GET("", deps.ConnectorHandler.ListIntegrations)
					integrations.POST("/:id/toggle", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.ConnectorHandler.ToggleIntegration)
					integrations.POST("/:id/credentials", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin), deps.ConnectorHandler.SaveCredentials)
				}
			}

			// 5.10 Audit Logs (Phase 6.4)
			if deps.AuditHandler != nil {
				auditLogs := g.Group("/audit-logs", middleware.RequireMinWorkspaceRole(middleware.RoleAdmin))
				{
					auditLogs.GET("", deps.AuditHandler.ListLogs)
				}
			}
		}

		// ---------------------------------------------------------------------
		// 6. Inbound Payment Gateway Webhooks (Unauthenticated, Signature-verified)
		// ---------------------------------------------------------------------
		paymentWebhooks := v1.Group("/billing/webhooks")
		{
			paymentWebhooks.POST("/stripe", deps.BillingHandler.HandleStripeWebhook)
			paymentWebhooks.POST("/razorpay", deps.BillingHandler.HandleRazorpayWebhook)
		}
	}
}

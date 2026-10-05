// =============================================================================
// Samvada Platform — ABS Billing, Subscriptions & Invoicing HTTP Handler (Phase 4)
// =============================================================================
// Package: handlers
//
// Handles:
//   - Organization Wallets & Credit Top-ups
//   - Pricing Plans & Subscription Tier Upgrades/Downgrades
//   - Payment Method Tokenization (Stripe & Razorpay)
//   - Quota Usage & Real-Time Metering Summaries
//   - Invoice Generation, Retries & Historical PDFs
//   - Inbound Gateway Webhooks (Stripe / Razorpay)
// =============================================================================

package handlers

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/api/dto"
	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/application/workspace"
)

type BillingHandler struct {
	walletService   billing.WalletService
	wsService       workspace.WorkspaceService
	subService      billing.SubscriptionService
	meteringService billing.UsageMeteringService
	invoiceService  billing.InvoiceService
}

func NewBillingHandler(
	walletService billing.WalletService,
	wsService workspace.WorkspaceService,
	subService billing.SubscriptionService,
	meteringService billing.UsageMeteringService,
	invoiceService billing.InvoiceService,
) *BillingHandler {
	return &BillingHandler{
		walletService:   walletService,
		wsService:       wsService,
		subService:      subService,
		meteringService: meteringService,
		invoiceService:  invoiceService,
	}
}

func (h *BillingHandler) getWorkspaceOrg(c *gin.Context) (uuid.UUID, error) {
	wsIDStr := middleware.GetWorkspaceID(c)
	if wsIDStr == "" {
		wsIDStr = c.Param("workspace_id")
	}
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		return uuid.Nil, err
	}

	ws, err := h.wsService.GetWorkspace(c.Request.Context(), wsID)
	if err != nil {
		return uuid.Nil, err
	}
	return ws.OrgID, nil
}

// -----------------------------------------------------------------------------
// 1. Wallets & Top-up
// -----------------------------------------------------------------------------

func (h *BillingHandler) GetWallet(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	wallet, err := h.walletService.GetWallet(c.Request.Context(), orgID)
	if err != nil {
		RespondError(c, http.StatusNotFound, "WALLET_NOT_FOUND", err.Error())
		return
	}

	authResult, _ := h.walletService.CheckCallAuthorization(c.Request.Context(), orgID)

	RespondSuccess(c, http.StatusOK, gin.H{
		"wallet":                     wallet,
		"call_authorization":         authResult,
		"initial_promotional_credit": wallet.PromotionalCreditUSD,
	})
}

func (h *BillingHandler) TopUp(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	var req dto.TopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	wallet, tx, err := h.walletService.TopUpWallet(c.Request.Context(), orgID, req.AmountUSD, req.Reference)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "TOPUP_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{
		"wallet":      wallet,
		"transaction": tx,
	})
}

func (h *BillingHandler) ListTransactions(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	txs, count, err := h.walletService.ListTransactions(c.Request.Context(), orgID, limit, offset)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{
		"transactions": txs,
		"total_count":  count,
	})
}

// -----------------------------------------------------------------------------
// 2. Plans & Subscriptions
// -----------------------------------------------------------------------------

func (h *BillingHandler) ListPlans(c *gin.Context) {
	if h.subService == nil {
		RespondSuccess(c, http.StatusOK, []interface{}{})
		return
	}

	plans, err := h.subService.ListPlans(c.Request.Context())
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "PLANS_LOOKUP_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, plans)
}

func (h *BillingHandler) GetSubscription(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	if h.subService == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "Subscription service not configured")
		return
	}

	sub, err := h.subService.GetSubscription(c.Request.Context(), orgID)
	if err != nil {
		RespondError(c, http.StatusNotFound, "SUBSCRIPTION_NOT_FOUND", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, sub)
}

func (h *BillingHandler) Subscribe(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	var req billing.SubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	sub, err := h.subService.Subscribe(c.Request.Context(), orgID, req)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "SUBSCRIBE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, sub)
}

func (h *BillingHandler) CancelSubscription(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	immediately := c.Query("immediately") == "true"
	if err := h.subService.CancelSubscription(c.Request.Context(), orgID, immediately); err != nil {
		RespondError(c, http.StatusInternalServerError, "CANCEL_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"canceled": true})
}

// -----------------------------------------------------------------------------
// 3. Payment Methods
// -----------------------------------------------------------------------------

func (h *BillingHandler) AddPaymentMethod(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	var req struct {
		Gateway     string `json:"gateway"`
		Token       string `json:"token" binding:"required"`
		MakeDefault bool   `json:"make_default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	pm, err := h.subService.AddPaymentMethod(c.Request.Context(), orgID, req.Gateway, req.Token, req.MakeDefault)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "PAYMENT_METHOD_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, pm)
}

func (h *BillingHandler) ListPaymentMethods(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	pms, err := h.subService.ListPaymentMethods(c.Request.Context(), orgID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, pms)
}

func (h *BillingHandler) DeletePaymentMethod(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	pmID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_PM_ID", "Invalid payment method UUID")
		return
	}

	if err := h.subService.DeletePaymentMethod(c.Request.Context(), orgID, pmID); err != nil {
		RespondError(c, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *BillingHandler) SetDefaultPaymentMethod(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	pmID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_PM_ID", "Invalid payment method UUID")
		return
	}

	if err := h.subService.SetDefaultPaymentMethod(c.Request.Context(), orgID, pmID); err != nil {
		RespondError(c, http.StatusInternalServerError, "SET_DEFAULT_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"default": true})
}

// -----------------------------------------------------------------------------
// 4. Usage & Quotas
// -----------------------------------------------------------------------------

func (h *BillingHandler) GetUsage(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	if h.meteringService == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "Metering service not configured")
		return
	}

	usage, err := h.meteringService.GetQuotaUsage(c.Request.Context(), orgID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "USAGE_LOOKUP_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, usage)
}

// -----------------------------------------------------------------------------
// 5. Invoices
// -----------------------------------------------------------------------------

func (h *BillingHandler) ListInvoices(c *gin.Context) {
	orgID, err := h.getWorkspaceOrg(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
		return
	}

	var invoices interface{}
	if h.invoiceService != nil {
		invs, err := h.invoiceService.ListInvoices(c.Request.Context(), orgID)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "INVOICE_LIST_FAILED", err.Error())
			return
		}
		invoices = invs
	} else {
		invs, err := h.walletService.ListInvoices(c.Request.Context(), orgID)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		invoices = invs
	}

	RespondSuccess(c, http.StatusOK, invoices)
}

func (h *BillingHandler) ChargeInvoice(c *gin.Context) {
	invID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_INVOICE_ID", "Invalid invoice UUID")
		return
	}

	if h.invoiceService == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "Invoice service not configured")
		return
	}

	if err := h.invoiceService.ChargeInvoice(c.Request.Context(), invID); err != nil {
		RespondError(c, http.StatusInternalServerError, "CHARGE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"charged": true})
}

// -----------------------------------------------------------------------------
// 6. Webhooks
// -----------------------------------------------------------------------------

func (h *BillingHandler) HandleStripeWebhook(c *gin.Context) {
	if h.subService == nil {
		c.Status(http.StatusOK)
		return
	}

	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	sig := c.GetHeader("Stripe-Signature")
	evt, err := h.subService.HandleWebhook(c.Request.Context(), "stripe", payload, sig)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"received": true, "event_id": evt.ID})
}

func (h *BillingHandler) HandleRazorpayWebhook(c *gin.Context) {
	if h.subService == nil {
		c.Status(http.StatusOK)
		return
	}

	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	sig := c.GetHeader("X-Razorpay-Signature")
	evt, err := h.subService.HandleWebhook(c.Request.Context(), "razorpay", payload, sig)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"received": true, "event_id": evt.ID})
}

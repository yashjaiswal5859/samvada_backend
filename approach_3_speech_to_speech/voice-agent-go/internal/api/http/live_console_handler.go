// =============================================================================
// Samvada Platform — Live Call Console & Operator Intervention (Phase 3.3)
// =============================================================================
// Manages real-time active calls, live supervisor monitoring, hold, transfer,
// and force disconnection via Redis state and pub/sub channels.
// =============================================================================

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"voice-agent-go/internal/db"

	"github.com/gin-gonic/gin"
)

type LiveCallSummary struct {
	CallID       string    `json:"call_id"`
	AgentID      string    `json:"agent_id"`
	WorkspaceID  string    `json:"workspace_id"`
	CallerNumber string    `json:"caller_number"`
	CalledNumber string    `json:"called_number"`
	CustomerName string    `json:"customer_name"`
	DurationSec  int64     `json:"duration_sec"`
	Status       string    `json:"status"` // active, hold, transferring
	Sentiment    string    `json:"sentiment"`
	StartedAt    time.Time `json:"started_at"`
}

type LiveConsoleHandler struct{}

func NewLiveConsoleHandler() *LiveConsoleHandler {
	return &LiveConsoleHandler{}
}

// ListLiveCalls returns all active calls currently running in the engine
func (h *LiveConsoleHandler) ListLiveCalls(c *gin.Context) {
	if db.RedisClient == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []LiveCallSummary{}})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	activeIDs, err := db.RedisClient.SMembers(ctx, "live_calls:active").Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var results []LiveCallSummary
	for _, callID := range activeIDs {
		data, err := db.RedisClient.HGetAll(ctx, "live_call:"+callID).Result()
		if err == nil && len(data) > 0 {
			var startTime time.Time
			if t, err := time.Parse(time.RFC3339, data["started_at"]); err == nil {
				startTime = t
			} else {
				startTime = time.Now().UTC()
			}
			duration := int64(time.Since(startTime).Seconds())

			results = append(results, LiveCallSummary{
				CallID:       callID,
				AgentID:      data["agent_id"],
				WorkspaceID:  data["workspace_id"],
				CallerNumber: data["caller_number"],
				CalledNumber: data["called_number"],
				CustomerName: data["customer_name"],
				DurationSec:  duration,
				Status:       data["status"],
				Sentiment:    data["sentiment"],
				StartedAt:    startTime,
			})
		}
	}

	if results == nil {
		results = []LiveCallSummary{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
		"count":   len(results),
	})
}

// TerminateCall sends a force-disconnect signal to the active call session
func (h *LiveConsoleHandler) TerminateCall(c *gin.Context) {
	callID := c.Param("call_id")
	if callID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "call_id required"})
		return
	}

	if db.RedisClient != nil {
		msg, _ := json.Marshal(map[string]string{
			"action":  "terminate",
			"call_id": callID,
		})
		_ = db.RedisClient.Publish(c.Request.Context(), "call:control:"+callID, string(msg)).Err()
		_ = db.RedisClient.HSet(c.Request.Context(), "live_call:"+callID, "status", "terminated").Err()
		_ = db.RedisClient.SRem(c.Request.Context(), "live_calls:active", callID).Err()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Call termination signal dispatched",
		"call_id": callID,
	})
}

// HoldCall toggles hold / resume for an active call
func (h *LiveConsoleHandler) HoldCall(c *gin.Context) {
	callID := c.Param("call_id")
	var req struct {
		Hold bool `json:"hold"`
	}
	_ = c.ShouldBindJSON(&req)

	status := "hold"
	if !req.Hold {
		status = "active"
	}

	if db.RedisClient != nil {
		msg, _ := json.Marshal(map[string]interface{}{
			"action":  "hold",
			"call_id": callID,
			"hold":    req.Hold,
		})
		_ = db.RedisClient.Publish(c.Request.Context(), "call:control:"+callID, string(msg)).Err()
		_ = db.RedisClient.HSet(c.Request.Context(), "live_call:"+callID, "status", status).Err()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"status":  status,
		"call_id": callID,
	})
}

// TransferCall dispatches a warm transfer command to an external human agent or SIP DID
func (h *LiveConsoleHandler) TransferCall(c *gin.Context) {
	callID := c.Param("call_id")
	var req struct {
		TargetPhone string `json:"target_phone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "target_phone required"})
		return
	}

	if db.RedisClient != nil {
		msg, _ := json.Marshal(map[string]string{
			"action":       "transfer",
			"call_id":      callID,
			"target_phone": strings.TrimSpace(req.TargetPhone),
		})
		_ = db.RedisClient.Publish(c.Request.Context(), "call:control:"+callID, string(msg)).Err()
		_ = db.RedisClient.HSet(c.Request.Context(), "live_call:"+callID, "status", "transferring").Err()
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "Warm transfer initiated to " + req.TargetPhone,
		"call_id":      callID,
		"target_phone": req.TargetPhone,
	})
}

// ListenCall provides live audio stream metadata / stream subscription info
func (h *LiveConsoleHandler) ListenCall(c *gin.Context) {
	callID := c.Param("call_id")
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"call_id":    callID,
		"stream_url": "ws://localhost:8000/ws/live-console?call_id=" + callID,
		"mode":       "supervisor_listen",
	})
}

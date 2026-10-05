package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"voice-agent-go/internal/services"
)

type ReportAPIHandler struct {
	reportService *services.ReportService
}

func NewReportAPIHandler(reportService *services.ReportService) *ReportAPIHandler {
	return &ReportAPIHandler{
		reportService: reportService,
	}
}

func (h *ReportAPIHandler) HandleGetReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Use GET.", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) > 0 {
			sessionID = parts[len(parts)-1]
		}
	}

	if sessionID == "" || sessionID == "reports" {
		http.Error(w, "Query parameter 'session_id' or path suffix required.", http.StatusBadRequest)
		return
	}

	report, err := h.reportService.GetReport(sessionID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Report not found: %v", err),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(report)
}

func (h *ReportAPIHandler) HandleGetEscalations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Use GET.", http.StatusMethodNotAllowed)
		return
	}

	escalations := h.reportService.GetPendingEscalations()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "success",
		"total_count": len(escalations),
		"escalations": escalations,
	})
}

func (h *ReportAPIHandler) HandleResolveEscalation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed. Use POST.", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil || id == 0 {
		http.Error(w, "Query parameter 'id' is required.", http.StatusBadRequest)
		return
	}

	success := h.reportService.ResolveEscalation(uint(id))
	w.Header().Set("Content-Type", "application/json")
	if success {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": fmt.Sprintf("Escalation ID %d marked as completed.", id),
		})
	} else {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Escalation ID %d not found.", id),
		})
	}
}

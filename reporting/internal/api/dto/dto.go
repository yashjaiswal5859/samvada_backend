// =============================================================================
// Samvada Platform — Reporting Engine API DTOs
// =============================================================================

package dto

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

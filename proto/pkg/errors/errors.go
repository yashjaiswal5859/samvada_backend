// =============================================================================
// Samvada Platform — Shared Enterprise Error Library
// =============================================================================
// Package: errors
// Module: github.com/samvada/proto/pkg/errors
//
// Description:
//   Standard error codes, HTTP status mappings, gRPC code translations, and
//   structured JSON error envelopes shared across all 4 Samvada microservices:
//     - Auth Service (:8081 REST, :9081 gRPC)
//     - ABS (Application & Billing Service :8082 REST, :9082 gRPC)
//     - Reporting Engine (:8083 REST, :9083 gRPC)
//     - S2S Engine (:8084 WS, :9084 gRPC)
// =============================================================================

package errors

import (
	"fmt"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Standard Error Codes
const (
	CodeInvalidInput       = "INVALID_INPUT"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeForbidden          = "FORBIDDEN"
	CodeNotFound           = "NOT_FOUND"
	CodeConflict           = "CONFLICT"
	CodeQuotaExceeded      = "QUOTA_EXCEEDED"
	CodeWalletExhausted    = "WALLET_EXHAUSTED"
	CodeWorkspaceSuspended = "WORKSPACE_SUSPENDED"
	CodeRateLimited        = "RATE_LIMITED"
	CodeCallUnauthorized   = "CALL_UNAUTHORIZED"
	CodeInternal           = "INTERNAL_SERVER_ERROR"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"

	// Auth-specific error codes — used by JWT middleware across all services
	// to communicate token state to frontend Axios interceptors.
	CodeTokenExpired = "TOKEN_EXPIRED"
	CodeInvalidToken = "INVALID_TOKEN"
	CodeTokenMissing = "TOKEN_MISSING"
)

// AppError is the standard structured error envelope across REST and gRPC boundaries.
type AppError struct {
	Code       string                 `json:"code"`
	Message    string                 `json:"message"`
	HTTPStatus int                    `json:"status"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// Error implements the standard Go error interface.
func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s (HTTP %d)", e.Code, e.Message, e.HTTPStatus)
}

// WithDetail returns a copy of AppError with an additional detail key-value pair.
func (e *AppError) WithDetail(key string, val interface{}) *AppError {
	newDetails := make(map[string]interface{})
	for k, v := range e.Details {
		newDetails[k] = v
	}
	newDetails[key] = val

	return &AppError{
		Code:       e.Code,
		Message:    e.Message,
		HTTPStatus: e.HTTPStatus,
		Details:    newDetails,
	}
}

// New creates a custom AppError.
func New(code, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Details:    make(map[string]interface{}),
	}
}

// Pre-defined canonical domain errors
func ErrInvalidInput(msg string) *AppError {
	return New(CodeInvalidInput, msg, http.StatusBadRequest)
}

func ErrUnauthorized(msg string) *AppError {
	return New(CodeUnauthorized, msg, http.StatusUnauthorized)
}

func ErrForbidden(msg string) *AppError {
	return New(CodeForbidden, msg, http.StatusForbidden)
}

func ErrNotFound(entity string, id string) *AppError {
	return New(CodeNotFound, fmt.Sprintf("%s with id '%s' not found", entity, id), http.StatusNotFound)
}

func ErrConflict(msg string) *AppError {
	return New(CodeConflict, msg, http.StatusConflict)
}

func ErrQuotaExceeded(msg string) *AppError {
	return New(CodeQuotaExceeded, msg, http.StatusTooManyRequests)
}

func ErrWalletExhausted(msg string) *AppError {
	return New(CodeWalletExhausted, msg, http.StatusPaymentRequired)
}

func ErrWorkspaceSuspended(msg string) *AppError {
	return New(CodeWorkspaceSuspended, msg, http.StatusForbidden)
}

func ErrRateLimited(msg string) *AppError {
	return New(CodeRateLimited, msg, http.StatusTooManyRequests)
}

func ErrInternal(msg string) *AppError {
	return New(CodeInternal, msg, http.StatusInternalServerError)
}

func ErrServiceUnavailable(msg string) *AppError {
	return New(CodeServiceUnavailable, msg, http.StatusServiceUnavailable)
}

// ToGRPCStatus converts an AppError to an appropriate gRPC status error.
func (e *AppError) ToGRPCStatus() error {
	var c codes.Code
	switch e.Code {
	case CodeInvalidInput:
		c = codes.InvalidArgument
	case CodeUnauthorized:
		c = codes.Unauthenticated
	case CodeForbidden, CodeWorkspaceSuspended:
		c = codes.PermissionDenied
	case CodeNotFound:
		c = codes.NotFound
	case CodeConflict:
		c = codes.AlreadyExists
	case CodeQuotaExceeded, CodeRateLimited:
		c = codes.ResourceExhausted
	case CodeWalletExhausted:
		c = codes.FailedPrecondition
	case CodeServiceUnavailable:
		c = codes.Unavailable
	default:
		c = codes.Internal
	}
	return status.Error(c, e.Message)
}

// FromGRPCError converts a gRPC error back into a structured AppError.
func FromGRPCError(err error) *AppError {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return ErrInternal(err.Error())
	}

	switch st.Code() {
	case codes.InvalidArgument:
		return ErrInvalidInput(st.Message())
	case codes.Unauthenticated:
		return ErrUnauthorized(st.Message())
	case codes.PermissionDenied:
		return ErrForbidden(st.Message())
	case codes.NotFound:
		return New(CodeNotFound, st.Message(), http.StatusNotFound)
	case codes.AlreadyExists:
		return ErrConflict(st.Message())
	case codes.ResourceExhausted:
		return ErrQuotaExceeded(st.Message())
	case codes.FailedPrecondition:
		return ErrWalletExhausted(st.Message())
	case codes.Unavailable:
		return ErrServiceUnavailable(st.Message())
	default:
		return ErrInternal(st.Message())
	}
}

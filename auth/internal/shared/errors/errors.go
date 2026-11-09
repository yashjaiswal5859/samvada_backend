package errors

import (
	"net/http"
)

type AppError struct {
	Code    int    `json:"-"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func NewAppError(code int, msg string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: msg,
		Err:     err,
	}
}

func BadRequest(msg string, err error) *AppError {
	return NewAppError(http.StatusBadRequest, msg, err)
}

func Unauthorized(msg string, err error) *AppError {
	return NewAppError(http.StatusUnauthorized, msg, err)
}

func Forbidden(msg string, err error) *AppError {
	return NewAppError(http.StatusForbidden, msg, err)
}

func NotFound(msg string, err error) *AppError {
	return NewAppError(http.StatusNotFound, msg, err)
}

func Conflict(msg string, err error) *AppError {
	return NewAppError(http.StatusConflict, msg, err)
}

func Internal(msg string, err error) *AppError {
	return NewAppError(http.StatusInternalServerError, msg, err)
}

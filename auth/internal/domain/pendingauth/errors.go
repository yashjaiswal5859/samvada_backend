package pendingauth

import "errors"

var (
	ErrOTPExpired           = errors.New("OTP has expired")
	ErrOTPInvalid           = errors.New("invalid OTP")
	ErrMaxAttemptsExceeded  = errors.New("maximum OTP verification attempts exceeded")
	ErrSessionNotFound      = errors.New("authentication session not found")
)

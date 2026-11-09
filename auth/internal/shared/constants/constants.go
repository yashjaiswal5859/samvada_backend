package constants

import "time"

const (
	OTPLength        = 6
	OTPDefaultExpiry = 5 * time.Minute
	OTPMaxAttempts   = 5

	AggregateTypeUser = "USER"
	EventVersion      = 1
)

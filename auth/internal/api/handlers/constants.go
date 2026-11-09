package handlers

import "time"

// Authentication Cookie MaxAge Constants (in seconds)
const (
	// RefreshTokenCookieMaxAge is 7 days (604,800 seconds)
	RefreshTokenCookieMaxAge = 7 * 24 * 3600 // 604800

	// AccessTokenDefaultMaxAge is 1 hour (3,600 seconds) if not provided by token response
	AccessTokenDefaultMaxAge = 3600

	// OAuthSessionCookieMaxAge is 1 hour (3,600 seconds)
	OAuthSessionCookieMaxAge = 3600

	// OTPCodeExpiryDuration is 5 minutes
	OTPCodeExpiryDuration = 5 * time.Minute
)

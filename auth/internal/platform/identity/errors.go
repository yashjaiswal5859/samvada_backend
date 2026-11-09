package identity

import "errors"

var (
	ErrIdentityAlreadyExists = errors.New("identity already exists in identity provider")
	ErrIdentityNotFound      = errors.New("identity not found in identity provider")
	ErrInvalidRefreshToken   = errors.New("invalid or expired refresh token")
	ErrPermissionDenied      = errors.New("permission denied")
	ErrOryUnreachable        = errors.New("ory services are unreachable")
)

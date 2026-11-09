package identity

import "context"

type IdentityProvider interface {
	CreateIdentity(ctx context.Context, email string) (string, error)
	GetIdentityByEmail(ctx context.Context, email string) (string, error)
	DeleteIdentity(ctx context.Context, kratosID string) error
}

type ConsentRequest struct {
	Subject        string `json:"subject"`
	LoginChallenge string `json:"login_challenge"`
}

type TokenProvider interface {
	GenerateTokens(ctx context.Context, kratosID string, customClaims map[string]interface{}) (*TokenPair, error)
	RotateTokens(ctx context.Context, refreshToken string) (*TokenPair, error)
	RevokeTokens(ctx context.Context, refreshToken string) error
	AcceptLoginChallenge(ctx context.Context, challenge string, subject string) (string, error)
	AcceptConsentChallenge(ctx context.Context, challenge string, subject string, scopes []string, customClaims map[string]interface{}) (string, error)
	GetConsentRequest(ctx context.Context, challenge string) (*ConsentRequest, error)
	ExchangeCodeForTokens(ctx context.Context, code string, codeVerifier string) (*TokenPair, error)
	IntrospectToken(ctx context.Context, token string) (bool, string, int64, error)
	ValidateIDToken(ctx context.Context, rawJWT string, expectedNonce string, expectedAudience string) (*IDTokenClaims, error)
	GetLoginRequest(ctx context.Context, challenge string) (bool, error)
}

type AuthorizationProvider interface {
	CreateRelationship(ctx context.Context, req RelationshipRequest) error
	CheckPermission(ctx context.Context, req PermissionRequest) (bool, error)
	DeleteRelationship(ctx context.Context, req RelationshipRequest) error
	IsDeviceOwned(ctx context.Context, deviceID string) (bool, error)
	DeleteAllRelationshipsForSubject(ctx context.Context, subject string) error
}

type IdentityOrchestrator interface {
	IdentityProvider
	TokenProvider
	AuthorizationProvider
}

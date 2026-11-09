package identity

import (
	"context"

	"gorm.io/gorm"
)

type orchestrator struct {
	idProvider    IdentityProvider
	tokenProvider TokenProvider
	authProvider  AuthorizationProvider
	db            *gorm.DB
	frontendURL   string
}

func NewIdentityOrchestrator(
	id IdentityProvider,
	token TokenProvider,
	auth AuthorizationProvider,
	db *gorm.DB,
	jwtSecret string, // retained for backward compatibility with DI wiring
	frontendURL string,
) IdentityOrchestrator {
	return &orchestrator{
		idProvider:    id,
		tokenProvider: token,
		authProvider:  auth,
		db:            db,
		frontendURL:   frontendURL,
	}
}

func (o *orchestrator) CreateIdentity(ctx context.Context, email string) (string, error) {
	return o.idProvider.CreateIdentity(ctx, email)
}

func (o *orchestrator) DeleteIdentity(ctx context.Context, kratosID string) error {
	return o.idProvider.DeleteIdentity(ctx, kratosID)
}

func (o *orchestrator) GetIdentityByEmail(ctx context.Context, email string) (string, error) {
	return o.idProvider.GetIdentityByEmail(ctx, email)
}

func (o *orchestrator) GenerateTokens(ctx context.Context, kratosID string, customClaims map[string]interface{}) (*TokenPair, error) {
	return o.tokenProvider.GenerateTokens(ctx, kratosID, customClaims)
}

func (o *orchestrator) RotateTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return o.tokenProvider.RotateTokens(ctx, refreshToken)
}

func (o *orchestrator) RevokeTokens(ctx context.Context, refreshToken string) error {
	return o.tokenProvider.RevokeTokens(ctx, refreshToken)
}

func (o *orchestrator) AcceptLoginChallenge(ctx context.Context, challenge string, subject string) (string, error) {
	return o.tokenProvider.AcceptLoginChallenge(ctx, challenge, subject)
}

func (o *orchestrator) AcceptConsentChallenge(ctx context.Context, challenge string, subject string, scopes []string, customClaims map[string]interface{}) (string, error) {
	return o.tokenProvider.AcceptConsentChallenge(ctx, challenge, subject, scopes, customClaims)
}

func (o *orchestrator) GetConsentRequest(ctx context.Context, challenge string) (*ConsentRequest, error) {
	return o.tokenProvider.GetConsentRequest(ctx, challenge)
}

func (o *orchestrator) ExchangeCodeForTokens(ctx context.Context, code string, codeVerifier string) (*TokenPair, error) {
	return o.tokenProvider.ExchangeCodeForTokens(ctx, code, codeVerifier)
}

func (o *orchestrator) IntrospectToken(ctx context.Context, token string) (bool, string, int64, error) {
	return o.tokenProvider.IntrospectToken(ctx, token)
}

func (o *orchestrator) CreateRelationship(ctx context.Context, req RelationshipRequest) error {
	return o.authProvider.CreateRelationship(ctx, req)
}

func (o *orchestrator) CheckPermission(ctx context.Context, req PermissionRequest) (bool, error) {
	return o.authProvider.CheckPermission(ctx, req)
}

func (o *orchestrator) DeleteRelationship(ctx context.Context, req RelationshipRequest) error {
	return o.authProvider.DeleteRelationship(ctx, req)
}

func (o *orchestrator) IsDeviceOwned(ctx context.Context, deviceID string) (bool, error) {
	return o.authProvider.IsDeviceOwned(ctx, deviceID)
}

func (o *orchestrator) DeleteAllRelationshipsForSubject(ctx context.Context, subject string) error {
	return o.authProvider.DeleteAllRelationshipsForSubject(ctx, subject)
}

func (o *orchestrator) ValidateIDToken(ctx context.Context, rawJWT string, expectedNonce string, expectedAudience string) (*IDTokenClaims, error) {
	return o.tokenProvider.ValidateIDToken(ctx, rawJWT, expectedNonce, expectedAudience)
}

func (o *orchestrator) GetLoginRequest(ctx context.Context, challenge string) (bool, error) {
	return o.tokenProvider.GetLoginRequest(ctx, challenge)
}

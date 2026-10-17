package admin_test

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/application/admin"
)

// mockKetoClient implements keto.Client for testing
type mockKetoClient struct {
	ensureAdminCalled bool
	allowed           bool
}

func (m *mockKetoClient) CheckPermission(ctx context.Context, namespace, object, relation, subjectID string) (bool, error) {
	return m.allowed, nil
}

func (m *mockKetoClient) CreateRelationship(ctx context.Context, namespace, object, relation, subjectID string) error {
	return nil
}

func (m *mockKetoClient) EnsureAdminRole(ctx context.Context, subjectID string) error {
	m.ensureAdminCalled = true
	return nil
}

func TestAdminService_Login_Success(t *testing.T) {
	cfg := &configs.Config{
		Auth: configs.AuthConfig{
			JWTSecret: "test-secret-key-2026",
		},
		Admin: configs.AdminConfig{
			Username: "admin",
			Password: "TestAdminPassword123!",
			UserID:   "00000000-0000-0000-0000-000000000001",
		},
	}
	ketoMock := &mockKetoClient{allowed: true}
	logger := zap.NewNop()

	svc := admin.NewAdminService(cfg, ketoMock, nil, logger)

	res, err := svc.Login(context.Background(), "admin", "TestAdminPassword123!")
	require.NoError(t, err)
	assert.NotEmpty(t, res.AccessToken)
	assert.NotEmpty(t, res.RefreshToken)
	assert.Equal(t, "Bearer", res.TokenType)
	assert.Equal(t, 3600, res.ExpiresIn)
	assert.True(t, ketoMock.ensureAdminCalled)

	// Validate JWT claims
	parsedToken, err := jwt.Parse(res.AccessToken, func(t *jwt.Token) (interface{}, error) {
		return []byte("test-secret-key-2026"), nil
	})
	require.NoError(t, err)
	require.True(t, parsedToken.Valid)

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", claims["sub"])
	assert.Equal(t, "admin", claims["role"])
	assert.Equal(t, "samvada-abs", claims["iss"])

	expVal := int64(claims["exp"].(float64))
	iatVal := int64(claims["iat"].(float64))
	assert.InDelta(t, 3600, expVal-iatVal, 5)
}

func TestAdminService_Login_InvalidCredentials(t *testing.T) {
	cfg := &configs.Config{
		Admin: configs.AdminConfig{
			Username: "admin",
			Password: "CorrectPassword",
			UserID:   "00000000-0000-0000-0000-000000000001",
		},
	}
	ketoMock := &mockKetoClient{allowed: true}
	logger := zap.NewNop()

	svc := admin.NewAdminService(cfg, ketoMock, nil, logger)

	// Wrong password
	_, err := svc.Login(context.Background(), "admin", "WrongPassword")
	assert.ErrorIs(t, err, admin.ErrInvalidCredentials)

	// Wrong username
	_, err = svc.Login(context.Background(), "fakeadmin", "CorrectPassword")
	assert.ErrorIs(t, err, admin.ErrInvalidCredentials)
}

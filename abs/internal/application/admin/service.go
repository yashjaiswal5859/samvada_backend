// =============================================================================
// Samvada Platform — ABS Admin Authentication Service
// =============================================================================
// Package: admin
//
// Responsibilities:
//   1. Authenticates administrative users via username/password (API/curl only).
//   2. Ensures the special Admin role relation is registered in Ory Keto ReBAC.
//   3. Issues standard JWT access token (1h TTL, unchanging) and refresh token (24h TTL, unchanging).
//   4. Validates refresh tokens stored in Redis for human curl operators.
// =============================================================================

package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/infrastructure/keto"
	"github.com/samvada/abs/internal/infrastructure/redis"
)

var (
	ErrInvalidCredentials = errors.New("invalid admin username or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

// LoginResult contains the generated token pair and standard expiry metadata.
type LoginResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // In seconds (3600 = 1 hour, standard TTL)
}

// AdminService defines operations for admin authentication and token management.
type AdminService interface {
	Login(ctx context.Context, username, password string) (*LoginResult, error)
	ValidateRefreshToken(ctx context.Context, refreshToken string) (string, error)
	RevokeRefreshToken(ctx context.Context, refreshToken string) error
}

type adminService struct {
	cfg        *configs.Config
	ketoClient keto.Client
	rdb        *redis.Client
	log        *zap.Logger
}

// NewAdminService creates an instance of AdminService.
func NewAdminService(cfg *configs.Config, ketoClient keto.Client, rdb *redis.Client, log *zap.Logger) AdminService {
	return &adminService{
		cfg:        cfg,
		ketoClient: ketoClient,
		rdb:        rdb,
		log:        log.Named("admin_service"),
	}
}

// Login validates username and password, ensures the Keto admin tuple exists, and generates the token pair.
func (s *adminService) Login(ctx context.Context, username, password string) (*LoginResult, error) {
	// 1. Constant-time comparison to prevent timing side-channel attacks
	userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(s.cfg.Admin.Username)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(s.cfg.Admin.Password)) == 1

	if !userMatch || !passMatch {
		s.log.Warn("Failed admin login attempt: invalid credentials", zap.String("username", username))
		return nil, ErrInvalidCredentials
	}

	adminUserID := s.cfg.Admin.UserID
	if adminUserID == "" {
		adminUserID = "00000000-0000-0000-0000-000000000001"
	}

	// 2. Ensure special Admin role in Ory Keto ReBAC
	if err := s.ketoClient.EnsureAdminRole(ctx, adminUserID); err != nil {
		s.log.Warn("Could not ensure admin role in Ory Keto during login", zap.Error(err), zap.String("user_id", adminUserID))
		// We log warning and continue; Keto might be temporarily slow or already populated
	}

	// 3. Issue Access Token (Standard JWT, 1 Hour TTL — Unchanging)
	now := time.Now().UTC()
	accessTTL := 1 * time.Hour
	claims := jwt.MapClaims{
		"sub":       adminUserID,
		"user_id":   adminUserID,
		"email":     s.cfg.Admin.Username + "@samvada.ai",
		"username":  s.cfg.Admin.Username,
		"role":      "admin",
		"iss":       "samvada-abs",
		"iat":       now.Unix(),
		"exp":       now.Add(accessTTL).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessTokenStr, err := token.SignedString([]byte(s.cfg.Auth.JWTSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to sign admin JWT: %w", err)
	}

	// 4. Issue Refresh Token (Secure Random String, 24 Hours TTL — Unchanging)
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate crypto-random refresh token: %w", err)
	}
	refreshTokenStr := hex.EncodeToString(randomBytes)
	refreshTTL := 24 * time.Hour

	// 5. Store Refresh Token in Redis for validation
	if s.rdb != nil {
		key := "admin:refresh:" + refreshTokenStr
		if err := s.rdb.Set(ctx, key, adminUserID, refreshTTL); err != nil {
			s.log.Error("Failed to store admin refresh token in Redis", zap.Error(err))
		}
	}

	s.log.Info("Admin login successful",
		zap.String("username", s.cfg.Admin.Username),
		zap.String("user_id", adminUserID),
		zap.Int("expires_in_seconds", int(accessTTL.Seconds())),
	)

	return &LoginResult{
		AccessToken:  accessTokenStr,
		RefreshToken: refreshTokenStr,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessTTL.Seconds()),
	}, nil
}

// ValidateRefreshToken checks if the given refresh token is active and returns the associated admin user_id.
func (s *adminService) ValidateRefreshToken(ctx context.Context, refreshToken string) (string, error) {
	if refreshToken == "" {
		return "", ErrInvalidToken
	}

	if s.rdb == nil {
		return s.cfg.Admin.UserID, nil
	}

	key := "admin:refresh:" + refreshToken
	userID, err := s.rdb.Get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("refresh token not found or expired: %w", err)
	}

	return userID, nil
}

// RevokeRefreshToken removes a refresh token from Redis.
func (s *adminService) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	if s.rdb == nil || refreshToken == "" {
		return nil
	}
	key := "admin:refresh:" + refreshToken
	return s.rdb.Del(ctx, key)
}

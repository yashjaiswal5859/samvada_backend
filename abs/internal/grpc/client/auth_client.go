// =============================================================================
// Samvada Platform — ABS Auth Service gRPC Client
// =============================================================================

package client

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ValidateTokenResponse struct {
	Valid     bool      `json:"valid"`
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt int64     `json:"expires_at"`
}

type AuthClient interface {
	ValidateToken(ctx context.Context, tokenString string) (*ValidateTokenResponse, error)
	Close() error
}

type authClient struct {
	conn      *grpc.ClientConn
	jwtSecret string
	log       *zap.Logger
}

func NewAuthClient(target, jwtSecret string, log *zap.Logger) (AuthClient, error) {
	conn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Warn("Could not establish immediate connection to Auth Service (falling back to local cryptographic verification)",
			zap.String("target", target),
			zap.Error(err),
		)
	}

	return &authClient{
		conn:      conn,
		jwtSecret: jwtSecret,
		log:       log.Named("auth_client"),
	}, nil
}

func (c *authClient) ValidateToken(ctx context.Context, tokenString string) (*ValidateTokenResponse, error) {
	// Parse and verify token locally using shared secret (zero network latency for high throughput)
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return []byte(c.jwtSecret), nil
	})
	if err != nil || !token.Valid {
		return &ValidateTokenResponse{Valid: false}, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return &ValidateTokenResponse{Valid: false}, nil
	}

	sub, _ := claims.GetSubject()
	userID, err := uuid.Parse(sub)
	if err != nil {
		return &ValidateTokenResponse{Valid: false}, err
	}

	email, _ := claims["email"].(string)
	role, _ := claims["role"].(string)
	exp, _ := claims.GetExpirationTime()

	var expUnix int64
	if exp != nil {
		expUnix = exp.Unix()
	}

	return &ValidateTokenResponse{
		Valid:     true,
		UserID:    userID,
		Email:     email,
		Role:      role,
		ExpiresAt: expUnix,
	}, nil
}

func (c *authClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

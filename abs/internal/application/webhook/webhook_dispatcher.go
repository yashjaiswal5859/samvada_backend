// =============================================================================
// Samvada Platform — ABS Integration & Webhook Application Service
// =============================================================================

package webhook

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type IntegrationService interface {
	CreateAPIKey(ctx context.Context, workspaceID uuid.UUID, name string) (string, *domain.APIKey, error)
	ListAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]*domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, workspaceID, id uuid.UUID) error

	CreateWebhook(ctx context.Context, workspaceID uuid.UUID, url string) (*domain.Webhook, string, error)
	ListWebhooks(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Webhook, error)
	DeleteWebhook(ctx context.Context, workspaceID, id uuid.UUID) error
}

type integrationService struct {
	integrationRepo domain.IntegrationRepository
	log             *zap.Logger
}

func NewIntegrationService(integrationRepo domain.IntegrationRepository, log *zap.Logger) IntegrationService {
	return &integrationService{
		integrationRepo: integrationRepo,
		log:             log.Named("integration_service"),
	}
}

func (s *integrationService) CreateAPIKey(
	ctx context.Context,
	workspaceID uuid.UUID,
	name string,
) (string, *domain.APIKey, error) {
	rawBytes := make([]byte, 24)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", nil, err
	}
	rawSecret := fmt.Sprintf("sk_live_%s", hex.EncodeToString(rawBytes))
	prefix := rawSecret[:12]

	hash := sha256.Sum256([]byte(rawSecret))
	hashedSecret := hex.EncodeToString(hash[:])

	apiKey := &domain.APIKey{
		ID:           uuid.New(),
		WorkspaceID:  workspaceID,
		Name:         name,
		KeyPrefix:    prefix,
		HashedSecret: hashedSecret,
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.integrationRepo.CreateAPIKey(ctx, apiKey); err != nil {
		s.log.Error("Failed to create API key", zap.Error(err))
		return "", nil, err
	}

	// Return raw secret once for client display
	return rawSecret, apiKey, nil
}

func (s *integrationService) ListAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]*domain.APIKey, error) {
	return s.integrationRepo.ListAPIKeys(ctx, workspaceID)
}

func (s *integrationService) RevokeAPIKey(ctx context.Context, workspaceID, id uuid.UUID) error {
	return s.integrationRepo.DeleteAPIKey(ctx, workspaceID, id)
}

func (s *integrationService) CreateWebhook(
	ctx context.Context,
	workspaceID uuid.UUID,
	url string,
) (*domain.Webhook, string, error) {
	secretBytes := make([]byte, 16)
	_, _ = rand.Read(secretBytes)
	signingSecret := fmt.Sprintf("whsec_%s", hex.EncodeToString(secretBytes))

	wh := &domain.Webhook{
		ID:                  uuid.New(),
		WorkspaceID:         workspaceID,
		URL:                 url,
		SigningSecret:       signingSecret,
		Events:              []string{"call.completed", "agent.published"},
		IsActive:            true,
		ConsecutiveFailures: 0,
		CreatedAt:           time.Now().UTC(),
	}

	if err := s.integrationRepo.CreateWebhook(ctx, wh); err != nil {
		s.log.Error("Failed to register webhook", zap.Error(err))
		return nil, "", err
	}

	return wh, signingSecret, nil
}

func (s *integrationService) ListWebhooks(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Webhook, error) {
	return s.integrationRepo.ListWebhooks(ctx, workspaceID)
}

func (s *integrationService) DeleteWebhook(ctx context.Context, workspaceID, id uuid.UUID) error {
	return s.integrationRepo.DeleteWebhook(ctx, workspaceID, id)
}

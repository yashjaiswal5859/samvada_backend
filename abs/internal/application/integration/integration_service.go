// =============================================================================
// Samvada Platform — ABS Third-Party Connector Application Service
// =============================================================================

package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type ToggleRequest struct {
	Connected bool `json:"connected"`
}

type CredentialsRequest struct {
	CredentialsJSON string `json:"credentials_json"`
}

type ConnectorService struct {
	repo domain.ConnectorRepository
	log  *zap.Logger
}

func NewConnectorService(repo domain.ConnectorRepository, log *zap.Logger) *ConnectorService {
	return &ConnectorService{
		repo: repo,
		log:  log.Named("connector_service"),
	}
}

var defaultConnectors = []struct {
	Provider string
	Name     string
}{
	{Provider: "hubspot", Name: "HubSpot CRM"},
	{Provider: "salesforce", Name: "Salesforce"},
	{Provider: "zapier", Name: "Zapier Automations"},
	{Provider: "twilio", Name: "Twilio Telephony"},
	{Provider: "slack", Name: "Slack Alerts"},
}

func (s *ConnectorService) ListIntegrations(ctx context.Context, workspaceID uuid.UUID) ([]domain.ConnectorIntegration, error) {
	existing, err := s.repo.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	byProvider := make(map[string]domain.ConnectorIntegration)
	for _, item := range existing {
		byProvider[item.Provider] = item
	}

	result := make([]domain.ConnectorIntegration, 0, len(defaultConnectors))
	for _, def := range defaultConnectors {
		if val, exists := byProvider[def.Provider]; exists {
			result = append(result, val)
		} else {
			result = append(result, domain.ConnectorIntegration{
				ID:          uuid.New(),
				WorkspaceID: workspaceID,
				Provider:    def.Provider,
				Name:        def.Name,
				IsConnected: false,
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			})
		}
	}

	return result, nil
}

func (s *ConnectorService) ToggleIntegration(ctx context.Context, workspaceID, id uuid.UUID, connected bool) (*domain.ConnectorIntegration, error) {
	item, err := s.repo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}

	if item == nil {
		return nil, fmt.Errorf("integration not found or not initialized")
	}

	if err := s.repo.Toggle(ctx, workspaceID, id, connected); err != nil {
		return nil, fmt.Errorf("failed to toggle integration: %w", err)
	}

	item.IsConnected = connected
	return item, nil
}

func (s *ConnectorService) SaveCredentials(ctx context.Context, workspaceID, id uuid.UUID, credsJSON string) (*domain.ConnectorIntegration, error) {
	item, err := s.repo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}

	if item == nil {
		return nil, fmt.Errorf("integration not found")
	}

	if err := s.repo.UpdateCredentials(ctx, workspaceID, id, credsJSON); err != nil {
		return nil, fmt.Errorf("failed to save integration credentials: %w", err)
	}

	item.CredentialsJSON = credsJSON
	item.IsConnected = true
	return item, nil
}

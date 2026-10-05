// =============================================================================
// Samvada Platform — Tenant Provisioning Kafka Consumer Worker
// =============================================================================
// Consumes auth.onboarding.completed (and auth.user.registered) events from Kafka
// and auto-provisions the Organization, Workspace, Owner/Admin Members,
// and Organization Wallet with $5.00 demo credits in PostgreSQL (samvada_abs).
// =============================================================================

package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/application/organization"
	"github.com/samvada/abs/internal/domain"
)

type TenantProvisionPayload struct {
	EventType     string  `json:"event_type"`
	UserID        string  `json:"user_id"`
	Email         string  `json:"email"`
	UserEmail     string  `json:"user_email"`
	FirstName     string  `json:"first_name"`
	LastName      *string `json:"last_name,omitempty"`
	AccountType   string  `json:"account_type"`
	OrgID         string  `json:"org_id"`
	OrgName       string  `json:"org_name"`
	WorkspaceID   string  `json:"workspace_id"`
	WorkspaceName string  `json:"workspace_name"`
}

type TenantProvisionConsumer struct {
	consumerGroup sarama.ConsumerGroup
	topics        []string
	orgService    organization.OrgService
	orgRepo       domain.OrgRepository
	wsRepo        domain.WorkspaceRepository
	profileRepo   domain.UserProfileRepository
	log           *zap.Logger
	stopChan      chan struct{}
}

func NewTenantProvisionConsumer(
	brokers []string,
	groupName string,
	topics []string,
	orgService organization.OrgService,
	orgRepo domain.OrgRepository,
	wsRepo domain.WorkspaceRepository,
	profileRepo domain.UserProfileRepository,
	log *zap.Logger,
) (*TenantProvisionConsumer, error) {
	if len(topics) == 0 {
		topics = []string{"auth.onboarding.completed"}
	}
	if groupName == "" {
		groupName = "abs-tenant-worker-group"
	}

	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}

	group, err := sarama.NewConsumerGroup(brokers, groupName, config)
	if err != nil {
		return nil, fmt.Errorf("creating tenant provision consumer group: %w", err)
	}

	return &TenantProvisionConsumer{
		consumerGroup: group,
		topics:        topics,
		orgService:    orgService,
		orgRepo:       orgRepo,
		wsRepo:        wsRepo,
		profileRepo:   profileRepo,
		log:           log.Named("tenant_provision_consumer"),
		stopChan:      make(chan struct{}),
	}, nil
}

func (w *TenantProvisionConsumer) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-w.stopChan:
				return
			case <-ctx.Done():
				return
			default:
				if err := w.consumerGroup.Consume(ctx, w.topics, w); err != nil {
					w.log.Warn("Kafka consumer error in tenant provision worker", zap.Error(err))
				}
			}
		}
	}()
	w.log.Info("Tenant Provision Kafka consumer started", zap.Strings("topics", w.topics))
}

func (w *TenantProvisionConsumer) Stop() {
	close(w.stopChan)
	if err := w.consumerGroup.Close(); err != nil {
		w.log.Warn("Error closing tenant provision consumer group", zap.Error(err))
	}
	w.log.Info("Tenant Provision consumer worker stopped")
}

func (w *TenantProvisionConsumer) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (w *TenantProvisionConsumer) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (w *TenantProvisionConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		w.processMessage(msg.Value)
		session.MarkMessage(msg, "")
	}
	return nil
}

func (w *TenantProvisionConsumer) processMessage(payloadBytes []byte) {
	// Try parsing as outbox envelope first
	var env AuthEventEnvelope
	var data TenantProvisionPayload

	if err := json.Unmarshal(payloadBytes, &env); err == nil && env.EventType != "" {
		switch p := env.Payload.(type) {
		case string:
			_ = json.Unmarshal([]byte(p), &data)
		case map[string]interface{}:
			b, _ := json.Marshal(p)
			_ = json.Unmarshal(b, &data)
		}
	}

	// If not extracted via envelope, try direct unmarshal
	if data.UserID == "" {
		_ = json.Unmarshal(payloadBytes, &data)
	}

	// If still no user_id, check aggregate_id
	if data.UserID == "" && env.AggregateID != "" {
		data.UserID = env.AggregateID
	}

	if data.UserID == "" {
		w.log.Warn("Skipping tenant provision message: missing user_id", zap.ByteString("raw", payloadBytes))
		return
	}

	parsedUserID, err := uuid.Parse(data.UserID)
	if err != nil {
		w.log.Error("Invalid user_id in tenant provision message", zap.String("user_id", data.UserID), zap.Error(err))
		return
	}

	email := data.Email
	if email == "" {
		email = data.UserEmail
	}
	if email == "" {
		email = fmt.Sprintf("user-%s@samvada.local", parsedUserID.String()[:8])
	}

	orgName := data.OrgName
	if orgName == "" {
		if data.FirstName != "" {
			orgName = data.FirstName + "'s Organization"
		} else {
			orgName = "Personal Organization"
		}
	}

	wsName := data.WorkspaceName
	if wsName == "" {
		wsName = "Default Workspace"
	}

	var parsedOrgID *uuid.UUID
	if data.OrgID != "" {
		if id, err := uuid.Parse(data.OrgID); err == nil && id != uuid.Nil {
			parsedOrgID = &id
		}
	}

	var parsedWsID *uuid.UUID
	if data.WorkspaceID != "" {
		if id, err := uuid.Parse(data.WorkspaceID); err == nil && id != uuid.Nil {
			parsedWsID = &id
		}
	}

	ctx := context.Background()

	// Check if this organization already exists
	if parsedOrgID != nil {
		if existingOrg, err := w.orgRepo.GetByID(ctx, *parsedOrgID); err == nil && existingOrg != nil {
			w.log.Info("Organization already exists in ABS, verifying membership", zap.String("org_id", parsedOrgID.String()))
			// Check member
			if _, mErr := w.orgRepo.GetMember(ctx, *parsedOrgID, parsedUserID); mErr != nil {
				_ = w.orgRepo.AddMember(ctx, &domain.OrgMember{
					ID:     uuid.New(),
					OrgID:  *parsedOrgID,
					UserID: parsedUserID,
					Email:  email,
					Role:   "owner",
					Status: "active",
				})
			}
			return
		}
	}

	// Check if user already has an org with this name
	userOrgs, _ := w.orgRepo.ListForUser(ctx, parsedUserID)
	if len(userOrgs) > 0 {
		for _, o := range userOrgs {
			if o.Name == orgName {
				w.log.Info("User already has organization with name", zap.String("org_name", orgName))
				return
			}
		}
	}

	// Create Organization, Workspace, Owner/Admin and Wallet
	org, ws, wallet, err := w.orgService.CreateOrganization(ctx, organization.CreateOrgRequest{
		Name:          orgName,
		WorkspaceName: wsName,
		BillingEmail:  email,
		UserID:        parsedUserID,
		UserEmail:     email,
		OrgID:         parsedOrgID,
		WorkspaceID:   parsedWsID,
	})
	if err != nil {
		w.log.Error("Failed to provision tenant from Kafka event",
			zap.String("user_id", parsedUserID.String()),
			zap.String("org_name", orgName),
			zap.Error(err),
		)
		return
	}

	// Upsert user profile in ABS PostgreSQL
	lastNameStr := ""
	if data.LastName != nil {
		lastNameStr = *data.LastName
	}
	if w.profileRepo != nil {
		_ = w.profileRepo.Upsert(ctx, &domain.UserProfile{
			UserID:    parsedUserID,
			FirstName: data.FirstName,
			LastName:  lastNameStr,
			Email:     email,
			Company:   orgName,
			UpdatedAt: time.Now().UTC(),
		})
	}

	w.log.Info(fmt.Sprintf("\n"+
		"======================================================================\n"+
		"  🏢 [ABS KAFKA WORKER] TENANT PROVISIONED SUCCESSFULLY\n"+
		"  User ID:       %s\n"+
		"  Email:         %s\n"+
		"  Organization:  %s (ID: %s, Slug: %s)\n"+
		"  Workspace:     %s (ID: %s)\n"+
		"  Owner/Admin:   Yes (Role: owner, Status: active)\n"+
		"  Wallet Balance: $%.2f USD ($%.2f Demo Credits)\n"+
		"======================================================================",
		parsedUserID.String(),
		email,
		org.Name, org.ID.String(), org.Slug,
		ws.Name, ws.ID.String(),
		wallet.BalanceUSD, wallet.PromotionalCreditUSD,
	))
}

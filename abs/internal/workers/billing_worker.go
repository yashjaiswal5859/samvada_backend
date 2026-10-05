// =============================================================================
// Samvada Platform — ABS Real-Time Billing & Wallet Deduction Worker (Phase 3.2)
// =============================================================================
// Consumes s2s.call.completed events from Kafka and performs atomic wallet deductions
// and ledger transaction logging in PostgreSQL.
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

	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/domain"
)

type CallCompletedEvent struct {
	CallID       uuid.UUID  `json:"call_id"`
	OrgID        uuid.UUID  `json:"org_id"`
	WorkspaceID  uuid.UUID  `json:"workspace_id"`
	AgentID      uuid.UUID  `json:"agent_id"`
	CampaignID   *uuid.UUID `json:"campaign_id,omitempty"`
	DurationSec  uint32     `json:"duration_sec"`
	BillableSec  uint32     `json:"billable_sec"`
	TotalCostUSD float64    `json:"total_cost_usd"`
	Disposition  string     `json:"disposition"`
}

type BillingWorker struct {
	consumerGroup sarama.ConsumerGroup
	walletService billing.WalletService
	campaignRepo  domain.CampaignRepository
	topic         string
	log           *zap.Logger
	stopChan      chan struct{}
}

func NewBillingWorker(
	brokers []string,
	groupName string,
	topic string,
	walletService billing.WalletService,
	campaignRepo domain.CampaignRepository,
	log *zap.Logger,
) (*BillingWorker, error) {
	if topic == "" {
		topic = "s2s.call.completed"
	}
	if groupName == "" {
		groupName = "abs-billing-group"
	}

	config := sarama.NewConfig()
	config.Version = sarama.V3_5_0_0
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}

	group, err := sarama.NewConsumerGroup(brokers, groupName, config)
	if err != nil {
		return nil, fmt.Errorf("creating billing consumer group: %w", err)
	}

	return &BillingWorker{
		consumerGroup: group,
		walletService: walletService,
		campaignRepo:  campaignRepo,
		topic:         topic,
		log:           log.Named("billing_worker"),
		stopChan:      make(chan struct{}),
	}, nil
}

func (w *BillingWorker) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-w.stopChan:
				return
			case <-ctx.Done():
				return
			default:
				topics := []string{w.topic}
				if err := w.consumerGroup.Consume(ctx, topics, w); err != nil {
					w.log.Warn("Kafka consumer error in billing worker", zap.Error(err))
					time.Sleep(2 * time.Second)
				}
			}
		}
	}()
	w.log.Info("ABS Billing worker started", zap.String("topic", w.topic))
}

func (w *BillingWorker) Stop() {
	close(w.stopChan)
	if err := w.consumerGroup.Close(); err != nil {
		w.log.Warn("Error closing billing consumer group", zap.Error(err))
	}
	w.log.Info("ABS Billing worker stopped")
}

func (w *BillingWorker) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (w *BillingWorker) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (w *BillingWorker) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		var ev CallCompletedEvent
		if err := json.Unmarshal(msg.Value, &ev); err != nil {
			w.log.Warn("Failed to unmarshal s2s.call.completed message", zap.Error(err))
			session.MarkMessage(msg, "")
			continue
		}

		if ev.TotalCostUSD > 0 && ev.OrgID != uuid.Nil {
			desc := fmt.Sprintf("Call %s deduction (%ds, %s)", ev.CallID.String()[:8], ev.DurationSec, ev.Disposition)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, _, err := w.walletService.DeductCallCost(ctx, ev.OrgID, ev.TotalCostUSD, desc, &ev.CallID)
			cancel()

			if err != nil {
				w.log.Error("Failed to deduct call cost from organization wallet",
					zap.String("org_id", ev.OrgID.String()),
					zap.String("call_id", ev.CallID.String()),
					zap.Float64("cost_usd", ev.TotalCostUSD),
					zap.Error(err),
				)
			} else {
				w.log.Info("Processed call completion wallet deduction",
					zap.String("org_id", ev.OrgID.String()),
					zap.String("call_id", ev.CallID.String()),
					zap.Float64("cost_usd", ev.TotalCostUSD),
				)
			}
		}

		if ev.CampaignID != nil && w.campaignRepo != nil {
			successful := 0
			failed := 0
			if ev.Disposition == "completed" {
				successful = 1
			} else {
				failed = 1
			}
			_ = w.campaignRepo.IncrementCampaignStats(context.Background(), *ev.CampaignID, 1, successful, failed)
		}

		session.MarkMessage(msg, "")
	}
	return nil
}

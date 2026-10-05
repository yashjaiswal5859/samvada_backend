// =============================================================================
// Samvada Platform — ABS Campaign Dispatch Kafka Consumer Worker
// =============================================================================
// Consumes abs.campaign.dispatch events from Kafka and initiates outbound calls
// via the Speech-to-Speech (S2S) voice engine.
// =============================================================================

package workers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/grpc/client"
)

type CampaignDispatchConsumer struct {
	consumerGroup sarama.ConsumerGroup
	s2sClient    client.S2SClient
	campaignRepo domain.CampaignRepository
	topic        string
	log          *zap.Logger
	stopChan     chan struct{}
}

func NewCampaignDispatchConsumer(
	brokers []string,
	groupName string,
	topic string,
	s2sClient client.S2SClient,
	campaignRepo domain.CampaignRepository,
	log *zap.Logger,
) (*CampaignDispatchConsumer, error) {
	if topic == "" {
		topic = "abs.campaign.dispatch"
	}
	if groupName == "" {
		groupName = "abs-campaign-worker-group"
	}

	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}

	group, err := sarama.NewConsumerGroup(brokers, groupName, config)
	if err != nil {
		return nil, fmt.Errorf("creating campaign dispatch consumer group: %w", err)
	}

	return &CampaignDispatchConsumer{
		consumerGroup: group,
		s2sClient:    s2sClient,
		campaignRepo: campaignRepo,
		topic:        topic,
		log:          log.Named("campaign_dispatch_consumer"),
		stopChan:     make(chan struct{}),
	}, nil
}

func (w *CampaignDispatchConsumer) Start(ctx context.Context) {
	topics := []string{w.topic}
	go func() {
		for {
			select {
			case <-w.stopChan:
				return
			case <-ctx.Done():
				return
			default:
				if err := w.consumerGroup.Consume(ctx, topics, w); err != nil {
					w.log.Warn("Kafka consumer error in campaign dispatch worker", zap.Error(err))
				}
			}
		}
	}()
	w.log.Info("Campaign dispatch consumer worker started", zap.String("topic", w.topic))
}

func (w *CampaignDispatchConsumer) Stop() {
	close(w.stopChan)
	if err := w.consumerGroup.Close(); err != nil {
		w.log.Warn("Error closing campaign dispatch consumer group", zap.Error(err))
	}
	w.log.Info("Campaign dispatch consumer worker stopped")
}

func (w *CampaignDispatchConsumer) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (w *CampaignDispatchConsumer) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (w *CampaignDispatchConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		w.processMessage(session.Context(), msg.Value)
		session.MarkMessage(msg, "")
	}
	return nil
}

func (w *CampaignDispatchConsumer) processMessage(ctx context.Context, payload []byte) {
	var ev domain.CampaignDispatchEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		w.log.Error("Failed to decode campaign dispatch event", zap.Error(err))
		return
	}

	w.log.Info("Consuming campaign dispatch event from Kafka",
		zap.String("event_id", ev.EventID.String()),
		zap.String("campaign_id", ev.CampaignID.String()),
		zap.String("phone", ev.PhoneNumber),
	)

	if w.s2sClient != nil {
		resp, err := w.s2sClient.InitiateOutboundCall(ctx, client.OutboundCallRequest{
			CampaignID:  ev.CampaignID,
			AgentID:     ev.AgentID,
			WorkspaceID: ev.WorkspaceID,
			ContactID:   ev.ContactID,
			PhoneNumber: ev.PhoneNumber,
		})
		if err != nil {
			w.log.Error("S2S engine failed to initiate outbound call",
				zap.String("campaign_id", ev.CampaignID.String()),
				zap.String("phone", ev.PhoneNumber),
				zap.Error(err),
			)
			return
		}
		w.log.Info("Outbound call successfully queued with S2S engine",
			zap.String("call_id", resp.CallID),
			zap.String("campaign_id", ev.CampaignID.String()),
		)
	}

	// Update contact record status to dialed
	if w.campaignRepo != nil {
		_ = w.campaignRepo.UpdateContactLastCalled(ctx, ev.ContactID, "dialed")
	}
}

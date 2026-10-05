// =============================================================================
// Samvada Platform — Outbound Campaign Dispatcher Worker (Phase 3.4)
// =============================================================================
// Periodically polls running campaigns, respects concurrency limits, and dispatches
// outbound calls to the S2S engine via gRPC.
// =============================================================================

package workers

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/infrastructure/kafka"
)

type CampaignRunner struct {
	campaignRepo domain.CampaignRepository
	producer     *kafka.Producer
	log          *zap.Logger
	ticker       *time.Ticker
	stopChan     chan struct{}
}

func NewCampaignRunner(
	campaignRepo domain.CampaignRepository,
	producer *kafka.Producer,
	log *zap.Logger,
) *CampaignRunner {
	return &CampaignRunner{
		campaignRepo: campaignRepo,
		producer:     producer,
		log:          log.Named("campaign_runner"),
		ticker:       time.NewTicker(5 * time.Second),
		stopChan:     make(chan struct{}),
	}
}

func (r *CampaignRunner) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-r.stopChan:
				return
			case <-ctx.Done():
				return
			case <-r.ticker.C:
				r.processRunningCampaigns(ctx)
			}
		}
	}()
	r.log.Info("ABS Campaign Runner worker started")
}

func (r *CampaignRunner) Stop() {
	r.ticker.Stop()
	close(r.stopChan)
	r.log.Info("ABS Campaign Runner worker stopped")
}

func (r *CampaignRunner) processRunningCampaigns(ctx context.Context) {
	campaigns, err := r.campaignRepo.ListRunningCampaigns(ctx)
	if err != nil {
		r.log.Warn("Failed to list running campaigns", zap.Error(err))
		return
	}

	for _, cmp := range campaigns {
		limit := cmp.ConcurrentCallsLimit
		if limit <= 0 {
			limit = 10
		}

		contacts, err := r.campaignRepo.GetNextPendingContacts(ctx, cmp.AudienceID, limit)
		if err != nil {
			r.log.Warn("Failed to fetch pending contacts", zap.String("campaign_id", cmp.ID.String()), zap.Error(err))
			continue
		}

		if len(contacts) == 0 {
			// No pending contacts left: mark campaign completed
			_ = r.campaignRepo.UpdateCampaignStatus(ctx, cmp.WorkspaceID, cmp.ID, "completed")
			r.log.Info("Campaign marked completed (all audience contacts dispatched)",
				zap.String("campaign_id", cmp.ID.String()),
				zap.String("name", cmp.Name),
			)
			continue
		}

		for _, c := range contacts {
			if r.producer != nil {
				ev := domain.CampaignDispatchEvent{
					EventID:     uuid.New(),
					CampaignID:  cmp.ID,
					AgentID:     cmp.AgentID,
					WorkspaceID: cmp.WorkspaceID,
					ContactID:   c.ID,
					PhoneNumber: c.Phone,
					Timestamp:   time.Now().UTC(),
				}

				_, _, err := r.producer.PublishJSON("abs.campaign.dispatch", cmp.ID.String(), ev)
				if err != nil {
					r.log.Error("Failed to publish campaign dispatch event to Kafka",
						zap.String("campaign_id", cmp.ID.String()),
						zap.String("phone", c.Phone),
						zap.Error(err),
					)
					continue
				}
				r.log.Info("Dispatched campaign contact to Kafka",
					zap.String("campaign_id", cmp.ID.String()),
					zap.String("contact_id", c.ID.String()),
					zap.String("phone", c.Phone),
				)
			}

			_ = r.campaignRepo.UpdateContactLastCalled(ctx, c.ID, "dialed")
		}
	}
}

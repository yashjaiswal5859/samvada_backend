// =============================================================================
// Samvada Platform — ABS Webhook Delivery & Retry Worker (Phase 4.3)
// =============================================================================

package workers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type WebhookRetryWorker struct {
	repo       domain.IntegrationRepository
	httpClient *http.Client
	pollFreq   time.Duration
	stopChan   chan struct{}
	log        *zap.Logger
}

func NewWebhookRetryWorker(
	repo domain.IntegrationRepository,
	pollFreq time.Duration,
	log *zap.Logger,
) *WebhookRetryWorker {
	if pollFreq <= 0 {
		pollFreq = 5 * time.Second
	}
	return &WebhookRetryWorker{
		repo: repo,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		pollFreq: pollFreq,
		stopChan: make(chan struct{}),
		log:      log.Named("webhook_retry_worker"),
	}
}

func (w *WebhookRetryWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.pollFreq)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-w.stopChan:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.processFailedOutboxEvents(ctx)
			}
		}
	}()
	w.log.Info("ABS Webhook Retry Worker started", zap.Duration("poll_freq", w.pollFreq))
}

func (w *WebhookRetryWorker) Stop() {
	close(w.stopChan)
	w.log.Info("ABS Webhook Retry Worker stopped")
}

func (w *WebhookRetryWorker) processFailedOutboxEvents(ctx context.Context) {
	pendingEvents, err := w.repo.ListPendingOutboxEvents(ctx, 20)
	if err != nil || len(pendingEvents) == 0 {
		return
	}

	for _, evt := range pendingEvents {
		if evt.EventType != "webhook.dispatch" {
			continue
		}

		payloadBytes := []byte(evt.Payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, evt.AggregateID, bytes.NewReader(payloadBytes))
		if err != nil {
			_ = w.repo.MarkOutboxFailed(ctx, evt.ID, err.Error())
			continue
		}

		// Signature calculation
		mac := hmac.New(sha256.New, []byte("whsec_samvada_default"))
		mac.Write(payloadBytes)
		signature := hex.EncodeToString(mac.Sum(nil))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Samvada-Signature", fmt.Sprintf("t=%d,v1=%s", time.Now().Unix(), signature))
		req.Header.Set("User-Agent", "Samvada-WebhookDispatcher/1.0")

		resp, err := w.httpClient.Do(req)
		if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errStr := "delivery failed"
			if err != nil {
				errStr = err.Error()
			} else {
				errStr = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			w.log.Warn("Webhook delivery failed, recording outbox failure",
				zap.String("event_id", evt.ID.String()),
				zap.String("target_url", evt.AggregateID),
				zap.String("error", errStr),
			)
			_ = w.repo.MarkOutboxFailed(ctx, evt.ID, errStr)
		} else {
			resp.Body.Close()
			_ = w.repo.MarkOutboxProcessed(ctx, evt.ID)
			w.log.Info("Webhook successfully delivered", zap.String("event_id", evt.ID.String()), zap.String("url", evt.AggregateID))
		}
	}
}

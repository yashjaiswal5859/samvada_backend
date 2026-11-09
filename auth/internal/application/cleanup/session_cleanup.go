package cleanup

import (
	"context"
	"time"

	"github.com/ridesafety/auth-service/internal/domain/oauthsession"
	"go.uber.org/zap"
)

// StartSessionCleanup runs a background goroutine that periodically removes
// expired oauth_sessions rows to prevent stale session accumulation (DoS protection).
func StartSessionCleanup(ctx context.Context, repo oauthsession.Repository, interval time.Duration, logger *zap.Logger) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				deleted, err := repo.DeleteExpired(cleanupCtx)
				if err != nil {
					logger.Error("failed to clean up expired oauth sessions", zap.Error(err))
				} else if deleted > 0 {
					logger.Info("cleaned up expired oauth sessions", zap.Int64("deleted", deleted))
				}
				cancel()
			}
		}
	}()
}

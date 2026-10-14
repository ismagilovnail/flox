package push

import (
	"context"
	"log/slog"
	"time"
)

// ScheduledSendBatch bounds how many due broadcasts one tick sends; the
// next tick picks up the rest.
const ScheduledSendBatch = 20

// RunScheduler sends due scheduled broadcasts (Repository.SendDue) every
// interval until ctx ends — apps/worker runs it. Its deliveries then go out
// through the usual queue (internal/pushsend).
func RunScheduler(ctx context.Context, repo *Repository, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		n, err := repo.SendDue(ctx, time.Now(), ScheduledSendBatch)
		if err != nil && ctx.Err() == nil {
			logger.Error("push: sending scheduled broadcasts", "error", err)
		}
		if n > 0 {
			logger.Info("push: scheduled broadcasts sent", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

package eventqueue

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DepthReporter is the narrow slice of *PostgresQueue PollDepth needs, so
// a test can substitute a fake without a real Postgres pool.
type DepthReporter interface {
	Depth(ctx context.Context) (int, error)
}

// PollDepth sets gauge to the queue's current depth every interval, until
// ctx is done — apps/worker's own counterpart to Flusher/Deliverer's
// PollLoop, for a value that's read rather than claimed. Runs once
// immediately on start so the gauge isn't left at zero (Prometheus's own
// default for a never-`Set` gauge) for a full interval after startup.
func PollDepth(ctx context.Context, reporter DepthReporter, gauge prometheus.Gauge, interval time.Duration, logger *slog.Logger) {
	tick := func() {
		n, err := reporter.Depth(ctx)
		if err != nil {
			logger.Error("polling event queue depth", "error", err)
			return
		}
		gauge.Set(float64(n))
	}

	tick()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}

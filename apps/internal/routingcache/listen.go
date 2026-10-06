package routingcache

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Channel is what migration 00050's triggers notify on any write to a
// routing table.
const Channel = "flox_routing_changed"

// Listen keeps a dedicated connection LISTENing on Channel and invalidates
// the cache on every notification, until ctx is done. It flushes after each
// (re)connect too, since edits made while it was not listening were never
// announced. A dedicated connection, not one borrowed from the pool, so a
// LISTEN never leaks into a pooled query's session.
func (c *Cache) Listen(ctx context.Context, databaseURL string) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := c.listenOnce(ctx, databaseURL)
		if ctx.Err() != nil {
			return
		}
		c.logger.Warn("routing cache listener disconnected, edits apply within the cache TTL until it reconnects",
			"error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (c *Cache) listenOnce(ctx context.Context, databaseURL string) error {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	c.Invalidate()
	c.logger.Info("routing cache listening for configuration changes", "channel", Channel)
	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return err
		}
		c.Invalidate()
	}
}

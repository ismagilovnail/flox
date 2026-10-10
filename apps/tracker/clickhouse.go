package main

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
)

// errClickHouseNotConnected is what lookups return until ClickHouse is
// reached. conversion.Service records such a postback as unknown_click
// with a retry (internal/reattribute), so it is attributed once ClickHouse
// answers.
var errClickHouseNotConnected = errors.New("tracker: clickhouse not connected yet")

// lateClickHouse is the tracker's ClickHouse, connected in the background.
// A ClickHouse that is down when the tracker starts must not block the
// redirect path (CLAUDE.md #9) or leave attribution without clicks until a
// restart (tech audit T1): the tracker keeps trying and switches over the
// moment it connects.
type lateClickHouse struct {
	resolver atomic.Pointer[chstore.ClickResolver]
	events   atomic.Pointer[chstore.EventStore]
}

var _ attribution.ClickResolver = (*lateClickHouse)(nil)

const (
	chRetryMin = time.Second
	chRetryMax = 30 * time.Second
)

// connect dials and migrates ClickHouse until it succeeds or ctx is done,
// then serves lookups through it. The connection is closed when ctx is.
func (c *lateClickHouse) connect(ctx context.Context, db *pgxpool.Pool, cfg config.ClickHouseConfig, logger *slog.Logger) {
	wait := chRetryMin
	for {
		conn, err := dial(ctx, db, cfg)
		if err == nil {
			c.use(conn)
			logger.Info("clickhouse connected, attribution matches real clicks")
			<-ctx.Done()
			_ = conn.Close()
			return
		}
		logger.Warn("clickhouse unavailable, conversions are recorded unknown_click and retried by the worker",
			"error", err, "retry_in", wait)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, chRetryMax)
	}
}

func dial(ctx context.Context, db *pgxpool.Pool, cfg config.ClickHouseConfig) (driver.Conn, error) {
	conn, err := chconn.NewConn(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := chstore.MigrateLocked(ctx, db, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (c *lateClickHouse) use(conn driver.Conn) {
	c.events.Store(chstore.NewEventStore(conn))
	c.resolver.Store(chstore.NewClickResolver(conn))
}

func (c *lateClickHouse) ByClickID(ctx context.Context, organizationID, clickID string) (attribution.Click, error) {
	r := c.resolver.Load()
	if r == nil {
		return attribution.Click{}, errClickHouseNotConnected
	}
	return r.ByClickID(ctx, organizationID, clickID)
}

func (c *lateClickHouse) ByExternalClickID(ctx context.Context, organizationID, externalClickID string) ([]attribution.Click, error) {
	r := c.resolver.Load()
	if r == nil {
		return nil, errClickHouseNotConnected
	}
	return r.ByExternalClickID(ctx, organizationID, externalClickID)
}

// NewConversionsByOffer implements offercap.Counter: no ClickHouse yet, no
// count, and the monitor keeps its (empty) snapshot — no offer is capped.
func (c *lateClickHouse) NewConversionsByOffer(ctx context.Context, targets []chstore.OfferConversionTarget) (map[string]uint64, error) {
	e := c.events.Load()
	if e == nil {
		return nil, errClickHouseNotConnected
	}
	return e.NewConversionsByOffer(ctx, targets)
}

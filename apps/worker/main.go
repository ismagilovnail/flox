// Command worker is FLOX's async background processor. It is a separate
// binary from apps/api and apps/tracker, same Go module, so it can be
// deployed and scaled independently of the redirect hot path — exactly the
// same reasoning apps/tracker's own doc comment gives.
//
// Three poll loops and one interval scheduler run here:
//   - internal/postback (§46, Phase 24): outgoing postback delivery —
//     claim due rows from postback_deliveries and dispatch them, with
//     exponential backoff and a dead-letter state after repeated failure.
//   - internal/eventqueue (§43/§47/§48, Phases 25-26): the tracker's event
//     queue, claimed in batches and routed by type into ClickHouse's
//     click_events/tracking_events/conversion_events (internal/chstore) —
//     the real five-table design, replacing Phase 25's disposable single
//     table. See docs/analytics-pipeline.md.
//   - internal/postbacklog (§48, Phase 26): the postback attempt audit
//     log — both directions (conversion's incoming outcomes, postback's
//     outgoing delivery attempts) claimed in batches and written to
//     ClickHouse's postback_events.
//   - internal/costsync (§74/§27-COST): the automated counterpart to the
//     API's on-demand POST .../connection/sync — on a fixed interval
//     (costSyncInterval below), pulls ad spend for every connected
//     Facebook/TikTok ad account across every org and writes matching
//     campaigns' cost_entries. Unlike the three loops above, this isn't a
//     claim-a-batch-of-due-rows queue drain — there's no per-row "due"
//     state to claim, just "sync everyone again every N hours" — so it
//     runs on a time.Ticker (costsync.Scheduler.RunLoop) rather than
//     PollLoop's claim/idle shape.
//
// §53/Phase 29: GET /metrics (Prometheus) joins /health on this binary's
// bare mux, and a fifth background goroutine (eventqueue.PollDepth)
// refreshes the event_queue_depth gauge on the same time.Ticker shape as
// costSyncScheduler above.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/facebookads"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/tiktokads"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/costsync"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/telemetry"
)

// postbackPollBatchSize/postbackPollIdle: how many due postback deliveries
// one poll claims, and how long to wait after a partial batch.
const (
	postbackPollBatchSize = 20
	postbackPollIdle      = 5 * time.Second
)

// eventPollBatchSize/eventPollIdle: same shape, sized for click/tracking
// event volume, which runs far higher than outgoing postback deliveries —
// a bigger batch and a shorter idle wait keeps the queue from backing up
// under normal traffic.
const (
	eventPollBatchSize = 500
	eventPollIdle      = 2 * time.Second
)

// attemptLogPollBatchSize/attemptLogPollIdle: the postback attempt audit
// log runs at roughly postback delivery + incoming postback volume — low
// relative to click events, so it shares postback's batch size/idle shape
// rather than events'.
const (
	attemptLogPollBatchSize = 20
	attemptLogPollIdle      = 5 * time.Second
)

// costSyncInterval: how often every connected Facebook/TikTok ad account
// gets re-synced. Both platforms' reporting APIs commonly revise very
// recent days' spend after the fact (the same reason costsync's own
// defaultLookbackDays re-pulls a 30-day window every run, not just "since
// last sync"), so re-running a few times a day catches those revisions
// without re-fetching so often it risks the ad platforms' own rate limits.
const costSyncInterval = 6 * time.Hour

// queueDepthPollInterval: how often §53's event_queue_depth gauge is
// refreshed. A plain SELECT count(*) — cheap enough to run often, but
// there is no reason to poll faster than anyone will actually look at a
// dashboard.
const queueDepthPollInterval = 15 * time.Second

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadWorker()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Setup(ctx, cfg.OTelServiceName, cfg.OTelExporterOTLPEndpoint)
	if err != nil {
		logger.Error("telemetry setup failed", "error", err)
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			logger.Error("telemetry shutdown failed", "error", err)
		}
	}()

	db, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		return err
	}
	defer db.Close()

	ch, err := chconn.NewConn(ctx, cfg.ClickHouse)
	if err != nil {
		logger.Error("clickhouse connection failed", "error", err)
		return err
	}
	defer ch.Close()
	if err := chstore.Migrate(ctx, ch); err != nil {
		logger.Error("clickhouse schema migration failed", "error", err)
		return err
	}
	chEvents := chstore.NewEventStore(ch)

	adAccountRepo := adaccount.NewRepository(db)
	costSyncSvc := costsync.NewService(adAccountRepo, campaign.NewRepository(db), cost.NewService(cost.NewRepository(db), conversion.NewPostgresFX(db)), costsync.Providers{
		FacebookAds: facebookads.New(),
		TikTokAds:   tiktokads.New(),
	})
	costSyncScheduler := costsync.NewScheduler(costSyncSvc, adAccountRepo, logger)

	attemptLogQueue := postbacklog.NewPostgresQueue(db, logger)
	// otelhttp.NewTransport gives each outbound postback delivery its own
	// trace (§53's "trace IDs") — there's no inbound request to inherit a
	// trace context from here (this fires from a poll loop, not an HTTP
	// handler), so each call becomes its own root span. A no-op when no
	// OTel endpoint is configured (telemetry.Setup's own doc comment).
	postbackClient := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	deliverer := postback.NewDeliverer(
		postback.NewPostgresStore(db),
		postbackClient,
		postbacklog.NewDeliveryAttemptLogger(attemptLogQueue),
		logger,
	)
	eventQueue := eventqueue.NewPostgresQueue(db)
	flusher := eventqueue.NewFlusher(eventQueue, chEvents, logger)
	attemptFlusher := postbacklog.NewFlusher(attemptLogQueue, chEvents, logger)

	// A bare mux for orchestration liveness probes + the Prometheus
	// scrape target — this binary has no other inbound routing, its work
	// is the poll loops below.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/metrics", metrics.Handler())
	healthSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("worker health endpoint starting", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("worker health endpoint failed", "error", err)
		}
	}()

	logger.Info("postback delivery poll loop starting", "batch_size", postbackPollBatchSize, "idle", postbackPollIdle)
	go deliverer.PollLoop(ctx, postbackPollBatchSize, postbackPollIdle)

	logger.Info("event flush poll loop starting", "batch_size", eventPollBatchSize, "idle", eventPollIdle)
	go flusher.PollLoop(ctx, eventPollBatchSize, eventPollIdle)

	logger.Info("postback attempt log poll loop starting", "batch_size", attemptLogPollBatchSize, "idle", attemptLogPollIdle)
	go attemptFlusher.PollLoop(ctx, attemptLogPollBatchSize, attemptLogPollIdle)

	logger.Info("ad spend sync scheduler starting", "interval", costSyncInterval)
	go costSyncScheduler.RunLoop(ctx, costSyncInterval)

	logger.Info("event queue depth poller starting", "interval", queueDepthPollInterval)
	go eventqueue.PollDepth(ctx, eventQueue, metrics.EventQueueDepth, queueDepthPollInterval, logger)

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return healthSrv.Shutdown(shutdownCtx)
}

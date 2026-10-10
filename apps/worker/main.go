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
// One internal/queuereaper loop per queue returns claims a dead worker left
// 'processing'; on SIGTERM the loops above stop claiming, finish the batch in
// hand, and run() waits for them (audit 2026-10-06, Q1).
//
// §53/Phase 29: GET /metrics (Prometheus) joins /health on this binary's
// bare mux, and a fifth background goroutine (eventqueue.PollDepth)
// refreshes the event_queue_depth gauge on the same time.Ticker shape as
// costSyncScheduler above.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/facebookads"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/googleads"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/tiktokads"
	"github.com/ismagilovnail/flox/apps/internal/alert"
	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/auth"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/costsync"
	"github.com/ismagilovnail/flox/apps/internal/domain"
	"github.com/ismagilovnail/flox/apps/internal/domain/tlsprobe"
	"github.com/ismagilovnail/flox/apps/internal/errtrack"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
	"github.com/ismagilovnail/flox/apps/internal/fx"
	"github.com/ismagilovnail/flox/apps/internal/fx/ecb"
	"github.com/ismagilovnail/flox/apps/internal/integrations"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/offercap"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/queuereaper"
	"github.com/ismagilovnail/flox/apps/internal/queuestats"
	"github.com/ismagilovnail/flox/apps/internal/reattribute"
	"github.com/ismagilovnail/flox/apps/internal/reconcile"
	"github.com/ismagilovnail/flox/apps/internal/retention"
	"github.com/ismagilovnail/flox/apps/internal/sslcheck"
	"github.com/ismagilovnail/flox/apps/internal/telemetry"
	"github.com/ismagilovnail/flox/apps/internal/urlsafety"
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

// reconcileInterval: how often Postgres conversions are checked against
// ClickHouse. Cheap (two grouped reads over three days, by index), and an
// hour is well inside the 2h alert window.
const reconcileInterval = time.Hour

// reattributeInterval/reattributeBatchSize: unknown_click conversions are
// looked up again (internal/reattribute). The first retry is due 30s after
// the postback, so polling faster than that only adds empty reads.
const (
	reattributeInterval  = 15 * time.Second
	reattributeBatchSize = 100
)

// fxSyncInterval: reference rates are published once per working day, so a
// few runs a day pick up each fixing the same afternoon without hammering
// the provider.
const fxSyncInterval = 4 * time.Hour

// shutdownDrainTimeout is how long shutdown waits for in-flight batches.
// Covers a ClickHouse insert (30s cap) but not a full 20-delivery postback
// batch at its worst; anything cut off is returned by the reaper. The
// orchestrator's termination grace period must be longer than this.
const shutdownDrainTimeout = 40 * time.Second

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

	// Error tracking (internal/errtrack): ERROR log records and panics go to
	// the tracker named by SENTRY_DSN; a no-op without it.
	env := cfg.SentryEnvironment
	if env == "" {
		env = cfg.Env
	}
	reporter, err := errtrack.New(errtrack.Config{DSN: cfg.SentryDSN, Environment: env, Service: "worker"})
	if err != nil {
		logger.Error("error tracker configuration failed", "error", err)
		return err
	}
	defer reporter.Flush(2 * time.Second)
	logger = slog.New(errtrack.Handler(logger.Handler(), reporter))

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
	metrics.RegisterPgxPool(db, "worker")

	ch, err := chconn.NewConn(ctx, cfg.ClickHouse)
	if err != nil {
		logger.Error("clickhouse connection failed", "error", err)
		return err
	}
	defer ch.Close()
	if err := chstore.MigrateLocked(ctx, db, ch); err != nil {
		logger.Error("clickhouse schema migration failed", "error", err)
		return err
	}
	// The postback attempt log's TTL (tech audit CH3, docs/retention.md).
	if changed, err := chstore.SetPostbackLogTTL(ctx, ch, cfg.RetentionPostbackLogMonths); err != nil {
		logger.Warn("postback log retention not applied", "error", err)
	} else if changed {
		logger.Info("postback log retention set", "months", cfg.RetentionPostbackLogMonths)
	}
	chEvents := chstore.NewEventStore(ch)
	// One-time copies of existing data into tables added by later schema
	// files (click_lookup, audit 2026-10-06 CH1). In the background: copying
	// a large history must not delay event flushing, and until it is done
	// attribution falls back to the old scan. Interrupted, it reruns on the
	// next start.
	go func() {
		start := time.Now()
		if err := chstore.Backfill(ctx, ch); err != nil {
			if ctx.Err() == nil {
				logger.Error("clickhouse backfill failed, will retry on next start", "error", err)
			}
			return
		}
		logger.Info("clickhouse backfills done", "took", time.Since(start))
	}()

	// Same "fail loudly at boot" bar as apps/api's own required-secret
	// parses — a misconfigured/missing key must never silently run the
	// Google Ads sync with a well-known key.
	adAccountKey, err := adaccount.ParseKey(cfg.AdAccountEncryptionKey)
	if err != nil {
		logger.Error("ad account encryption key configuration failed", "error", err)
		return err
	}
	// Reference FX rates: without them every non-USD payout and spend has no
	// USD value and drops out of Revenue/Cost/ROI (audit 2026-10-06, AN1).
	// Same "fail loudly at boot" bar as the key above: a typo must not
	// silently leave every non-USD amount without a USD value.
	var fxProvider fx.Provider
	switch cfg.FXProvider {
	case "ecb":
		fxProvider = ecb.New()
	case "none":
		logger.Warn("FX_PROVIDER=none: fx_rates is not synced, non-USD amounts get a USD value only if rates are loaded by other means")
	default:
		err := fmt.Errorf("unknown FX_PROVIDER %q (want ecb or none)", cfg.FXProvider)
		logger.Error("fx provider configuration failed", "error", err)
		return err
	}

	adAccountRepo := adaccount.NewRepository(db, adAccountKey)
	costSvc := cost.NewService(cost.NewRepository(db), conversion.NewPostgresFX(db))
	costSyncSvc := costsync.NewService(adAccountRepo, campaign.NewRepository(db), costSvc, costsync.Providers{
		FacebookAds: facebookads.New(),
		TikTokAds:   tiktokads.New(),
		GoogleAds:   googleads.New(),
	})
	costSyncScheduler := costsync.NewScheduler(costSyncSvc, adAccountRepo, logger)

	attemptLogQueue := postbacklog.NewPostgresQueue(db, logger)
	// otelhttp.NewTransport gives each outbound postback delivery its own
	// trace (§53's "trace IDs") — there's no inbound request to inherit a
	// trace context from here (this fires from a poll loop, not an HTTP
	// handler), so each call becomes its own root span. A no-op when no
	// OTel endpoint is configured (telemetry.Setup's own doc comment).
	//
	// DialContext: urlsafety.SafeDialContext (§54/Phase 30) — del.URL is
	// built from a network operator's own postback_url template
	// (macro-substituted, apps/internal/macro), so this is the one
	// outbound call in this codebase that fetches an operator-supplied
	// destination. network.Service.isValidURL only ever ran once, at
	// save time; SafeDialContext is the delivery-time recheck that
	// actually stops an SSRF attempt, including one that only appears
	// after the URL was saved (DNS rebinding). A fresh *http.Transport
	// rather than http.DefaultTransport, since DialContext can't be set
	// on the shared package-level default without mutating it for every
	// other caller in this process — the other field values here match
	// http.DefaultTransport's own defaults.
	safeTransport := &http.Transport{
		DialContext:           urlsafety.SafeDialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	postbackClient := &http.Client{Transport: otelhttp.NewTransport(safeTransport)}
	deliveryStore := postback.NewPostgresStore(db)
	deliverer := postback.NewDeliverer(
		deliveryStore,
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
		// Health and metrics only (tech audit RL5).
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  time.Minute,
	}
	go func() {
		logger.Info("worker health endpoint starting", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("worker health endpoint failed", "error", err)
		}
	}()

	// Every loop is tracked so shutdown can wait for in-flight batches:
	// returning from run() while a flusher was mid-insert used to cut its
	// batch off and leave it 'processing' (audit 2026-10-06, Q1).
	var loops sync.WaitGroup
	start := func(fn func()) {
		loops.Add(1)
		go func() {
			defer loops.Done()
			errtrack.Go(reporter, "worker loop", fn) // reports a panic, then crashes as before
		}()
	}

	logger.Info("postback delivery poll loop starting", "batch_size", postbackPollBatchSize, "idle", postbackPollIdle)
	start(func() { deliverer.PollLoop(ctx, postbackPollBatchSize, postbackPollIdle) })

	logger.Info("event flush poll loop starting", "batch_size", eventPollBatchSize, "idle", eventPollIdle)
	start(func() { flusher.PollLoop(ctx, eventPollBatchSize, eventPollIdle) })

	logger.Info("postback attempt log poll loop starting", "batch_size", attemptLogPollBatchSize, "idle", attemptLogPollIdle)
	start(func() { attemptFlusher.PollLoop(ctx, attemptLogPollBatchSize, attemptLogPollIdle) })

	logger.Info("ad spend sync scheduler starting", "interval", costSyncInterval)
	start(func() { costSyncScheduler.RunLoop(ctx, costSyncInterval) })

	logger.Info("event queue depth poller starting", "interval", queueDepthPollInterval)
	start(func() { eventqueue.PollDepth(ctx, eventQueue, metrics.EventQueueDepth, queueDepthPollInterval, logger) })
	// Per-queue open rows and the age of the oldest due row (audit
	// 2026-10-06, §14) — depth alone misses a small stalled queue.
	start(func() { queuestats.Poll(ctx, db, queueDepthPollInterval, logger) })

	// Postgres postbacks vs ClickHouse conversion_events, hourly over the
	// last 3 days (internal/reconcile): every lost, duplicated or mis-summed
	// conversion shows up in flox_reconciliation_mismatched_buckets.
	reconciler := reconcile.New(db, ch, logger)
	logger.Info("postgres/clickhouse reconciliation starting", "interval", reconcileInterval)
	start(func() {
		reconciler.RunLoop(ctx, reconcileInterval, reconcile.Window{Lookback: 72 * time.Hour, Settle: 30 * time.Minute})
	})

	if fxProvider != nil {
		// USD values filled in after the fact are rewritten in ClickHouse
		// for whatever day they belong to (tech audit AN1, docs/fx.md).
		fxSyncer := fx.NewSyncer(fxProvider, fx.NewStore(db), logger).
			OnRepaired(func(ctx context.Context, buckets []fx.Bucket) error {
				bs := make([]reconcile.Bucket, len(buckets))
				for i, b := range buckets {
					bs[i] = reconcile.Bucket{OrganizationID: b.OrganizationID, Day: b.Day, Status: b.Status}
				}
				_, err := reconciler.RepairBuckets(ctx, bs)
				return err
			})
		logger.Info("fx rate sync starting", "provider", cfg.FXProvider, "interval", fxSyncInterval)
		start(func() { fxSyncer.RunLoop(ctx, fxSyncInterval) })
	}

	// unknown_click conversions whose click was not in ClickHouse yet: looked
	// up again for ~7 hours, then rewritten in Postgres and ClickHouse
	// (internal/reattribute, audit 2026-10-06 T1).
	retrier := reattribute.New(db, conversion.NewPostgresStore(db), chstore.NewClickResolver(ch), chEvents, logger)
	logger.Info("attribution retry starting", "interval", reattributeInterval)
	start(func() { retrier.RunLoop(ctx, reattributeInterval, reattributeBatchSize) })

	// Postbacks that were not recorded (duplicate/error/ignored) expire
	// (internal/retention, tech audit DB2).
	if days := cfg.RetentionFailedPostbacksDays; days > 0 {
		expire := &retention.FailedPostbacks{DB: db, Age: time.Duration(days) * 24 * time.Hour, Logger: logger}
		logger.Info("postback retention starting", "older_than_days", days, "interval", retention.Interval)
		start(func() { expire.RunLoop(ctx, retention.Interval) })
	}

	// Expired sessions, sign-in steps, invite and verification tokens and
	// over login-failure windows (internal/retention, tech audit DB5).
	expiredAuth := &retention.ExpiredAuth{DB: db, LoginFailureWindow: auth.LoginFailureWindow, Logger: logger}
	logger.Info("expired auth cleanup starting", "interval", retention.Interval)
	start(func() { expiredAuth.RunLoop(ctx, retention.Interval) })

	// Every domain's SSL status from a real TLS connection to it
	// (internal/sslcheck, marketer audit #4).
	sslChecker := &sslcheck.Checker{Repo: domain.NewRepository(db), Prober: &tlsprobe.Prober{}, Logger: logger}
	logger.Info("domain ssl check starting", "interval", sslcheck.Interval)
	start(func() { sslChecker.RunLoop(ctx, sslcheck.Interval) })

	// Alerts (internal/alert, marketer audit #10): spend, ROI, offer capped
	// or paused, SSL expiring — into the app's notifications and, per rule,
	// Slack. Slack needs the integrations key to read the webhook; without
	// it notifications still appear in the app.
	alertEvaluator := &alert.Evaluator{
		Repo:    alert.NewRepository(db),
		Reports: analytics.NewService(chEvents, costSvc),
		Caps: func(ctx context.Context, now time.Time) ([]offercap.Status, error) {
			return offercap.Statuses(ctx, offercap.NewPostgresSource(db), chEvents, now)
		},
		Logger: logger,
	}
	if cfg.IntegrationEncryptionKey == "" {
		logger.Warn("INTEGRATION_ENCRYPTION_KEY is not set: alert notifications are not sent to Slack")
	} else {
		integrationKey, err := integrations.ParseKey(cfg.IntegrationEncryptionKey)
		if err != nil {
			logger.Error("integration encryption key configuration failed", "error", err)
			return err
		}
		alertEvaluator.Slack = slackPoster{integrations.NewService(integrations.NewRepository(db), integrationKey, nil)}
	}
	logger.Info("alert evaluation starting", "interval", alert.Interval)
	start(func() { alertEvaluator.RunLoop(ctx, alert.Interval) })

	// Backstop for claims a crashed worker (this one's previous process, or
	// another replica) never finished — see internal/queuereaper.
	logger.Info("queue reapers starting", "interval", queuereaper.Interval, "stuck_after", queuereaper.StuckAfter)
	for name, r := range map[string]queuereaper.Reaper{
		"event_queue":            eventQueue,
		"postback_attempt_queue": attemptLogQueue,
		"postback_deliveries":    deliveryStore,
	} {
		start(func() { queuereaper.RunLoop(ctx, name, r, queuereaper.Interval, queuereaper.StuckAfter, logger) })
	}

	<-ctx.Done()
	logger.Info("shutting down, waiting for in-flight batches")

	drained := make(chan struct{})
	go func() {
		loops.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		logger.Info("all worker loops stopped")
	case <-time.After(shutdownDrainTimeout):
		// Whatever is still running is left for the reaper on next start.
		logger.Warn("worker loops still running after drain timeout, exiting anyway", "timeout", shutdownDrainTimeout)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return healthSrv.Shutdown(shutdownCtx)
}

// slackPoster adapts the integrations service to alert.SlackSender.
type slackPoster struct{ svc *integrations.Service }

func (p slackPoster) PostSlack(ctx context.Context, orgID, text string) error {
	err := p.svc.PostSlack(ctx, orgID, text)
	if errors.Is(err, integrations.ErrNotConnected) {
		return alert.ErrSlackNotConnected
	}
	return err
}

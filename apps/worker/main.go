// Command worker is FLOX's background processor: a separate binary from
// apps/api and apps/tracker (same module), so it scales apart from the
// redirect hot path. It runs:
//   - outgoing postback delivery (internal/postback, §46): due
//     postback_deliveries, with backoff and a dead-letter state;
//   - pixel delivery (internal/pixelsend, §29): events sent server-side
//     to the ad platforms of each click's stream set's pixels;
//   - push delivery (internal/pushsend, docs/push.md): broadcasts sent
//     to installed PWAs' subscribers through their push services;
//   - the event pipeline (internal/eventqueue, internal/eventstream; §43,
//     §47): tracker events into ClickHouse's click, tracking and
//     conversion tables (docs/analytics-pipeline.md, docs/event-stream.md);
//   - the postback attempt log (internal/postbacklog, §48) into
//     postback_events;
//   - scheduled jobs: ad spend sync (internal/costsync), FX rates,
//     attribution retries, Postgres/ClickHouse reconciliation, retention,
//     expired auth cleanup, domain SSL checks, alert evaluation;
//   - a reaper per queue (internal/queuereaper) returning claims a dead
//     worker left 'processing'.
//
// On SIGTERM the loops stop claiming, finish the batch in hand, and run()
// waits for them (tech audit Q1). /health and /metrics are served too.
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

	"github.com/jackc/pgx/v5/pgxpool"
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
	"github.com/ismagilovnail/flox/apps/internal/eventstream"
	"github.com/ismagilovnail/flox/apps/internal/fx"
	"github.com/ismagilovnail/flox/apps/internal/fx/ecb"
	"github.com/ismagilovnail/flox/apps/internal/integrations"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/offercap"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/pixelsend"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/push"
	"github.com/ismagilovnail/flox/apps/internal/pushsend"
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

// pixelPollBatchSize/pixelPollIdle: the same shape for pixel deliveries.
const (
	pixelPollBatchSize = 20
	pixelPollIdle      = 5 * time.Second
)

// pushPollBatchSize/pushPollIdle: push deliveries — a broadcast queues a
// row per subscriber at once, so batches are larger.
const (
	pushPollBatchSize = 100
	pushPollIdle      = 2 * time.Second
	// pushScheduleInterval: how often due scheduled broadcasts are looked
	// for — a broadcast goes out at most this late.
	pushScheduleInterval = 15 * time.Second
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
	// files (click_lookup, tech audit CH1). In the background: copying a
	// large history must not delay event flushing, and until it is done
	// attribution scans click_events instead. Interrupted, it reruns on the
	// next start.
	go func() {
		start := time.Now()
		if err := chstore.Backfill(ctx, ch); err != nil {
			if ctx.Err() == nil {
				logger.Error("clickhouse backfill failed, will retry on next start", "error", err)
			}
			return
		}
		// Networks for clicks from before schema/017, from their offer.
		if err := chstore.BackfillNetworks(ctx, ch, func(ctx context.Context) (map[string]string, error) {
			return offerNetworks(ctx, db)
		}); err != nil {
			if ctx.Err() == nil {
				logger.Error("clickhouse network backfill failed, will retry on next start", "error", err)
			}
			return
		}
		// Offer and source on LTV rows from before schema/018.
		if err := chstore.BackfillLTVAttribution(ctx, ch); err != nil {
			if ctx.Err() == nil {
				logger.Error("clickhouse ltv attribution backfill failed, will retry on next start", "error", err)
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
	// Pixels' access tokens, read back to send events (internal/pixelsend).
	pixelKey, err := pixel.ParseKey(cfg.PixelEncryptionKey)
	if err != nil {
		logger.Error("pixel encryption key configuration failed", "error", err)
		return err
	}
	// Reference FX rates: without them every non-USD payout and spend has no
	// USD value and drops out of Revenue/Cost/ROI (tech audit AN1).
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
	// Each outbound postback is its own root trace (§53): it starts in a
	// poll loop, with no inbound request to inherit one from. A no-op
	// without an OTel endpoint.
	//
	// SafeDialContext (§54): the URL comes from a network operator's
	// postback_url template — the one operator-supplied destination this
	// codebase fetches. Validation at save time cannot stop DNS rebinding;
	// this check at dial time does. A fresh Transport, since DialContext
	// cannot be set on http.DefaultTransport without changing it for every
	// caller; the other fields match its defaults.
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
	// Pixels (internal/pixelsend): the platforms' APIs and generic pixel
	// URLs, through the same dial-time SSRF guard as postbacks — a generic
	// pixel's URL is operator-supplied too.
	pixelStore := pixelsend.NewPostgresStore(db)
	pixelDeliverer := pixelsend.NewDeliverer(pixelStore, pixel.NewRepository(db, pixelKey), pixelsend.Senders(postbackClient), logger)
	clickResolver := chstore.NewClickResolver(ch)
	// Push broadcasts (internal/pushsend, docs/push.md): to the push
	// services subscriptions name, through the same dial-time SSRF guard —
	// an endpoint comes from a browser.
	pushStore := pushsend.NewPostgresStore(db)
	var pushDeliverer *pushsend.Deliverer
	if cfg.PushVAPIDPublicKey != "" && cfg.PushVAPIDPrivateKey != "" {
		pushDeliverer = pushsend.NewDeliverer(pushStore, &pushsend.WebPush{
			Client: postbackClient, PublicKey: cfg.PushVAPIDPublicKey, PrivateKey: cfg.PushVAPIDPrivateKey,
			Subject: cfg.PushVAPIDSubject, TTL: 24 * 60 * 60,
		}, chEvents, logger)
	} else {
		logger.Warn("push not configured (PUSH_VAPID_*): broadcasts stay queued")
	}

	// Tracker events reach ClickHouse through this sink, which first
	// queues the ones a pixel reports (PWA_INSTALL) for their pixels.
	eventSink := &pixelsend.TriggerSink{Next: chEvents, Queue: pixelStore, Clicks: clickResolver, Logger: logger}
	eventQueue := eventqueue.NewPostgresQueue(db)
	flusher := eventqueue.NewFlusher(eventQueue, eventSink, logger)
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

	// Every loop is tracked so shutdown waits for in-flight batches; one cut
	// off mid-insert would stay 'processing' (tech audit Q1).
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

	logger.Info("pixel delivery poll loop starting", "batch_size", pixelPollBatchSize, "idle", pixelPollIdle)
	start(func() { pixelDeliverer.PollLoop(ctx, pixelPollBatchSize, pixelPollIdle) })

	if pushDeliverer != nil {
		logger.Info("push delivery poll loop starting", "batch_size", pushPollBatchSize, "idle", pushPollIdle)
		start(func() { pushDeliverer.PollLoop(ctx, pushPollBatchSize, pushPollIdle) })
	}

	// Scheduled push broadcasts go out when due (docs/push.md), configured
	// or not: their deliveries then wait in the queue like any other.
	logger.Info("push scheduler starting", "interval", pushScheduleInterval)
	start(func() { push.RunScheduler(ctx, push.NewRepository(db), pushScheduleInterval, logger) })

	logger.Info("event flush poll loop starting", "batch_size", eventPollBatchSize, "idle", eventPollIdle)
	start(func() { flusher.PollLoop(ctx, eventPollBatchSize, eventPollIdle) })

	logger.Info("postback attempt log poll loop starting", "batch_size", attemptLogPollBatchSize, "idle", attemptLogPollIdle)
	start(func() { attemptFlusher.PollLoop(ctx, attemptLogPollBatchSize, attemptLogPollIdle) })

	logger.Info("ad spend sync scheduler starting", "interval", costSyncInterval)
	start(func() { costSyncScheduler.RunLoop(ctx, costSyncInterval) })

	// The JetStream event stream (internal/eventstream), when configured:
	// tracker events arrive there; event_queue above still carries
	// conversions and whatever the tracker could not publish. The
	// topology is retried in the background so a NATS outage at start
	// delays only this loop.
	if cfg.NATSURL != "" {
		nc, js, err := eventstream.Connect(cfg.NATSURL, "flox-worker", logger)
		if err != nil {
			logger.Error("nats connection failed", "error", err)
			return err
		}
		defer nc.Close()
		start(func() {
			cons, err := eventstream.EnsureTopologyLoop(ctx, js, eventstream.Topology{Replicas: cfg.NATSStreamReplicas, MaxBytes: int64(cfg.NATSStreamMaxMB) << 20}, logger)
			if err != nil {
				return
			}
			logger.Info("nats event stream consumer starting", "stream", eventstream.StreamName, "consumer", eventstream.ConsumerName)
			go eventstream.PollPending(ctx, cons, queueDepthPollInterval, logger)
			eventstream.NewConsumer(cons, eventSink, eventQueue, eventstream.ConsumerConfig{}, logger).Run(ctx)
		})
	} else {
		logger.Warn("NATS_URL not set, events arrive through the postgres event_queue only")
	}

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
	// (internal/reattribute, tech audit T1).
	retrier := reattribute.New(db, conversion.NewPostgresStore(db), clickResolver, chEvents, logger).WithPixels(pixelStore)
	logger.Info("attribution retry starting", "interval", reattributeInterval)
	start(func() { retrier.RunLoop(ctx, reattributeInterval, reattributeBatchSize) })

	// Postbacks that were not recorded (duplicate/error/ignored) expire
	// (internal/retention, tech audit DB2).
	if days := cfg.RetentionFailedPostbacksDays; days > 0 {
		expire := &retention.FailedPostbacks{DB: db, Age: time.Duration(days) * 24 * time.Hour, Logger: logger}
		logger.Info("postback retention starting", "older_than_days", days, "interval", retention.Interval)
		start(func() { expire.RunLoop(ctx, retention.Interval) })
	}

	// Sent and dead-lettered pixel deliveries, which hold visitors' IPs and
	// user agents (internal/retention).
	if days := cfg.RetentionPixelDeliveriesDays; days > 0 {
		expire := &retention.FinishedDeliveries{DB: db, Table: retention.PixelDeliveriesTable, Age: time.Duration(days) * 24 * time.Hour, Logger: logger}
		logger.Info("pixel delivery retention starting", "older_than_days", days, "interval", retention.Interval)
		start(func() { expire.RunLoop(ctx, retention.Interval) })
	}
	// Finished push deliveries; their broadcasts keep the counts.
	if days := cfg.RetentionPushDeliveriesDays; days > 0 {
		expire := &retention.FinishedDeliveries{DB: db, Table: retention.PushDeliveriesTable, Age: time.Duration(days) * 24 * time.Hour, Logger: logger}
		logger.Info("push delivery retention starting", "older_than_days", days, "interval", retention.Interval)
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
		"pixel_deliveries":       pixelStore,
		"push_deliveries":        pushStore,
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

// offerNetworks maps every offer id to its network, across organizations
// — for chstore.BackfillNetworks.
func offerNetworks(ctx context.Context, db *pgxpool.Pool) (map[string]string, error) {
	rows, err := db.Query(ctx, `SELECT id, network_id FROM offers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, network string
		if err := rows.Scan(&id, &network); err != nil {
			return nil, err
		}
		out[id] = network
	}
	return out, rows.Err()
}

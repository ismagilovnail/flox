// Package config loads apps/api's runtime configuration from environment
// variables. Every variable here already exists in .env.example (§7/§17/§33)
// — Phase 16 only wires up the ones this phase's server actually uses
// (HTTP port, log level, OTel). DATABASE_URL/CLICKHOUSE_URL/REDIS_URL/S3_*
// are parsed and carried on Config now so later phases don't touch this
// file again, but nothing connects to them yet — that starts Phase 17+.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// --- App ---
	Env      string // development | staging | production
	HTTPAddr string // host:port the HTTP server listens on
	LogLevel string // debug | info | warn | error
	// AppURL is apps/web's own origin (APP_URL in .env.example) — apps/api
	// uses it as the one allowed CORS origin (Phase 27): the browser calls
	// the control-plane API directly from apps/web, a different origin in
	// dev (localhost:3000 vs :8080), so without this every fetch() would
	// be blocked before ever reaching a handler.
	AppURL string

	// --- SMTP / email ---
	// SMTPURL: apps/internal/mailer.New's input (e.g.
	// "smtp://localhost:1025" in dev, against infra/docker-compose.dev.yml's
	// mailhog catcher). MailFrom: the From header every outbound email
	// (currently just auth's email-verification send) carries.
	SMTPURL  string
	MailFrom string

	// TOTPEncryptionKey is 32 raw bytes, base64-encoded, used by
	// apps/internal/totp to AES-256-GCM encrypt a user's TOTP secret at
	// rest (users.totp_secret) — the one secret in this codebase that
	// must be decryptable, not just hashed, since verifying a live code
	// needs the plaintext seed back. No default: an empty/missing key in
	// a real deployment must fail loudly at startup rather than silently
	// run 2FA with a well-known key.
	TOTPEncryptionKey string

	// IntegrationEncryptionKey is the same shape as TOTPEncryptionKey
	// (32 raw bytes, base64-encoded, AES-256-GCM) but a deliberately
	// separate key — used by apps/internal/integrations to encrypt
	// Settings > Integrations credentials (Namecheap/Cloudflare/Slack) at
	// rest. A real API credential must be decryptable to make a real
	// verify call with it, same reasoning as the TOTP secret; rotating
	// or compromising one key should never force rotating the other.
	IntegrationEncryptionKey string

	// TrackerDNSTarget is the real, publicly-reachable hostname or IP this
	// deployment's apps/tracker is served from — used only by apps/
	// internal/domain's DNS-record automation (§30) when creating/
	// updating a domain's DNS record to point at FLOX. Deliberately no
	// default: this environment has nothing publicly deployed, so a
	// plausible-looking fake value would silently point a real customer's
	// domain at nothing. domain.Service.SyncDNS returns a clear
	// validation error if this is empty rather than proceeding with a
	// fabricated target.
	TrackerDNSTarget string

	// AdAccountEncryptionKey is the same shape again (32 raw bytes,
	// base64-encoded, AES-256-GCM) but its own deliberately separate
	// key — used by apps/internal/adaccount to encrypt Google Ads'
	// developer token/client secret/refresh token at rest. Needed by
	// both apps/api (Connect/Verify) and apps/worker (the ad-spend sync
	// reads these credentials back to call the real Ads API).
	// Facebook/TikTok Ads' own access_token predates this and stays
	// plain text (migration 00018) — this key never touches it.
	AdAccountEncryptionKey string

	// SentryDSN enables error tracking (internal/errtrack) — any tracker
	// that speaks the Sentry protocol. Empty: off. SentryEnvironment labels
	// events; it defaults to Env.
	SentryDSN, SentryEnvironment string

	// FXProvider is where apps/worker pulls reference FX rates from
	// (internal/fx): "ecb" (default) or "none" — offline dev, or rates
	// loaded into fx_rates by other means.
	FXProvider string

	// Retention (apps/worker, internal/retention; tech audit CH3/DB2).
	// FailedPostbacksDays: postbacks rows that were not recorded
	// (duplicate/error/ignored) are deleted after this many days; 0 keeps
	// them. PostbackLogMonths: TTL of ClickHouse's postback_events attempt
	// log; 0 removes it. Successful conversions and the report tables are
	// never expired.
	RetentionFailedPostbacksDays int
	RetentionPostbackLogMonths   int

	// LandingLinkKey is the HMAC key (32+ raw bytes, base64-encoded)
	// apps/tracker signs internal landings' click links with
	// (apps/internal/landinglink). Missing or invalid: the tracker logs a
	// warning at startup and skips internal landing stages — redirecting
	// straight to the flow's destination, exactly as before landings were
	// served — rather than serving pages whose links it can't sign.
	LandingLinkKey string

	// StickyCookieKey is the HMAC key (32+ raw bytes, base64-encoded) the
	// tracker signs sticky cookies with (apps/tracker/sticky.go, tech audit
	// T5). Required by apps/tracker; shared by its replicas and stable:
	// changing it re-buckets every returning visitor.
	StickyCookieKey string

	// TrackerPublicURL is where a browser reaches apps/tracker directly
	// (dev: http://localhost:8081) — used by apps/api only to mint the
	// PWA constructor's test-page links (internal/pwa/testlink.go). Real
	// visitor traffic reaches the tracker on campaign tracking domains,
	// never through this. Empty: test links are unavailable.
	TrackerPublicURL string

	// --- Geo/ASN (apps/internal/classifier/maxmind) ---
	// Local MaxMind GeoLite2 .mmdb file paths — both empty (the default,
	// since no database ships in the repo; MaxMind's EULA requires a free
	// account + license key to download one) means apps/tracker falls back
	// to classifier.NoopGeoProvider/NoopASNProvider, same as before this
	// existed. See docs/geoip.md for how to obtain and mount real
	// databases.
	GeoIPCityDBPath string
	GeoIPASNDBPath  string

	// TrustedProxies is a raw comma-separated TRUSTED_PROXIES value (CIDR
	// blocks or bare IPs), passed to apps/internal/clientip.ParseList by
	// whichever binary constructs a Resolver. Empty means "use
	// clientip.DefaultTrustedProxies" (loopback + RFC 1918/4193 private
	// ranges) — the safe default for a deployment with no reverse proxy
	// in front of it yet: a public peer address is never in that set, so
	// X-Forwarded-For is ignored regardless of TRUSTED_PROXIES until a
	// real proxy's address range is configured here (security audit
	// finding #2).
	TrustedProxies string

	// --- OpenTelemetry ---
	OTelExporterOTLPEndpoint string
	OTelServiceName          string

	// --- Data stores (parsed, not yet connected — Phase 17+) ---
	DatabaseURL string
	ClickHouse  ClickHouseConfig
	RedisURL    string
	S3          S3Config

	// NATSURL turns on the JetStream event stream (internal/eventstream):
	// the tracker publishes its events there and the worker writes them to
	// ClickHouse. Unset, events go through the Postgres event_queue as
	// before. NATSStreamReplicas: 1 for a single server, 3 in a cluster.
	NATSURL            string
	NATSStreamReplicas int
}

type ClickHouseConfig struct {
	URL      string // http:// or https:// (TLS)
	Database string
	User     string
	Password string

	// Limits every query of this binary runs under (internal/chconn; tech
	// audit CH4) — so one heavy report from apps/api cannot hold the
	// ClickHouse that postback attribution needs. Zero: no limit (the
	// server's). Defaults per binary (chLimits), overridden by
	// CLICKHOUSE_MAX_EXECUTION_TIME (seconds) and CLICKHOUSE_MAX_MEMORY_USAGE
	// (bytes).
	MaxExecutionTime time.Duration
	MaxMemoryUsage   int64
}

// chLimits are a binary's default ClickHouse query limits.
type chLimits struct {
	execution time.Duration
	memory    int64
}

var (
	// apps/api: reports. 30 s and 4 GiB — a report needing more is a
	// date range or a breakdown to narrow, and says so (apierror).
	apiCHLimits = chLimits{execution: 30 * time.Second, memory: 4 << 30}
	// apps/tracker: attribution lookups and the offer-cap snapshot —
	// point queries; 10 s is already a ClickHouse in trouble.
	trackerCHLimits = chLimits{execution: 10 * time.Second, memory: 1 << 30}
	// apps/worker: inserts, backfills, the reconciliation — long by
	// nature, bounded by their own batching rather than a timeout.
	workerCHLimits = chLimits{}
)

type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	// PublicEndpoint is what a presigned URL is signed against — the
	// endpoint a *browser* can reach, which is not always Endpoint (the
	// one this Go process itself talks to). In the dev/test docker
	// compose stacks, apps/api/apps/worker reach minio at the in-network
	// name http://minio:9000, but a browser on the host can only reach
	// http://localhost:9000 — infra/docker-compose.*.yml sets
	// S3_PUBLIC_ENDPOINT explicitly for exactly this split. Defaults to
	// Endpoint, which is correct for a real deployment where the S3-
	// compatible endpoint is equally reachable from both sides.
	PublicEndpoint string
}

// Load builds the control-plane API's configuration (apps/api).
func Load() (Config, error) {
	return load("API_URL", "http://localhost:8080", "flox-api", apiCHLimits)
}

// LoadTracker builds the hot-path tracker's configuration (apps/tracker).
// Same shape and same environment variables as the API's — the two
// services differ only in which URL variable supplies their listen port
// and in their default OTel service name, so they share one loader rather
// than each growing a near-identical copy that can drift.
func LoadTracker() (Config, error) {
	return load("TRACKER_URL", "http://localhost:8081", "flox-tracker", trackerCHLimits)
}

// LoadWorker builds the async worker's configuration (apps/worker, Phase
// 24). It has no inbound routing to serve — its HTTP surface is a bare
// health check for orchestration liveness probes — but shares the same
// config shape so it can be wired up identically.
func LoadWorker() (Config, error) {
	return load("WORKER_URL", "http://localhost:8082", "flox-worker", workerCHLimits)
}

func load(portVar, portDefault, defaultServiceName string, ch chLimits) (Config, error) {
	port, err := parsePort(getEnv(portVar, portDefault))
	if err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", portVar, err)
	}

	execSeconds, err := parseNonNegative("CLICKHOUSE_MAX_EXECUTION_TIME", int(ch.execution/time.Second))
	if err != nil {
		return Config{}, err
	}
	memory, err := parseNonNegative("CLICKHOUSE_MAX_MEMORY_USAGE", int(ch.memory))
	if err != nil {
		return Config{}, err
	}
	failedDays, err := parseNonNegative("RETENTION_FAILED_POSTBACKS_DAYS", 90)
	if err != nil {
		return Config{}, err
	}
	logMonths, err := parseNonNegative("RETENTION_POSTBACK_LOG_MONTHS", 13)
	if err != nil {
		return Config{}, err
	}
	natsReplicas, err := parseNonNegative("NATS_STREAM_REPLICAS", 1)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Env:      getEnv("NODE_ENV", "development"),
		HTTPAddr: fmt.Sprintf(":%d", port),
		LogLevel: getEnv("LOG_LEVEL", "info"),
		AppURL:   getEnv("APP_URL", "http://localhost:3000"),

		SMTPURL:  getEnv("SMTP_URL", "smtp://localhost:1025"),
		MailFrom: getEnv("MAIL_FROM", "FLOX <no-reply@floxlink.io>"),

		TOTPEncryptionKey:        getEnv("TOTP_ENCRYPTION_KEY", ""),
		IntegrationEncryptionKey: getEnv("INTEGRATION_ENCRYPTION_KEY", ""),
		AdAccountEncryptionKey:   getEnv("ADACCOUNT_ENCRYPTION_KEY", ""),
		LandingLinkKey:           getEnv("LANDING_LINK_KEY", ""),
		StickyCookieKey:          getEnv("STICKY_COOKIE_KEY", ""),
		FXProvider:               getEnv("FX_PROVIDER", "ecb"),

		RetentionFailedPostbacksDays: failedDays,
		RetentionPostbackLogMonths:   logMonths,
		SentryDSN:                    getEnv("SENTRY_DSN", ""),
		SentryEnvironment:            getEnv("SENTRY_ENVIRONMENT", ""),
		TrackerPublicURL:             getEnv("TRACKER_PUBLIC_URL", ""),
		TrackerDNSTarget:             getEnv("TRACKER_DNS_TARGET", ""),

		GeoIPCityDBPath: getEnv("GEOIP_CITY_DB_PATH", ""),
		GeoIPASNDBPath:  getEnv("GEOIP_ASN_DB_PATH", ""),

		TrustedProxies: getEnv("TRUSTED_PROXIES", ""),

		OTelExporterOTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelServiceName:          getEnv("OTEL_SERVICE_NAME", defaultServiceName),

		DatabaseURL: getEnv("DATABASE_URL", ""),
		ClickHouse: ClickHouseConfig{
			URL:      getEnv("CLICKHOUSE_URL", ""),
			Database: getEnv("CLICKHOUSE_DATABASE", ""),
			User:     getEnv("CLICKHOUSE_USER", ""),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),

			MaxExecutionTime: time.Duration(execSeconds) * time.Second,
			MaxMemoryUsage:   int64(memory),
		},
		RedisURL: getEnv("REDIS_URL", ""),

		NATSURL:            getEnv("NATS_URL", ""),
		NATSStreamReplicas: natsReplicas,
		S3: S3Config{
			Endpoint:        getEnv("S3_ENDPOINT", ""),
			Region:          getEnv("S3_REGION", ""),
			Bucket:          getEnv("S3_BUCKET", ""),
			AccessKeyID:     getEnv("S3_ACCESS_KEY_ID", ""),
			SecretAccessKey: getEnv("S3_SECRET_ACCESS_KEY", ""),
			PublicEndpoint:  getEnv("S3_PUBLIC_ENDPOINT", getEnv("S3_ENDPOINT", "")),
		},
	}, nil
}

// parseNonNegative reads a whole number ≥ 0, fallback when unset.
func parseNonNegative(key string, fallback int) (int, error) {
	raw := getEnv(key, "")
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s: want a whole number ≥ 0, got %q", key, raw)
	}
	return n, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// parsePort extracts the port from a URL like "http://localhost:8080", or
// accepts a bare "8080"/":8080", since API_URL doubles as "what port do I
// bind to" for this single-service phase.
func parsePort(raw string) (int, error) {
	s := raw
	if i := strings.LastIndex(s, ":"); i != -1 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(s, "/")
	port, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("no valid port found in %q", raw)
	}
	return port, nil
}

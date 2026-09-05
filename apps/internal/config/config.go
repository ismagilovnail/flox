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

	// --- Geo/ASN (apps/internal/classifier/maxmind) ---
	// Local MaxMind GeoLite2 .mmdb file paths — both empty (the default,
	// since no database ships in the repo; MaxMind's EULA requires a free
	// account + license key to download one) means apps/tracker falls back
	// to classifier.NoopGeoProvider/NoopASNProvider, same as before this
	// existed. See docs/geoip.md for how to obtain and mount real
	// databases.
	GeoIPCityDBPath string
	GeoIPASNDBPath  string

	// --- OpenTelemetry ---
	OTelExporterOTLPEndpoint string
	OTelServiceName          string

	// --- Data stores (parsed, not yet connected — Phase 17+) ---
	DatabaseURL string
	ClickHouse  ClickHouseConfig
	RedisURL    string
	S3          S3Config
}

type ClickHouseConfig struct {
	URL      string
	Database string
	User     string
	Password string
}

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
	return load("API_URL", "http://localhost:8080", "flox-api")
}

// LoadTracker builds the hot-path tracker's configuration (apps/tracker).
// Same shape and same environment variables as the API's — the two
// services differ only in which URL variable supplies their listen port
// and in their default OTel service name, so they share one loader rather
// than each growing a near-identical copy that can drift.
func LoadTracker() (Config, error) {
	return load("TRACKER_URL", "http://localhost:8081", "flox-tracker")
}

// LoadWorker builds the async worker's configuration (apps/worker, Phase
// 24). It has no inbound routing to serve — its HTTP surface is a bare
// health check for orchestration liveness probes — but shares the same
// config shape so it can be wired up identically.
func LoadWorker() (Config, error) {
	return load("WORKER_URL", "http://localhost:8082", "flox-worker")
}

func load(portVar, portDefault, defaultServiceName string) (Config, error) {
	port, err := parsePort(getEnv(portVar, portDefault))
	if err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", portVar, err)
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

		GeoIPCityDBPath: getEnv("GEOIP_CITY_DB_PATH", ""),
		GeoIPASNDBPath:  getEnv("GEOIP_ASN_DB_PATH", ""),

		OTelExporterOTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelServiceName:          getEnv("OTEL_SERVICE_NAME", defaultServiceName),

		DatabaseURL: getEnv("DATABASE_URL", ""),
		ClickHouse: ClickHouseConfig{
			URL:      getEnv("CLICKHOUSE_URL", ""),
			Database: getEnv("CLICKHOUSE_DATABASE", ""),
			User:     getEnv("CLICKHOUSE_USER", ""),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),
		},
		RedisURL: getEnv("REDIS_URL", ""),
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

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
}

func Load() (Config, error) {
	port, err := parsePort(getEnv("API_URL", "http://localhost:8080"))
	if err != nil {
		return Config{}, fmt.Errorf("parsing API_URL: %w", err)
	}

	return Config{
		Env:      getEnv("NODE_ENV", "development"),
		HTTPAddr: fmt.Sprintf(":%d", port),
		LogLevel: getEnv("LOG_LEVEL", "info"),

		OTelExporterOTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelServiceName:          getEnv("OTEL_SERVICE_NAME", "flox-api"),

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

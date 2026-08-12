// Package logging builds the process-wide slog.Logger (§33). JSON output
// always — this process's logs are meant to be scraped by a log pipeline,
// not read in a terminal.
package logging

import (
	"log/slog"
	"os"
)

func New(level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	}))
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

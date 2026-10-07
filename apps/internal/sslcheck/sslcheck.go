// Package sslcheck is the worker loop that keeps every domain's SSL status
// true to what the domain actually serves (marketer audit 2026-10-06, #4).
// Each tick it claims the domains due for a check, opens a TLS connection
// to each (internal/domain/tlsprobe) and records the outcome.
//
// A domain with a valid certificate is rechecked every few hours, which is
// enough to see its expiry approach; any other domain every few minutes,
// so a domain whose DNS was just pointed turns "issued" soon after the
// proxy issues its certificate — which the check's own connection triggers
// under on-demand TLS (docs/deployment.md).
package sslcheck

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/domain"
)

const (
	Interval    = time.Minute
	IssuedEvery = 6 * time.Hour
	OtherEvery  = 10 * time.Minute
	batchSize   = 50
	parallel    = 8
)

type Checker struct {
	Repo   *domain.Repository
	Prober domain.SSLProber
	Logger *slog.Logger
}

// RunOnce checks every domain currently due and returns how many it
// checked.
func (c *Checker) RunOnce(ctx context.Context) (int, error) {
	due, err := c.Repo.ClaimSSLChecks(ctx, batchSize, IssuedEvery, OtherEvery)
	if err != nil {
		return 0, err
	}
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for _, d := range due {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res := domain.ProbeSSL(ctx, c.Prober, d.Domain)
			if _, err := c.Repo.ApplySSLCheck(ctx, d.OrganizationID, d.ID, res); err != nil {
				c.Logger.Error("recording ssl check", "domain", d.Domain, "error", err)
			}
		}()
	}
	wg.Wait()
	return len(due), nil
}

func (c *Checker) RunLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		// A full batch means more are due: go again without waiting.
		for {
			n, err := c.RunOnce(ctx)
			if err != nil {
				if ctx.Err() == nil {
					c.Logger.Error("ssl check", "error", err)
				}
				break
			}
			if n < batchSize {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

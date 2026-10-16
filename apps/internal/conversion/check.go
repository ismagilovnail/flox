package conversion

import (
	"context"
	"errors"
	"fmt"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

// Previewer answers what Store.Record would do with an accepted postback's
// entry — applied, a duplicate, or refused under the click's lock — and
// writes nothing. PostgresStore.Preview runs the very write Record runs
// and rolls it back, so the answer cannot drift from the real one.
type Previewer interface {
	Preview(ctx context.Context, e Entry) (Recorded, error)
}

// ErrNoPreview is returned by Check on a Service built without WithPreview.
var ErrNoPreview = errors.New("conversion: dry run is not set up")

// WithPreview returns the service set up for Check.
func (s *Service) WithPreview(p Previewer) *Service {
	s.preview = p
	return s
}

// CheckResult is what FLOX would do with a postback (Service.Check).
type CheckResult struct {
	// Result is the outcome Record would report; its ID is always empty,
	// nothing being written.
	Result
	// Revenue, Currency and USDValue are the money the conversion would
	// carry: Currency the network's when the postback sent none
	// (CurrencyAssumed), USDValue nil when there is no rate for the day.
	Revenue         *float64
	Currency        string
	CurrencyAssumed bool
	USDValue        *float64
	// OutgoingURL is the postback that would be queued to the network's
	// outgoing URL — empty when there is none or the postback is not
	// applied.
	OutgoingURL string
}

// Check is a dry run of Record: it takes p through the same steps — the
// event mapping, the progression rule, the dedup key, attribution, FX —
// and says what would happen, writing nothing: no ledger row, no CPA
// event, no outgoing postback, no pixel, no attempt log, no cache (audit
// 2026-10-15, UX-3). An operator checks a network's postback URL with it
// before the network sends real ones.
func (s *Service) Check(ctx context.Context, p Postback) (CheckResult, error) {
	if s.preview == nil {
		return CheckResult{}, ErrNoPreview
	}
	pv := &previewer{s: s}
	res, err := s.run(ctx, p, pv)
	if err != nil {
		return CheckResult{}, err
	}
	pv.check.Result = res
	return pv.check, nil
}

// previewer is Check's outcome: it asks Previewer instead of Store and
// keeps what the entry would carry.
type previewer struct {
	s     *Service
	check CheckResult
}

func (pv *previewer) refuse(_ context.Context, p Postback, _ string, status event.Type, _ string, kind ResultKind, message string) (Result, error) {
	if kind != ResultError {
		pv.money(p, p.Revenue, nil)
	}
	return Result{Kind: kind, Status: status, Message: refusalMessage(p, kind, message)}, nil
}

func (pv *previewer) commit(ctx context.Context, p Postback, entry Entry, attr attribution.Attribution) (Result, error) {
	rec, err := pv.s.preview.Preview(ctx, entry)
	if err != nil {
		return Result{}, fmt.Errorf("conversion: previewing: %w", err)
	}
	pv.money(p, entry.Revenue, entry.USDValue)
	if rec.Kind == ResultSuccess && entry.Delivery != nil {
		pv.check.OutgoingURL = entry.Delivery.URL
	}
	return Result{Kind: rec.Kind, Status: entry.Status, Attribution: attr, Message: committedMessage(p, entry, rec)}, nil
}

func (pv *previewer) money(p Postback, revenue, usdValue *float64) {
	pv.check.Revenue = revenue
	pv.check.Currency = p.Currency
	pv.check.CurrencyAssumed = p.currencyAssumed
	pv.check.USDValue = usdValue
}

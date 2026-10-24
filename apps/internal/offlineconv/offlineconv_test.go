package offlineconv

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/googleads"
	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// --- fakes ---

// clicks is a ClickResolver over a few clicks, and the attribution of a
// postback to the click it names.
type clicks map[string]attribution.Click

func (c clicks) ByClickID(_ context.Context, _, clickID string) (attribution.Click, error) {
	if click, ok := c[clickID]; ok {
		return click, nil
	}
	return attribution.Click{}, attribution.ErrClickNotFound
}

func (c clicks) ByExternalClickID(context.Context, string, string) ([]attribution.Click, error) {
	return nil, nil
}

func (c clicks) AttributeConversion(_ context.Context, conv attribution.Conversion) (attribution.Attribution, error) {
	click, ok := c[conv.ClickID]
	if !ok {
		return attribution.Attribution{Outcome: attribution.OutcomeUnknownClick}, nil
	}
	return attribution.Attribution{Outcome: attribution.OutcomeAttributed, Method: attribution.MethodClickID, Click: click}, nil
}

type mapper map[string]event.Type

func (m mapper) MapStatus(_ context.Context, _, _, raw string) (event.Type, error) {
	if s, ok := m[raw]; ok {
		return s, nil
	}
	return "", conversion.ErrUnmapped
}

type usd struct{}

func (usd) ToUSD(_ context.Context, _ string, amount float64, _ time.Time) (float64, bool, error) {
	return amount / 100, true, nil
}

type noAttempts struct{}

func (noAttempts) LogAttempt(context.Context, conversion.AttemptRecord) {}

type creds struct{}

func (creds) CredentialsByTrafficSourceID(context.Context, string, string) (adaccount.Credentials, error) {
	return adaccount.Credentials{AdAccountID: "123-456-7890", RefreshToken: "r"}, nil
}

// google records what it was sent; an event with gbraid "BAD" makes it
// refuse the whole request, and down makes it answer like a server error.
type google struct {
	mu       sync.Mutex
	requests [][]googleads.Event
	dests    []googleads.Destination
	down     bool
}

func (g *google) Ingest(_ context.Context, _ adaccount.Credentials, dest googleads.Destination, events []googleads.Event, _ bool) (googleads.IngestResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, events)
	g.dests = append(g.dests, dest)
	if g.down {
		return googleads.IngestResult{}, &googleads.IngestError{Status: 503, Message: "unavailable"}
	}
	for _, e := range events {
		if e.GBRAID == "BAD" {
			return googleads.IngestResult{}, &googleads.IngestError{Status: 400, Message: "INVALID_ARGUMENT: bad gbraid", Permanent: true}
		}
	}
	return googleads.IngestResult{RequestID: "req"}, nil
}

// sentByTxn: the last event sent per transaction id.
func (g *google) sentByTxn() map[string]googleads.Event {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[string]googleads.Event{}
	for _, r := range g.requests {
		for _, e := range r {
			out[e.TransactionID] = e
		}
	}
	return out
}

// --- seed ---

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// Audit UNI-2 part 2: conversions of a Google Ads source's clicks with a
// Google click id reach Google once each, as the action named for their
// status; everything else is skipped with its reason; a refused event
// fails alone; a refund restates the purchase under the same transaction;
// an outage is retried. On the real Postgres, postbacks recorded by the
// real conversion engine.
func TestOfflineConversionsEndToEnd(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := idgen.New()
	exec(t, pool, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, "Org "+orgID)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })

	googleSource, fbSource := idgen.New(), idgen.New()
	exec(t, pool, `INSERT INTO traffic_sources (id, organization_id, name, type, cost_integration) VALUES
		($1, $3, 'Google', 'Google', 'google_ads'), ($2, $3, 'Facebook', 'Facebook', 'none')`, googleSource, fbSource, orgID)
	exec(t, pool, `INSERT INTO ad_account_connections (id, organization_id, traffic_source_id, ad_account_id, refresh_token)
		VALUES ($1, $2, $3, '123-456-7890', 'ciphertext')`, idgen.New(), orgID, googleSource)
	googleCampaign, fbCampaign := idgen.New(), idgen.New()
	exec(t, pool, `INSERT INTO campaigns (id, organization_id, traffic_source_id, name) VALUES
		($1, $3, $4, 'G'), ($2, $3, $5, 'F')`, googleCampaign, fbCampaign, orgID, googleSource, fbSource)
	networkID := idgen.New()
	exec(t, pool, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'Shop')`, networkID, orgID)

	gclidClick, plainClick, fbClick, badClick, earlyClick := idgen.New(), idgen.New(), idgen.New(), idgen.New(), idgen.New()
	cl := clicks{
		gclidClick: {ClickID: gclidClick, CampaignID: googleCampaign, AdClickParam: "gclid", AdClickID: "Cj0K"},
		plainClick: {ClickID: plainClick, CampaignID: googleCampaign},
		fbClick:    {ClickID: fbClick, CampaignID: fbCampaign, AdClickParam: "gclid", AdClickID: "Cj0F"},
		badClick:   {ClickID: badClick, CampaignID: googleCampaign, AdClickParam: "gbraid", AdClickID: "BAD"},
		earlyClick: {ClickID: earlyClick, CampaignID: googleCampaign, AdClickParam: "gclid", AdClickID: "Cj0E"},
	}
	engine := conversion.NewService(mapper{"lead": event.CpaHold, "sale": event.CpaAccept, "rebuy": event.CpaRedep, "refund": event.CpaDecline},
		conversion.NewPostgresStore(pool), usd{}, cl, noAttempts{})
	record := func(clickID, raw, txn string, revenue, refund *float64) string {
		t.Helper()
		p := conversion.Postback{OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, RawStatus: raw,
			NetworkTxnID: txn, Revenue: revenue, RefundAmount: refund, OccurredAt: time.Now().UTC()}
		if revenue != nil || refund != nil {
			p.Currency = "RUB"
		}
		res, err := engine.Record(ctx, p)
		if err != nil || res.Kind != conversion.ResultSuccess {
			t.Fatalf("%s %s: %+v %v", clickID, raw, res, err)
		}
		var id string
		if err := pool.QueryRow(ctx, `SELECT id FROM postbacks WHERE organization_id = $1 AND click_id = $2 AND raw_status = $3
			ORDER BY id DESC LIMIT 1`, orgID, clickID, raw).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f := func(v float64) *float64 { return &v }

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := NewRepository(pool)
	repo.onlyOrg = orgID
	g := &google{}
	svc := NewService(repo, cl, creds{}, g, logger)

	// Before the export is on: never sent.
	record(earlyClick, "sale", "", f(100), nil)

	// Settings: an action to turn it on, digits only.
	if _, err := svc.Update(ctx, orgID, googleSource, Settings{Enabled: true}); err == nil {
		t.Fatal("turned on without an action")
	}
	if _, err := svc.Update(ctx, orgID, googleSource, Settings{Enabled: true, PurchaseActionID: "abc"}); err == nil {
		t.Fatal("took a non-numeric action id")
	}
	if _, err := svc.Update(ctx, orgID, fbSource, Settings{Enabled: true, PurchaseActionID: "1"}); err == nil {
		t.Fatal("turned on for a source without a Google Ads connection")
	}
	if _, err := svc.Update(ctx, orgID, googleSource, Settings{Enabled: true, LeadActionID: "111", PurchaseActionID: "222", LoginAccountID: "999-888-7777"}); err != nil {
		t.Fatal(err)
	}

	lead := record(gclidClick, "lead", "", nil, nil)
	sale := record(gclidClick, "sale", "", f(1000), nil)
	record(gclidClick, "rebuy", "R-2", f(400), nil) // no repeat purchase action: skipped
	record(plainClick, "sale", "", f(10), nil)      // no Google click id
	record(fbClick, "sale", "", f(10), nil)         // another source
	bad := record(badClick, "sale", "", f(10), nil) // Google refuses it

	if err := svc.Enqueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.Send(ctx); err != nil {
		t.Fatal(err)
	}
	sent := g.sentByTxn()
	if len(sent) != 3 { // the lead, the sale, and the refused one tried
		t.Fatalf("sent %d transactions: %+v", len(sent), sent)
	}
	if e := sent[sale]; e.GCLID != "Cj0K" || e.Value == nil || *e.Value != 1000 || e.Currency != "RUB" {
		t.Fatalf("the sale went as %+v", e)
	}
	if e := sent[lead]; e.GCLID != "Cj0K" || e.Value != nil {
		t.Fatalf("the lead went as %+v", e)
	}
	for _, d := range g.dests {
		if d.AccountID != "123-456-7890" || d.LoginAccountID != "999-888-7777" || (d.ActionID != "111" && d.ActionID != "222") {
			t.Fatalf("destination %+v", d)
		}
	}

	st, err := svc.Get(ctx, orgID, googleSource)
	if err != nil {
		t.Fatal(err)
	}
	if st.Counts[StatusSent] != 2 || st.Counts[StatusFailed] != 1 || st.Skipped[SkipNoAction] != 1 || st.Skipped[SkipNoGoogleClick] != 1 {
		t.Fatalf("status = %+v", st)
	}
	var fbReason string
	if err := pool.QueryRow(ctx, `SELECT u.skip_reason FROM offline_conversion_uploads u JOIN postbacks p ON p.id = u.postback_id
		WHERE p.click_id = $1`, fbClick).Scan(&fbReason); err != nil || fbReason != SkipNotConnected {
		t.Fatalf("the other source's sale: %q %v", fbReason, err)
	}
	var failedMsg string
	if err := pool.QueryRow(ctx, `SELECT last_error FROM offline_conversion_uploads WHERE postback_id = $1 AND status = 'failed'`, bad).Scan(&failedMsg); err != nil {
		t.Fatalf("the refused sale is not failed: %v", err)
	}

	// Nothing new: nothing sent again.
	before := len(g.requests)
	if err := svc.Enqueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.Send(ctx); err != nil {
		t.Fatal(err)
	}
	if len(g.requests) != before {
		t.Fatalf("a second run sent %d more requests", len(g.requests)-before)
	}

	// A partial refund restates the sale under its transaction id; an
	// outage leaves it pending to try again.
	record(gclidClick, "refund", "", nil, f(250))
	g.down = true
	if err := svc.Enqueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.Send(ctx); err != nil {
		t.Fatal(err)
	}
	var status, lastErr string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT status, attempts, last_error FROM offline_conversion_uploads WHERE postback_id = $1`, sale).
		Scan(&status, &attempts, &lastErr); err != nil || status != StatusPending || attempts != 1 || lastErr == "" {
		t.Fatalf("after an outage: %s, %d attempts, %q (%v); want pending, 1, the error", status, attempts, lastErr, err)
	}
	g.down = false
	exec(t, pool, `UPDATE offline_conversion_uploads SET next_attempt_at = now() WHERE organization_id = $1`, orgID)
	if err := svc.Send(ctx); err != nil {
		t.Fatal(err)
	}
	if e := g.sentByTxn()[sale]; e.Value == nil || *e.Value != 750 || e.GCLID != "Cj0K" {
		t.Fatalf("the restated sale went as %+v, want 750 under the same transaction", e)
	}
	var sentValue float64
	if err := pool.QueryRow(ctx, `SELECT status, sent_value FROM offline_conversion_uploads WHERE postback_id = $1`, sale).
		Scan(&status, &sentValue); err != nil || status != StatusSent || sentValue != 750 {
		t.Fatalf("after the restatement: %s %v (%v)", status, sentValue, err)
	}
}

func TestRetryDelay(t *testing.T) {
	if retryDelay(0) != time.Minute || retryDelay(3) != 8*time.Minute || retryDelay(20) != 6*time.Hour {
		t.Fatal(retryDelay(0), retryDelay(3), retryDelay(20))
	}
}

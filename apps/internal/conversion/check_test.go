package conversion_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	pbstore "github.com/ismagilovnail/flox/apps/internal/postback"
)

func TestCheckWithoutPreviewIsAnError(t *testing.T) {
	svc, mapper, _, _, _ := newHarness()
	mapper.set("net-1", "ftd", event.CpaAccept)
	if _, err := svc.Check(ctx(), postback("net-1", "click-1", "ftd")); !errors.Is(err, conversion.ErrNoPreview) {
		t.Fatalf("Check without WithPreview: err = %v, want ErrNoPreview", err)
	}
}

// TestCheckSaysWhatRecordWouldDoAndWritesNothing runs the dry run against
// the real schema: every answer Record would give — applied, duplicate,
// progression refusal, a decline reversing the accept, an unmapped
// status — and after each one the click has exactly the rows it had
// before: no ledger row, no CPA event, no outgoing postback, no attempt.
func TestCheckSaysWhatRecordWouldDoAndWritesNothing(t *testing.T) {
	pool := mustPool(t)
	c := context.Background()
	orgID := seedOrg(t, c, pool)
	networkID := seedNetwork(t, c, pool, orgID, false)
	if _, err := pool.Exec(c, `UPDATE networks SET postback_url = 'https://partner.example/pb?cid={click_id}&s={status}' WHERE id = $1`, networkID); err != nil {
		t.Fatalf("setting outgoing URL: %v", err)
	}
	seedEventMapping(t, c, pool, orgID, networkID, "lead", event.CpaHold)
	seedEventMapping(t, c, pool, orgID, networkID, "ftd", event.CpaAccept)
	seedEventMapping(t, c, pool, orgID, networkID, "chargeback", event.CpaDecline)
	at := time.Now().UTC().Add(-time.Hour)
	seedFXRate(t, c, pool, "XTS", at, 0.5)

	store := conversion.NewPostgresStore(pool).WithDeliveries(pbstore.NewPostgresStore(pool))
	attempts := &fakeAttempts{}
	clickID := idgen.New()
	attr := &fakeAttribution{outcome: attribution.OutcomeAttributed, click: attribution.Click{
		ClickID: clickID, OrganizationID: orgID, CampaignID: "campaign-1", OccurredAt: at.Add(-time.Hour),
	}}
	svc := conversion.NewService(conversion.NewPostgresMapper(pool), store, conversion.NewPostgresFX(pool), attr, attempts).
		WithPreview(store)

	pb := func(raw string) conversion.Postback {
		rev := 40.0
		return conversion.Postback{
			OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, RawStatus: raw,
			Revenue: &rev, NetworkCurrency: "XTS",
			PostbackURL: "https://partner.example/pb?cid={click_id}&s={status}", OccurredAt: at,
		}
	}
	check := func(raw string) conversion.CheckResult {
		t.Helper()
		before := clickRows(t, c, pool, orgID, clickID)
		res, err := svc.Check(c, pb(raw))
		if err != nil {
			t.Fatalf("Check(%s): %v", raw, err)
		}
		// A dev worker may consume event_queue meanwhile, so only growth
		// counts as a write.
		after := clickRows(t, c, pool, orgID, clickID)
		for i := range after {
			if after[i] > before[i] {
				t.Fatalf("Check(%s) wrote: rows %v → %v", raw, before, after)
			}
		}
		if res.ID != "" {
			t.Fatalf("Check(%s): ID = %q, want none", raw, res.ID)
		}
		return res
	}

	res := check("ftd")
	if res.Kind != conversion.ResultSuccess || res.Status != event.CpaAccept {
		t.Fatalf("first ftd: %s %s, want success CPA_ACCEPT", res.Kind, res.Status)
	}
	if res.Currency != "XTS" || !res.CurrencyAssumed || res.USDValue == nil || !near(*res.USDValue, 20) {
		t.Fatalf("money: currency %q assumed %v usd %v, want XTS assumed 20", res.Currency, res.CurrencyAssumed, res.USDValue)
	}
	if res.Attribution.Click.CampaignID != "campaign-1" {
		t.Fatalf("attribution: %+v", res.Attribution)
	}
	if want := "https://partner.example/pb?cid=" + clickID + "&s="; !strings.HasPrefix(res.OutgoingURL, want) {
		t.Fatalf("outgoing URL = %q, want it to start %q", res.OutgoingURL, want)
	}

	if _, err := svc.Record(c, pb("ftd")); err != nil {
		t.Fatalf("Record(ftd): %v", err)
	}
	if res := check("ftd"); res.Kind != conversion.ResultDuplicate || res.OutgoingURL != "" {
		t.Fatalf("ftd again: %s, outgoing %q — want duplicate and nothing sent", res.Kind, res.OutgoingURL)
	}
	if res := check("lead"); res.Kind != conversion.ResultIgnored || !strings.Contains(res.Message, "back to CPA_HOLD") {
		t.Fatalf("lead after ftd: %s %q, want the progression refusal", res.Kind, res.Message)
	}
	if res := check("chargeback"); res.Kind != conversion.ResultSuccess || !strings.Contains(res.Message, "reverses the CPA_ACCEPT payout") {
		t.Fatalf("chargeback: %s %q, want success reversing the accept", res.Kind, res.Message)
	}
	if res := check("approved"); res.Kind != conversion.ResultError || !strings.Contains(res.Message, "no event mapping") {
		t.Fatalf("unmapped: %s %q, want the mapping error", res.Kind, res.Message)
	}

	if n := attempts.count(); n != 1 {
		t.Fatalf("attempts logged = %d, want 1 (the Record only)", n)
	}
}

// clickRows counts what a postback for clickID can leave behind: ledger
// rows, queued CPA events, outgoing postbacks.
func clickRows(t *testing.T, c context.Context, pool *pgxpool.Pool, orgID, clickID string) [3]int {
	t.Helper()
	var n [3]int
	if err := pool.QueryRow(c, `
		SELECT (SELECT count(*) FROM postbacks WHERE organization_id = $1 AND click_id = $2),
		       (SELECT count(*) FROM event_queue WHERE payload->>'clickId' = $2),
		       (SELECT count(*) FROM postback_deliveries WHERE organization_id = $1 AND click_id = $2)`,
		orgID, clickID,
	).Scan(&n[0], &n[1], &n[2]); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	return n
}

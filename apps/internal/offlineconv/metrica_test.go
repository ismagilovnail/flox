package offlineconv

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/yandexmetrica"
)

// metrica records the files it took (and counts every call); a row with
// yclid "BAD" makes it refuse the whole file, down makes it answer like a
// server error, and status is what Check answers per upload id
// (PROCESSED when not set).
type metrica struct {
	mu     sync.Mutex
	files  [][]yandexmetrica.Conversion
	calls  int
	down   bool
	status map[int64]string
}

func (m *metrica) Counter(_ context.Context, token, counterID string) error {
	switch {
	case token != "good-token":
		return &yandexmetrica.Error{Status: 403, Message: "invalid_token: bad token"}
	case counterID != "4242":
		return &yandexmetrica.Error{Status: 404, Message: "not_found"}
	}
	return nil
}

func (m *metrica) Upload(_ context.Context, token, counterID string, rows []yandexmetrica.Conversion) (yandexmetrica.Uploading, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if token != "good-token" || counterID != "4242" {
		return yandexmetrica.Uploading{}, &yandexmetrica.Error{Status: 403, Message: "access_denied"}
	}
	m.calls++
	if m.down {
		return yandexmetrica.Uploading{}, &yandexmetrica.Error{Status: 503, Message: "backend_error"}
	}
	for _, r := range rows {
		if r.Yclid == "BAD" {
			return yandexmetrica.Uploading{}, &yandexmetrica.Error{Status: 400, Message: "invalid_uploading: bad yclid", Permanent: true}
		}
	}
	m.files = append(m.files, rows)
	return yandexmetrica.Uploading{ID: int64(len(m.files)), Status: "UPLOADED", SourceQuantity: len(rows), LineQuantity: len(rows)}, nil
}

func (m *metrica) Check(_ context.Context, _, _ string, id int64) (yandexmetrica.Uploading, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.status[id]; ok {
		return yandexmetrica.Uploading{ID: id, Status: st}, nil
	}
	return yandexmetrica.Uploading{ID: id, Status: yandexmetrica.StatusProcessed}, nil
}

// sentByYclid: the rows sent per yclid, in order.
func (m *metrica) sentByYclid() map[string][]yandexmetrica.Conversion {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string][]yandexmetrica.Conversion{}
	for _, f := range m.files {
		for _, r := range f {
			out[r.Yclid] = append(out[r.Yclid], r)
		}
	}
	return out
}

// Audit UNI-2 part 3: a source's Metrica connection is checked with
// Metrica and keeps its token encrypted; conversions of its yclid clicks
// reach the counter as the goal named for their status, priced in the
// counter's currency; other clicks and old visits are skipped; a refused
// file is halved until only the bad row fails; a refund is not sent
// again; Check fails a file Metrica matched none of.
func TestMetricaOfflineConversionsEndToEnd(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := idgen.New()
	exec(t, pool, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, "Org "+orgID)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })

	direct := idgen.New()
	exec(t, pool, `INSERT INTO traffic_sources (id, organization_id, name, type, cost_integration) VALUES ($1, $2, 'Direct', 'Yandex', 'none')`, direct, orgID)
	campaign := idgen.New()
	exec(t, pool, `INSERT INTO campaigns (id, organization_id, traffic_source_id, name) VALUES ($1, $2, $3, 'D')`, campaign, orgID, direct)
	networkID := idgen.New()
	exec(t, pool, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'Shop')`, networkID, orgID)

	now := time.Now().UTC()
	yc, usdClick, plain, gclick, old, bad, late := idgen.New(), idgen.New(), idgen.New(), idgen.New(), idgen.New(), idgen.New(), idgen.New()
	cl := clicks{
		yc:       {ClickID: yc, CampaignID: campaign, AdClickParam: "yclid", AdClickID: "111", OccurredAt: now.Add(-time.Hour)},
		usdClick: {ClickID: usdClick, CampaignID: campaign, AdClickParam: "yclid", AdClickID: "222", OccurredAt: now.Add(-time.Hour)},
		plain:    {ClickID: plain, CampaignID: campaign, OccurredAt: now.Add(-time.Hour)},
		gclick:   {ClickID: gclick, CampaignID: campaign, AdClickParam: "gclid", AdClickID: "Cj0", OccurredAt: now.Add(-time.Hour)},
		old:      {ClickID: old, CampaignID: campaign, AdClickParam: "yclid", AdClickID: "333", OccurredAt: now.Add(-30 * 24 * time.Hour)},
		bad:      {ClickID: bad, CampaignID: campaign, AdClickParam: "yclid", AdClickID: "BAD", OccurredAt: now.Add(-time.Hour)},
		late:     {ClickID: late, CampaignID: campaign, AdClickParam: "yclid", AdClickID: "444", OccurredAt: now.Add(-time.Hour)},
	}
	engine := conversion.NewService(mapper{"lead": event.CpaHold, "sale": event.CpaAccept, "rebuy": event.CpaRedep, "refund": event.CpaDecline},
		conversion.NewPostgresStore(pool), usd{}, cl, noAttempts{})
	record := func(clickID, raw, currency string, revenue, refund *float64) {
		t.Helper()
		p := conversion.Postback{OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, RawStatus: raw,
			Revenue: revenue, RefundAmount: refund, OccurredAt: now}
		if revenue != nil || refund != nil {
			p.Currency = currency
		}
		if res, err := engine.Record(ctx, p); err != nil || res.Kind != conversion.ResultSuccess {
			t.Fatalf("%s %s: %+v %v", clickID, raw, res, err)
		}
	}
	f := func(v float64) *float64 { return &v }

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := NewRepository(pool).WithKey(key)
	repo.onlyOrg = orgID
	m := &metrica{status: map[int64]string{}}
	svc := NewService(repo, cl, creds{}, &google{}, logger).WithMetrica(m)

	// Not connected: defaults, no error.
	st, err := svc.GetMetrica(ctx, orgID, direct)
	if err != nil || st.Connected || st.Currency != "RUB" {
		t.Fatalf("before connecting: %+v %v", st, err)
	}
	if _, err := svc.GetMetrica(ctx, idgen.New(), direct); err == nil {
		t.Fatal("another organization read the source's Metrica settings")
	}

	fieldErr := func(in MetricaInput, field string) {
		t.Helper()
		_, err := svc.PutMetrica(ctx, orgID, direct, in)
		apiErr, ok := err.(*apierror.Error)
		if !ok || apiErr.Fields[field] == "" {
			t.Fatalf("%+v: %v, want a %s field error", in, err, field)
		}
	}
	fieldErr(MetricaInput{MetricaSettings: MetricaSettings{CounterID: "4242"}}, "token")
	fieldErr(MetricaInput{Token: "good-token", MetricaSettings: MetricaSettings{CounterID: "abc"}}, "counterId")
	fieldErr(MetricaInput{Token: "good-token", MetricaSettings: MetricaSettings{CounterID: "4242", Enabled: true}}, "purchaseGoal")
	fieldErr(MetricaInput{Token: "good-token", MetricaSettings: MetricaSettings{CounterID: "4242", LeadGoal: "a b"}}, "leadGoal")
	fieldErr(MetricaInput{Token: "wrong-token", MetricaSettings: MetricaSettings{CounterID: "4242"}}, "token")
	fieldErr(MetricaInput{Token: "good-token", MetricaSettings: MetricaSettings{CounterID: "777"}}, "counterId")

	st, err = svc.PutMetrica(ctx, orgID, direct, MetricaInput{Token: "good-token", MetricaSettings: MetricaSettings{
		Enabled: true, CounterID: "4242", Currency: "rub", LeadGoal: "lead", PurchaseGoal: "purchase"}})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Connected || st.TokenPreview != "oken" || st.Currency != "RUB" || st.Since == nil {
		t.Fatalf("after connecting: %+v", st)
	}
	raw, _ := json.Marshal(st)
	var stored string
	if err := pool.QueryRow(ctx, `SELECT access_token FROM metrica_connections WHERE traffic_source_id = $1`, direct).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "good-token") || strings.Contains(stored, "good-token") {
		t.Fatalf("the token is readable: answer %s, stored %q", raw, stored)
	}
	// Saving again without the token keeps it.
	if _, err := svc.PutMetrica(ctx, orgID, direct, MetricaInput{MetricaSettings: MetricaSettings{
		Enabled: true, CounterID: "4242", Currency: "RUB", LeadGoal: "lead", PurchaseGoal: "purchase"}}); err != nil {
		t.Fatal(err)
	}

	record(yc, "lead", "", nil, nil)
	record(yc, "sale", "RUB", f(1000), nil)
	record(yc, "rebuy", "RUB", f(500), nil) // no repeat purchase goal
	record(usdClick, "sale", "USD", f(10), nil)
	record(plain, "sale", "RUB", f(10), nil)
	record(gclick, "sale", "RUB", f(10), nil)
	record(old, "sale", "RUB", f(10), nil)
	record(bad, "sale", "RUB", f(10), nil)

	if err := svc.Enqueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.Send(ctx); err != nil {
		t.Fatal(err)
	}
	sent := m.sentByYclid()
	if rows := sent["111"]; len(rows) != 2 {
		t.Fatalf("yclid 111 went as %+v, want the lead and the sale", rows)
	}
	for _, r := range sent["111"] {
		switch r.Goal {
		case "lead":
			if r.Price != nil {
				t.Fatalf("the lead went with a price: %+v", r)
			}
		case "purchase":
			if r.Price == nil || *r.Price != 1000 || r.Currency != "RUB" || r.At.Unix() != now.Unix() {
				t.Fatalf("the sale went as %+v", r)
			}
		default:
			t.Fatalf("goal %q", r.Goal)
		}
	}
	var rubPerUSD float64
	if err := pool.QueryRow(ctx, `SELECT 1 / rate_to_usd::float8 FROM fx_rates WHERE currency = 'RUB' AND rate_date <= $1::date
		ORDER BY rate_date DESC LIMIT 1`, now.Format(time.DateOnly)).Scan(&rubPerUSD); err != nil {
		t.Fatalf("no RUB rate in fx_rates: %v", err)
	}
	if rows := sent["222"]; len(rows) != 1 || rows[0].Price == nil || math.Abs(*rows[0].Price-10*rubPerUSD) > 0.01 || rows[0].Currency != "RUB" {
		t.Fatalf("the USD sale went as %+v, want %.2f RUB", rows, 10*rubPerUSD)
	}
	if len(sent["333"]) != 0 || len(sent["Cj0"]) != 0 {
		t.Fatalf("an old visit or a Google click went to Metrica: %+v", sent)
	}
	// The refused file: halved until the bad row is alone; it fails, the
	// rest went.
	var badStatus, badErr string
	if err := pool.QueryRow(ctx, `SELECT u.status, u.last_error FROM offline_conversion_uploads u JOIN postbacks p ON p.id = u.postback_id
		WHERE p.click_id = $1`, bad).Scan(&badStatus, &badErr); err != nil || badStatus != StatusFailed || !strings.Contains(badErr, "bad yclid") {
		t.Fatalf("the bad row: %s %q %v", badStatus, badErr, err)
	}

	st, err = svc.GetMetrica(ctx, orgID, direct)
	if err != nil {
		t.Fatal(err)
	}
	if st.Counts[StatusSent] != 3 || st.Counts[StatusFailed] != 1 || st.Processing != 3 ||
		st.Skipped[SkipNoAction] != 1 || st.Skipped[SkipNoYandexClick] != 2 || st.Skipped[SkipVisitTooOld] != 1 {
		t.Fatalf("status = %+v", st)
	}

	// A refund after the upload: Metrica has no restating, nothing is sent
	// again.
	calls := m.calls
	record(yc, "refund", "RUB", nil, f(250))
	if err := svc.Enqueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.Send(ctx); err != nil {
		t.Fatal(err)
	}
	if m.calls != calls {
		t.Fatalf("a refund sent %d more files", m.calls-calls)
	}

	// Check: not before the recheck interval; then the file with the USD
	// sale matched nothing, the others were processed.
	var usdFile string
	if err := pool.QueryRow(ctx, `SELECT request_id FROM offline_conversion_uploads WHERE organization_id = $1 AND ad_click_id = '222'`, orgID).Scan(&usdFile); err != nil {
		t.Fatal(err)
	}
	var inUSDFile int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM offline_conversion_uploads WHERE organization_id = $1 AND request_id = $2`, orgID, usdFile).Scan(&inUSDFile); err != nil {
		t.Fatal(err)
	}
	usdID, _ := strconv.ParseInt(usdFile, 10, 64)
	m.status[usdID] = yandexmetrica.StatusLinkageFailure
	if err := svc.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ := svc.GetMetrica(ctx, orgID, direct); st.Processing != 3 {
		t.Fatalf("checked before the recheck interval: %+v", st)
	}
	exec(t, pool, `UPDATE offline_conversion_uploads SET updated_at = now() - interval '1 hour' WHERE organization_id = $1`, orgID)
	if err := svc.Check(ctx); err != nil {
		t.Fatal(err)
	}
	st, err = svc.GetMetrica(ctx, orgID, direct)
	if err != nil {
		t.Fatal(err)
	}
	wantSent, wantFailed := 3-inUSDFile, 1+inUSDFile
	if st.Processing != 0 || st.Counts[StatusSent] != wantSent || st.Counts[StatusFailed] != wantFailed || !strings.Contains(st.LastError, "LINKAGE_FAILURE") {
		t.Fatalf("after the check: %+v", st)
	}

	// An outage leaves a new conversion pending, retried.
	m.down = true
	record(late, "lead", "", nil, nil)
	if err := svc.Enqueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.Send(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT u.status, u.attempts FROM offline_conversion_uploads u JOIN postbacks p ON p.id = u.postback_id
		WHERE p.click_id = $1`, late).Scan(&status, &attempts); err != nil || status != StatusPending || attempts != 1 {
		t.Fatalf("after an outage: %s %d %v", status, attempts, err)
	}

	// Disconnecting keeps the rows, unsent.
	if err := svc.DeleteMetrica(ctx, orgID, direct); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteMetrica(ctx, orgID, direct); err == nil {
		t.Fatal("deleted twice")
	}
}

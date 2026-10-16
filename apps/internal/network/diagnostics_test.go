package network_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/network"
)

type redepMapper struct{}

func (redepMapper) MapStatus(_ context.Context, _, _, raw string) (event.Type, error) {
	return event.CpaRedep, nil
}

type usdFX struct{}

func (usdFX) ToUSD(_ context.Context, _ string, amount float64, _ time.Time) (float64, bool, error) {
	return amount, true, nil
}

type unknownClick struct{}

func (unknownClick) AttributeConversion(context.Context, attribution.Conversion) (attribution.Attribution, error) {
	return attribution.Attribution{Outcome: attribution.OutcomeUnknownClick}, nil
}

type noAttempts struct{}

func (noAttempts) LogAttempt(context.Context, conversion.AttemptRecord) {}

// Redeposits without txn_id: the second of a click is a duplicate, and the
// network page says how many were lost (audit 2026-10-15, DATA-2). The rows
// come from the real conversion engine, so the count reads what it writes.
func TestDiagnosticsCountRedepositsWithoutTxnID(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := network.NewService(network.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Redep Network", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatal(err)
	}
	networkID := created.Network.ID

	d, err := svc.Diagnostics(ctx, orgID, networkID)
	if err != nil || d.RedepWithoutTxn != (network.RedepWithoutTxn{}) || d.WindowDays != 30 {
		t.Fatalf("Diagnostics of a new network = %+v, %v; want nothing counted", d, err)
	}

	engine := conversion.NewService(redepMapper{}, conversion.NewPostgresStore(pool), usdFX{}, unknownClick{}, noAttempts{})
	clickID := idgen.New()
	amount := 50.0
	record := func(txn string) conversion.Result {
		t.Helper()
		res, err := engine.Record(ctx, conversion.Postback{
			OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, RawStatus: "redep",
			Revenue: &amount, Currency: "USD", NetworkTxnID: txn, OccurredAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("Record(txn %q): %v", txn, err)
		}
		return res
	}
	if res := record(""); res.Kind != conversion.ResultSuccess {
		t.Fatalf("first redeposit = %s, want success", res.Kind)
	}
	dup := record("")
	if dup.Kind != conversion.ResultDuplicate || !strings.Contains(dup.Message, "txn_id") {
		t.Fatalf("second redeposit without txn_id = %s %q; want a duplicate that names txn_id", dup.Kind, dup.Message)
	}
	// With a transaction id it is its own deposit, and not counted here.
	if res := record("tx-1"); res.Kind != conversion.ResultSuccess {
		t.Fatalf("redeposit with txn_id = %s, want success", res.Kind)
	}

	d, err = svc.Diagnostics(ctx, orgID, networkID)
	if err != nil {
		t.Fatal(err)
	}
	if d.RedepWithoutTxn.Received != 2 || d.RedepWithoutTxn.Duplicates != 1 || d.RedepWithoutTxn.LastAt == nil {
		t.Fatalf("RedepWithoutTxn = %+v; want 2 received, 1 duplicate, a last time", d.RedepWithoutTxn)
	}

	// Another organization does not find the network (CLAUDE.md #5).
	otherOrg := seedOrg(t, ctx, pool)
	var apiErr *apierror.Error
	if _, err := svc.Diagnostics(ctx, otherOrg, networkID); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("Diagnostics from another organization: %v, want 404", err)
	}
}

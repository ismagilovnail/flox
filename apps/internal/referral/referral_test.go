package referral_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/referral"
)

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedOrgAndUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (orgID, userID string) {
	t.Helper()
	orgID = idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, "Test Org "+orgID); err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	userID = idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'Test User')`, userID, userID+"@example.com"); err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return orgID, userID
}

func TestEnsureCodeIsStableAndUnique(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA, _ := seedOrgAndUser(t, ctx, pool)
	orgB, _ := seedOrgAndUser(t, ctx, pool)
	svc := referral.NewService(referral.NewRepository(pool))

	dashA1, err := svc.Dashboard(ctx, orgA)
	if err != nil {
		t.Fatalf("Dashboard (org A, first): %v", err)
	}
	dashA2, err := svc.Dashboard(ctx, orgA)
	if err != nil {
		t.Fatalf("Dashboard (org A, second): %v", err)
	}
	if dashA1.Code != dashA2.Code {
		t.Fatalf("code changed between calls: %q vs %q", dashA1.Code, dashA2.Code)
	}

	dashB, err := svc.Dashboard(ctx, orgB)
	if err != nil {
		t.Fatalf("Dashboard (org B): %v", err)
	}
	if dashB.Code == dashA1.Code {
		t.Fatal("two different orgs got the same referral code")
	}
}

func TestRecordSignupAttributesRealSignup(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	referrerOrg, _ := seedOrgAndUser(t, ctx, pool)
	referredOrg, _ := seedOrgAndUser(t, ctx, pool)
	svc := referral.NewService(referral.NewRepository(pool))

	dash, err := svc.Dashboard(ctx, referrerOrg)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}

	if err := svc.RecordSignup(ctx, dash.Code, referredOrg, "New User", "new@example.com"); err != nil {
		t.Fatalf("RecordSignup: %v", err)
	}

	dash2, err := svc.Dashboard(ctx, referrerOrg)
	if err != nil {
		t.Fatalf("Dashboard after signup: %v", err)
	}
	if len(dash2.Signups) != 1 || dash2.Signups[0].Email != "new@example.com" || dash2.Signups[0].Status != referral.StatusTrial {
		t.Fatalf("Signups = %+v, want exactly one trial signup for new@example.com", dash2.Signups)
	}
}

func TestRecordSignupSilentlyIgnoresUnknownCode(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	referredOrg, _ := seedOrgAndUser(t, ctx, pool)
	svc := referral.NewService(referral.NewRepository(pool))

	if err := svc.RecordSignup(ctx, "not-a-real-code", referredOrg, "New User", "new@example.com"); err != nil {
		t.Fatalf("RecordSignup with unknown code returned an error, want silent no-op: %v", err)
	}
}

func TestAddTransactionAndBalance(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := referral.NewService(referral.NewRepository(pool))

	if _, err := svc.AddTransaction(ctx, orgID, userID, referral.AddTransactionInput{
		Type: referral.TxnAccrual, Amount: 50, Description: "Referral bonus",
	}); err != nil {
		t.Fatalf("AddTransaction accrual: %v", err)
	}
	if _, err := svc.AddTransaction(ctx, orgID, userID, referral.AddTransactionInput{
		Type: referral.TxnAdjustment, Amount: -10, Description: "Correction",
	}); err != nil {
		t.Fatalf("AddTransaction adjustment: %v", err)
	}

	dash, err := svc.Dashboard(ctx, orgID)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dash.Balance.TotalEarned != 40 {
		t.Fatalf("TotalEarned = %v, want 40 (50 - 10)", dash.Balance.TotalEarned)
	}
	if dash.Balance.AvailableBalance != 40 {
		t.Fatalf("AvailableBalance = %v, want 40", dash.Balance.AvailableBalance)
	}
}

func TestAddTransactionRejectsInvalidInput(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := referral.NewService(referral.NewRepository(pool))

	cases := []referral.AddTransactionInput{
		{Type: referral.TxnPayoutPaid, Amount: 10, Description: "not directly insertable"},
		{Type: referral.TxnAccrual, Amount: 0, Description: "zero amount"},
		{Type: referral.TxnAccrual, Amount: 10, Description: ""},
	}
	for _, in := range cases {
		if _, err := svc.AddTransaction(ctx, orgID, userID, in); err == nil {
			t.Errorf("%+v: accepted, want rejected", in)
		}
	}
}

func TestPayoutStateMachine(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := referral.NewService(referral.NewRepository(pool))

	if _, err := svc.AddTransaction(ctx, orgID, userID, referral.AddTransactionInput{
		Type: referral.TxnAccrual, Amount: 100, Description: "Bonus",
	}); err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	// Cannot request more than the available balance.
	if _, err := svc.RequestPayout(ctx, orgID, userID, 150); err == nil {
		t.Fatal("requested more than available balance, want rejected")
	}

	payout, err := svc.RequestPayout(ctx, orgID, userID, 60)
	if err != nil {
		t.Fatalf("RequestPayout: %v", err)
	}
	if payout.Status != referral.PayoutPending {
		t.Fatalf("Status = %q, want pending", payout.Status)
	}

	// A pending payout counts against available balance for a second request.
	dash, err := svc.Dashboard(ctx, orgID)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dash.Balance.AvailableBalance != 40 {
		t.Fatalf("AvailableBalance after one pending payout = %v, want 40 (100 - 60)", dash.Balance.AvailableBalance)
	}

	// Cannot mark-paid before approval.
	if _, err := svc.MarkPayoutPaid(ctx, orgID, payout.ID, userID); err == nil {
		t.Fatal("marked a pending payout as paid, want rejected")
	}

	approved, err := svc.ApprovePayout(ctx, orgID, payout.ID, userID)
	if err != nil {
		t.Fatalf("ApprovePayout: %v", err)
	}
	if approved.Status != referral.PayoutApproved || approved.ResolvedAt == nil {
		t.Fatalf("approved = %+v, want approved with ResolvedAt set", approved)
	}

	// Cannot approve twice.
	if _, err := svc.ApprovePayout(ctx, orgID, payout.ID, userID); err == nil {
		t.Fatal("approved an already-approved payout, want rejected")
	}

	paid, err := svc.MarkPayoutPaid(ctx, orgID, payout.ID, userID)
	if err != nil {
		t.Fatalf("MarkPayoutPaid: %v", err)
	}
	if paid.Status != referral.PayoutPaid {
		t.Fatalf("Status = %q, want paid", paid.Status)
	}

	dashFinal, err := svc.Dashboard(ctx, orgID)
	if err != nil {
		t.Fatalf("Dashboard (final): %v", err)
	}
	if dashFinal.Balance.TotalPaid != 60 {
		t.Fatalf("TotalPaid = %v, want 60", dashFinal.Balance.TotalPaid)
	}
	if dashFinal.Balance.PendingPayout != 0 {
		t.Fatalf("PendingPayout = %v, want 0 (payout is now paid, not pending)", dashFinal.Balance.PendingPayout)
	}
	if dashFinal.Balance.AvailableBalance != 40 {
		t.Fatalf("AvailableBalance = %v, want 40 (100 earned - 60 paid)", dashFinal.Balance.AvailableBalance)
	}

	found := false
	for _, txn := range dashFinal.Transactions {
		if txn.Type == referral.TxnPayoutPaid {
			found = true
			if txn.Amount != -60 {
				t.Fatalf("payout_paid transaction amount = %v, want -60", txn.Amount)
			}
			if txn.RelatedPayoutID == nil || *txn.RelatedPayoutID != payout.ID {
				t.Fatalf("payout_paid transaction RelatedPayoutID = %v, want %q", txn.RelatedPayoutID, payout.ID)
			}
		}
	}
	if !found {
		t.Fatal("no payout_paid transaction found after MarkPayoutPaid")
	}
}

func TestRejectPayoutRequiresNote(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := referral.NewService(referral.NewRepository(pool))

	if _, err := svc.AddTransaction(ctx, orgID, userID, referral.AddTransactionInput{
		Type: referral.TxnAccrual, Amount: 100, Description: "Bonus",
	}); err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}
	payout, err := svc.RequestPayout(ctx, orgID, userID, 50)
	if err != nil {
		t.Fatalf("RequestPayout: %v", err)
	}

	if _, err := svc.RejectPayout(ctx, orgID, payout.ID, userID, ""); err == nil {
		t.Fatal("rejected without a note, want rejected-request rejected")
	}

	rejected, err := svc.RejectPayout(ctx, orgID, payout.ID, userID, "Suspicious activity")
	if err != nil {
		t.Fatalf("RejectPayout: %v", err)
	}
	if rejected.Status != referral.PayoutRejected || rejected.Note != "Suspicious activity" {
		t.Fatalf("rejected = %+v, want status=rejected note=%q", rejected, "Suspicious activity")
	}

	// A rejected payout must not still count against available balance.
	dash, err := svc.Dashboard(ctx, orgID)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dash.Balance.AvailableBalance != 100 {
		t.Fatalf("AvailableBalance after rejection = %v, want 100 (full amount freed)", dash.Balance.AvailableBalance)
	}
}

func TestCrossOrgIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA, userA := seedOrgAndUser(t, ctx, pool)
	orgB, userB := seedOrgAndUser(t, ctx, pool)
	svc := referral.NewService(referral.NewRepository(pool))

	if _, err := svc.AddTransaction(ctx, orgA, userA, referral.AddTransactionInput{
		Type: referral.TxnAccrual, Amount: 100, Description: "Org A bonus",
	}); err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}
	payout, err := svc.RequestPayout(ctx, orgA, userA, 50)
	if err != nil {
		t.Fatalf("RequestPayout: %v", err)
	}

	if _, err := svc.ApprovePayout(ctx, orgB, payout.ID, userB); err == nil {
		t.Fatal("org B approved org A's payout, want rejected")
	}

	dashB, err := svc.Dashboard(ctx, orgB)
	if err != nil {
		t.Fatalf("Dashboard (org B): %v", err)
	}
	if len(dashB.Payouts) != 0 || len(dashB.Transactions) != 0 || dashB.Balance.AvailableBalance != 0 {
		t.Fatalf("org B dashboard leaked org A data: %+v", dashB)
	}
}

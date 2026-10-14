package ltv_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/fx"
	"github.com/ismagilovnail/flox/apps/internal/ltv"
)

type fakeRepo struct {
	ftdHistories []chstore.ClickHistory
	regHistories []chstore.ClickHistory
	ftdFilter    chstore.LTVFilter
	regFilter    chstore.LTVFilter
}

func (r *fakeRepo) ClicksByFTDAnchor(_ context.Context, _ string, filter chstore.LTVFilter) ([]chstore.ClickHistory, error) {
	r.ftdFilter = filter
	return r.ftdHistories, nil
}

func (r *fakeRepo) ClicksByRegAnchor(_ context.Context, _ string, filter chstore.LTVFilter) ([]chstore.ClickHistory, error) {
	r.regFilter = filter
	return r.regHistories, nil
}

func TestServiceFTDCohortsPassesThroughToComputation(t *testing.T) {
	repo := &fakeRepo{ftdHistories: []chstore.ClickHistory{
		{ClickID: "c1", Deposits: []chstore.Deposit{{EventAt: at(0), Type: event.CpaAccept, USDValue: 5, HasUSDValue: true}}},
	}}
	svc := ltv.NewService(repo)

	cohorts, err := svc.FTDCohorts(context.Background(), "org-1",
		chstore.LTVFilter{From: at(-1), To: at(1)}, ltv.PeriodDay, at(200))
	if err != nil {
		t.Fatalf("FTDCohorts: %v", err)
	}
	if len(cohorts) != 1 || cohorts[0].LTVTotalUSD != 5 {
		t.Fatalf("cohorts = %+v, want one cohort with LTVTotalUSD=5", cohorts)
	}
}

func TestServiceRejectsMissingOrg(t *testing.T) {
	svc := ltv.NewService(&fakeRepo{})
	_, err := svc.FTDCohorts(context.Background(), "", chstore.LTVFilter{From: at(0), To: at(1)}, ltv.PeriodDay, at(200))
	if err == nil {
		t.Fatal("expected an error for a missing organization id")
	}
}

func TestServiceRejectsInvalidPeriod(t *testing.T) {
	svc := ltv.NewService(&fakeRepo{})
	_, err := svc.FTDCohorts(context.Background(), "org-1", chstore.LTVFilter{From: at(0), To: at(1)}, ltv.CohortPeriod("year"), at(200))
	if err == nil {
		t.Fatal("expected an error for an invalid period")
	}
}

func TestServiceRejectsInvertedRange(t *testing.T) {
	svc := ltv.NewService(&fakeRepo{})
	_, err := svc.RegCohorts(context.Background(), "org-1", chstore.LTVFilter{From: at(5), To: at(0)}, ltv.PeriodDay, at(200))
	if err == nil {
		t.Fatal("expected an error when To is before From")
	}
}

func TestServiceRejectsRangeTooWide(t *testing.T) {
	repo := &fakeRepo{}
	svc := ltv.NewService(repo)
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.FTDCohorts(context.Background(), "org-1", chstore.LTVFilter{From: from, To: from.AddDate(2, 0, 0)}, ltv.PeriodDay, at(200))
	if err == nil {
		t.Fatal("expected an error for a multi-year anchor range")
	}
}

type fakeMoney struct{ conv *fx.Conversion }

func (m fakeMoney) ReportingConversion(context.Context, string, time.Time, time.Time) (*fx.Conversion, error) {
	return m.conv, nil
}

func TestServiceConvertsEachDepositAtItsDaysRate(t *testing.T) {
	day0 := at(0).Truncate(24 * time.Hour)
	repo := &fakeRepo{ftdHistories: []chstore.ClickHistory{{ClickID: "c1", Deposits: []chstore.Deposit{
		{EventAt: at(0), Type: event.CpaAccept, USDValue: 10, HasUSDValue: true},
		{EventAt: at(0).AddDate(0, 0, 1), Type: event.CpaRedep, USDValue: 10, HasUSDValue: true},
	}}}}
	// 1 unit = 2 USD on day 0, = 4 USD from day 1: factors 0.5 and 0.25.
	conv := fx.NewConversion("EUR", []fx.Rate{{Date: day0, RateToUSD: 2}, {Date: day0.AddDate(0, 0, 1), RateToUSD: 4}}, day0, day0.AddDate(0, 0, 3))
	svc := ltv.NewService(repo).WithMoney(fakeMoney{conv})

	cohorts, err := svc.FTDCohorts(context.Background(), "org-1", chstore.LTVFilter{From: at(-1), To: at(1)}, ltv.PeriodDay, at(200))
	if err != nil {
		t.Fatalf("FTDCohorts: %v", err)
	}
	if len(cohorts) != 1 || cohorts[0].TotalDepositRevenueUSD != 7.5 { // 10*0.5 + 10*0.25
		t.Fatalf("cohorts = %+v, want deposit revenue 7.5", cohorts)
	}
	if cur, err := svc.Currency(context.Background(), "org-1"); err != nil || cur != "EUR" {
		t.Fatalf("Currency = %q, %v", cur, err)
	}
}

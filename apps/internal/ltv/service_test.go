package ltv_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
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

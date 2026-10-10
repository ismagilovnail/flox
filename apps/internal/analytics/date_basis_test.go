package analytics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// Tech audit AN5: the date basis reaches the conversion query, an unknown
// one is a validation error, and conversions kept on their own date are
// counted on the rows and the dashboard totals.
func TestDateBasisReachesTheConversionQuery(t *testing.T) {
	day := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	repo := &reportFakeRepo{conv: []chstore.ConversionAgg{{Conversions: 3, RevenueUSD: 112, DatedByConversion: 1}}}
	svc := analytics.NewService(repo, nil)

	for _, basis := range []chstore.DateBasis{"", chstore.DateByConversion, chstore.DateByClick} {
		rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{From: day, To: day, DateBasis: basis})
		if err != nil {
			t.Fatalf("%q: %v", basis, err)
		}
		if repo.lastConvQuery == nil || repo.lastConvQuery.DateBasis != basis {
			t.Fatalf("%q: conversion query basis = %+v", basis, repo.lastConvQuery)
		}
		if len(rows) != 1 || rows[0].DatedByConversion != 1 {
			t.Fatalf("%q: rows = %+v, want datedByConversion 1 carried onto the row", basis, rows)
		}
	}

	dash, err := svc.Dashboard(context.Background(), "org-1", day, day, "", chstore.DateByClick)
	if err != nil {
		t.Fatal(err)
	}
	if dash.DatedByConversion != 1 || repo.lastConvQuery.DateBasis != chstore.DateByClick {
		t.Fatalf("dashboard: datedByConversion = %d, basis = %q", dash.DatedByConversion, repo.lastConvQuery.DateBasis)
	}

	calls := []func() error{
		func() error {
			_, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{From: day, To: day, DateBasis: "postback"})
			return err
		},
		func() error {
			_, err := svc.Timeseries(context.Background(), "org-1", analytics.TimeseriesInput{From: day, To: day, DateBasis: "postback"})
			return err
		},
		func() error {
			_, err := svc.Breakdown(context.Background(), "org-1", analytics.BreakdownInput{Dim: chstore.DimCampaign, From: day, To: day, DateBasis: "postback"})
			return err
		},
		func() error {
			_, err := svc.Dashboard(context.Background(), "org-1", day, day, "", "postback")
			return err
		},
		func() error {
			_, err := svc.CampaignDailyRevenue(context.Background(), "org-1", "camp-1", day, day, "postback")
			return err
		},
	}
	for i, call := range calls {
		var apiErr *apierror.Error
		if err := call(); !errors.As(err, &apiErr) || apiErr.Fields["dateBasis"] == "" {
			t.Fatalf("call %d with an unknown basis: %v, want a dateBasis validation error", i, err)
		}
	}
}

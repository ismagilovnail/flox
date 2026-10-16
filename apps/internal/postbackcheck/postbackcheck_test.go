package postbackcheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

type networks map[string]conversion.Network

func (n networks) ByID(_ context.Context, id string) (conversion.Network, error) {
	if nw, ok := n[id]; ok {
		return nw, nil
	}
	return conversion.Network{}, conversion.ErrNetworkNotFound
}

// engine records the postback it was asked about and answers success.
type engine struct{ got *conversion.Postback }

func (e *engine) Check(_ context.Context, p conversion.Postback) (conversion.CheckResult, error) {
	e.got = &p
	usd := 20.0
	return conversion.CheckResult{
		Result: conversion.Result{Kind: conversion.ResultSuccess, Status: event.CpaAccept, Message: "matched",
			Attribution: attribution.Attribution{Outcome: attribution.OutcomeAttributed, Method: attribution.MethodClickID,
				Click: attribution.Click{ClickID: p.ClickID, CampaignID: "camp"}}},
		Revenue: p.Revenue, Currency: "EUR", CurrencyAssumed: true, USDValue: &usd,
	}, nil
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func setup() (*Service, *engine, string, string, string) {
	orgA, orgB := idgen.New(), idgen.New()
	netA, netB := idgen.New(), idgen.New()
	eng := &engine{}
	svc := NewService(networks{
		netA: {ID: netA, OrganizationID: orgA, PostbackSecretHash: hash("s3cret"), Currency: "EUR"},
		netB: {ID: netB, OrganizationID: orgB, PostbackSecretHash: hash("s3cret")},
	}, eng, "https://t.example.com")
	svc.now = func() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC) }
	return svc, eng, orgA, netA, netB
}

func TestCheckRunsTheURLThroughTheEngine(t *testing.T) {
	svc, eng, orgA, netA, _ := setup()
	out, err := svc.Check(context.Background(), orgA,
		"https://t.example.com/postback/"+netA+"?secret=s3cret&click_id=c1&status=ftd&revenue=40&txn_id=T1")
	if err != nil {
		t.Fatal(err)
	}
	if out.HTTPStatus != http.StatusOK || out.Result != "success" || out.Status != "CPA_ACCEPT" {
		t.Fatalf("outcome = %+v", out)
	}
	if out.Attribution == nil || out.Attribution.CampaignID != "camp" || out.USDValue == nil || *out.USDValue != 20 || !out.CurrencyAssumed {
		t.Fatalf("details = %+v", out)
	}
	if len(out.Notes) != 0 {
		t.Fatalf("notes = %v, want none for the tracker's host", out.Notes)
	}
	p := eng.got
	if p.OrganizationID != orgA || p.NetworkID != netA || p.ClickID != "c1" || p.RawStatus != "ftd" ||
		p.NetworkTxnID != "T1" || p.NetworkCurrency != "EUR" || p.Revenue == nil || *p.Revenue != 40 {
		t.Fatalf("postback = %+v", p)
	}
}

func TestAnotherOrganizationsNetworkIsNotFound(t *testing.T) {
	svc, _, orgA, _, netB := setup()
	for name, id := range map[string]string{"another organization's": netB, "unknown": idgen.New()} {
		_, err := svc.Check(context.Background(), orgA, "https://t.example.com/postback/"+id+"?secret=s3cret&click_id=c&status=ftd")
		var ae *apierror.Error
		if !errors.As(err, &ae) || ae.Status != http.StatusNotFound || ae.Message != "network not found" {
			t.Fatalf("%s network: err = %v, want 404 network not found", name, err)
		}
	}
}

func TestTrackerRefusalsComeFirst(t *testing.T) {
	svc, eng, orgA, netA, _ := setup()
	base := "https://t.example.com/postback/" + netA
	cases := []struct {
		query  string
		status int
		msg    string
	}{
		{"?click_id=c&status=ftd", http.StatusUnauthorized, "invalid or missing secret"},
		{"?secret=wrong&click_id=c&status=ftd", http.StatusUnauthorized, "invalid or missing secret"},
		{"?secret=s3cret&click_id=c&status=ftd&revenue=-5", http.StatusBadRequest, "invalid revenue"},
		{"?secret=s3cret&click_id=c&status=ftd&event_time=2099-01-01T00:00:00Z", http.StatusBadRequest, "in the future"},
	}
	for _, tc := range cases {
		out, err := svc.Check(context.Background(), orgA, base+tc.query)
		if err != nil {
			t.Fatalf("%s: %v", tc.query, err)
		}
		if out.HTTPStatus != tc.status || out.Result != "error" || !strings.Contains(out.Message, tc.msg) {
			t.Fatalf("%s: %+v, want %d %q", tc.query, out, tc.status, tc.msg)
		}
	}
	if eng.got != nil {
		t.Fatal("the engine ran for a postback the tracker refuses before it")
	}
}

func TestURLProblems(t *testing.T) {
	svc, _, orgA, netA, _ := setup()
	for _, raw := range []string{"", "   ", "https://t.example.com/?click_id=c", "https://t.example.com/postback/not-an-id?x=1"} {
		_, err := svc.Check(context.Background(), orgA, raw)
		var ae *apierror.Error
		if !errors.As(err, &ae) || ae.Status != http.StatusUnprocessableEntity {
			t.Fatalf("%q: err = %v, want a validation error", raw, err)
		}
	}

	out, err := svc.Check(context.Background(), orgA, "https://api.example.com/postback/"+netA+"/?secret=s3cret&click_id=c&status=ftd")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Notes) != 1 || !strings.Contains(out.Notes[0], "api.example.com") {
		t.Fatalf("notes = %v, want the wrong host named", out.Notes)
	}
}

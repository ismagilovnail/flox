package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

type allSales struct{}

func (allSales) MapStatus(context.Context, string, string, string) (event.Type, error) {
	return event.CpaAccept, nil
}

// captureStore keeps what the service recorded.
type captureStore struct{ entries []conversion.Entry }

func (s *captureStore) LastStatus(context.Context, string, string) (event.Type, bool, error) {
	return "", false, nil
}

func (s *captureStore) Record(_ context.Context, e conversion.Entry) (conversion.Recorded, error) {
	s.entries = append(s.entries, e)
	return conversion.Recorded{ID: idgen.New(), Kind: e.Kind, Message: e.Message}, nil
}

type usdOnly struct{}

func (usdOnly) ToUSD(_ context.Context, _ string, amount float64, _ time.Time) (float64, bool, error) {
	return amount, true, nil
}

type noClicks struct{}

func (noClicks) AttributeConversion(context.Context, attribution.Conversion) (attribution.Attribution, error) {
	return attribution.Attribution{Outcome: attribution.OutcomeUnknownClick}, nil
}

type noAttempts struct{}

func (noAttempts) LogAttempt(context.Context, conversion.AttemptRecord) {}

// event_time (or ts) dates the conversion; a bad one is a 400 the network
// must not retry (tech audit T3).
func TestPostbackEventTime(t *testing.T) {
	networkID := idgen.New()
	store := &captureStore{}
	h := &PostbackHandler{
		networks: staticNetworks{conversion.Network{ID: networkID, OrganizationID: idgen.New(), PostbackSecretHash: hashFor("s3cret")}},
		service:  conversion.NewService(allSales{}, store, usdOnly{}, noClicks{}, noAttempts{}),
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	r := chi.NewRouter()
	h.Register(r)
	send := func(extra string) (*httptest.ResponseRecorder, postbackResponse) {
		req := httptest.NewRequest(http.MethodGet, "/postback/"+networkID+"?secret=s3cret&click_id="+idgen.New()+"&status=sale"+extra, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var body postbackResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec, body
	}

	yesterday := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	for _, param := range []string{"event_time", "ts"} {
		before := len(store.entries)
		if rec, body := send("&" + param + "=" + yesterday.Format(time.RFC3339)); rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d %+v", param, rec.Code, body)
		}
		if len(store.entries) != before+1 || !store.entries[before].Event.EventAt.Equal(yesterday) {
			t.Fatalf("%s: recorded event at %v, want %v", param, store.entries[len(store.entries)-1].Event.EventAt, yesterday)
		}
	}

	started := time.Now()
	if rec, _ := send(""); rec.Code != http.StatusOK || store.entries[len(store.entries)-1].Event.EventAt.Before(started.Add(-time.Second)) {
		t.Fatal("without event_time the conversion must be dated now")
	}

	for _, bad := range []string{"tomorrow", time.Now().Add(time.Hour).Format(time.RFC3339), "2020-01-01T00:00:00Z"} {
		before := len(store.entries)
		rec, body := send("&event_time=" + bad)
		if rec.Code != http.StatusBadRequest || !strings.HasPrefix(body.Message, "invalid event_time") || len(store.entries) != before {
			t.Errorf("event_time=%s: %d %+v, want a 400 before anything is recorded", bad, rec.Code, body)
		}
	}
}

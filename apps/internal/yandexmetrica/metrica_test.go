package yandexmetrica_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/yandexmetrica"
)

func ptr(v float64) *float64 { return &v }

// Audit UNI-2 part 3: the documented upload — OAuth header, multipart
// field "file", a CSV with the header row, Unix seconds, a dot decimal,
// no price or currency for a conversion without revenue.
func TestUploadSendsTheDocumentedFile(t *testing.T) {
	var file string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/management/v1/counter/12345/offline_conversions/upload" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "OAuth tok" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("no file part: %v", err)
		}
		b, _ := io.ReadAll(f)
		file = string(b)
		_, _ = w.Write([]byte(`{"uploading": {"id": 77, "status": "UPLOADED", "source_quantity": 2, "line_quantity": 2, "client_id_type": "YCLID"}}`))
	}))
	defer s.Close()

	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	got, err := (&yandexmetrica.Client{BaseURL: s.URL}).Upload(context.Background(), "tok", "12345", []yandexmetrica.Conversion{
		{Yclid: "111", Goal: "purchase", At: at, Price: ptr(1234.5), Currency: "RUB"},
		{Yclid: "222", Goal: "lead", At: at, Currency: "RUB"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 77 || got.Status != "UPLOADED" || got.LineQuantity != 2 {
		t.Errorf("uploading = %+v", got)
	}
	want := "Yclid,Target,DateTime,Price,Currency\n111,purchase,1792497600,1234.50,RUB\n222,lead,1792497600,,\n"
	if file != want {
		t.Errorf("file =\n%s\nwant\n%s", file, want)
	}
}

// A 400 (the file is invalid) is permanent; a refused token or a quota is
// not; the message is Metrica's errors.
func TestErrorsArePermanentOnlyFor400(t *testing.T) {
	for _, tc := range []struct {
		status    int
		permanent bool
		auth      bool
	}{
		{http.StatusBadRequest, true, false},
		{http.StatusUnauthorized, false, true},
		{http.StatusForbidden, false, true},
		{http.StatusTooManyRequests, false, false},
		{http.StatusServiceUnavailable, false, false},
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"errors": [{"error_type": "invalid_uploading", "message": "bad file"}], "code": 400, "message": "bad file"}`))
		}))
		_, err := (&yandexmetrica.Client{BaseURL: s.URL}).Upload(context.Background(), "tok", "1", []yandexmetrica.Conversion{{Yclid: "1", Goal: "g", At: time.Now()}})
		s.Close()
		if err == nil {
			t.Fatalf("%d: no error", tc.status)
		}
		if yandexmetrica.IsPermanent(err) != tc.permanent || yandexmetrica.IsAuth(err) != tc.auth {
			t.Errorf("%d: permanent %v auth %v", tc.status, yandexmetrica.IsPermanent(err), yandexmetrica.IsAuth(err))
		}
		if !strings.Contains(err.Error(), "invalid_uploading: bad file") {
			t.Errorf("%d: %v", tc.status, err)
		}
	}
}

func TestCheckAndCounter(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/management/v1/counter/9/offline_conversions/uploading/77":
			_, _ = w.Write([]byte(`{"uploading": {"id": 77, "status": "LINKAGE_FAILURE"}}`))
		case "/management/v1/counter/9":
			_, _ = w.Write([]byte(`{"counter": {"id": 9}}`))
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errors": [{"error_type": "access_denied", "message": "Access is denied"}]}`))
		}
	}))
	defer s.Close()
	c := &yandexmetrica.Client{BaseURL: s.URL}
	u, err := c.Check(context.Background(), "tok", "9", 77)
	if err != nil || u.Status != yandexmetrica.StatusLinkageFailure || !yandexmetrica.Final(u.Status) {
		t.Errorf("check = %+v, %v", u, err)
	}
	if err := c.Counter(context.Background(), "tok", "9"); err != nil {
		t.Errorf("counter 9: %v", err)
	}
	if err := c.Counter(context.Background(), "tok", "8"); !yandexmetrica.IsAuth(err) {
		t.Errorf("counter 8: %v, want access denied", err)
	}
}

package campaignscope_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/campaignscope"
)

func TestScope(t *testing.T) {
	ctx := context.Background()
	if s, err := campaignscope.From(ctx); err != nil || !s.All || !s.Allows("anything") || !s.Allows("") {
		t.Errorf("no resolver = %+v, %v; want everything", s, err)
	}
	if ids, err := campaignscope.IDs(ctx); err != nil || ids != nil {
		t.Errorf("IDs without a resolver = %v, %v; want nil (no filter)", ids, err)
	}

	calls := 0
	ctx = campaignscope.WithResolver(ctx, func(context.Context) (campaignscope.Scope, error) {
		calls++
		return campaignscope.Only([]string{"a", "b"}), nil
	})
	if calls != 0 {
		t.Fatal("resolved before anyone asked")
	}
	for i := 0; i < 3; i++ {
		s, err := campaignscope.From(ctx)
		if err != nil || s.All || !s.Allows("a") || s.Allows("c") || s.Allows("") {
			t.Fatalf("scope = %+v, %v", s, err)
		}
	}
	if calls != 1 {
		t.Errorf("resolved %d times, want once per request", calls)
	}
	if ids, _ := campaignscope.IDs(ctx); len(ids) != 2 {
		t.Errorf("IDs = %v", ids)
	}

	none := campaignscope.With(context.Background(), campaignscope.Only(nil))
	if ids, _ := campaignscope.IDs(none); ids == nil || len(ids) != 0 {
		t.Errorf("owning nothing = %#v, want an empty, non-nil set (matches nothing)", ids)
	}
	failing := campaignscope.WithResolver(context.Background(), func(context.Context) (campaignscope.Scope, error) {
		return campaignscope.Scope{}, errors.New("postgres down")
	})
	if _, err := campaignscope.From(failing); err == nil {
		t.Error("a failing resolver read as a scope")
	}
}

package telegram_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/telegram"
)

func (w *webhookFixture) invitePool(org string) *telegram.InvitePool {
	return &telegram.InvitePool{Repo: w.repo, API: telegram.NewHTTPBotAPI(w.apiURL), Key: key, Logger: w.logger, OrganizationID: org}
}

// fillPool runs the worker until the bot's pool is full.
func (w *webhookFixture) fillPool(t *testing.T) {
	t.Helper()
	pool := w.invitePool(w.org)
	for range 5 {
		if _, _, err := pool.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce: %v", err)
		}
	}
	if n := w.free(t); n != telegram.InvitePoolSize {
		t.Fatalf("free = %d, want %d", n, telegram.InvitePoolSize)
	}
}

// claim is GET /tgj/{botId}?click= on the tracker; it returns the status
// and the Location.
func (w *webhookFixture) claim(t *testing.T, botID, query string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/tgj/"+botID+query, nil)
	rec := httptest.NewRecorder()
	w.router.ServeHTTP(rec, req)
	return rec.Code, rec.Header().Get("Location")
}

func (w *webhookFixture) free(t *testing.T) int {
	t.Helper()
	bots, err := w.svc.List(context.Background(), w.org)
	if err != nil || len(bots) != 1 {
		t.Fatalf("List = %v, %v", bots, err)
	}
	return bots[0].Channels[0].FreeInviteLinks
}

func newInviteFixture(t *testing.T, clicks attribution.ClickResolver) *webhookFixture {
	w := newWebhookFixture(t, clicks)
	telegram.NewInviteRedirect(w.repo, w.logger).Register(w.router)
	return w
}

// The worker fills the pool in steps; each click gets a link of its own,
// the same one when it asks again; without a pool or a click id the
// visitor still reaches the channel by its shared link.
func TestInviteRedirect(t *testing.T) {
	w := newInviteFixture(t, attribution.NewMemoryResolver())
	ctx := context.Background()

	clickA, clickB := idgen.New(), idgen.New()
	if code, loc := w.claim(t, w.ch.ID, "?click="+clickA); code != http.StatusFound || loc != "https://t.me/joinchan" {
		t.Fatalf("empty pool = %d %q, want the shared link", code, loc)
	}

	created, _, err := w.invitePool(w.org).RunOnce(ctx)
	if err != nil || created != 20 {
		t.Fatalf("first run created %d, %v; want 20 (one run's share)", created, err)
	}
	w.fillPool(t) // the remaining 30, then nothing more
	if n := w.free(t); n != telegram.InvitePoolSize {
		t.Fatalf("free = %d, want %d", n, telegram.InvitePoolSize)
	}

	bots, _ := w.svc.List(ctx, w.org)
	if bots[0].Channels[0].ClickInviteLink != "https://t.example/tgj/"+w.ch.ID+"?click={click_id}" {
		t.Fatalf("clickInviteLink = %q", bots[0].Channels[0].ClickInviteLink)
	}

	_, linkA := w.claim(t, w.ch.ID, "?click="+clickA)
	_, againA := w.claim(t, w.ch.ID, "?click="+clickA)
	_, linkB := w.claim(t, w.ch.ID, "?click="+clickB)
	if linkA == "https://t.me/joinchan" || linkA == "" || againA != linkA || linkB == linkA || linkB == "https://t.me/joinchan" {
		t.Fatalf("A = %q, A again = %q, B = %q; want A's own link twice and another for B", linkA, againA, linkB)
	}
	if code, loc := w.claim(t, w.ch.ID, ""); code != http.StatusFound || loc != "https://t.me/joinchan" {
		t.Fatalf("no click = %d %q, want the shared link", code, loc)
	}
	if code, loc := w.claim(t, w.ch.ID, "?click=%7Bclick_id%7D"); code != http.StatusFound || loc != "https://t.me/joinchan" {
		t.Fatalf("unresolved macro = %d %q, want the shared link", code, loc)
	}
	if n := w.free(t); n != telegram.InvitePoolSize-2 {
		t.Fatalf("free = %d, want %d (two clicks claimed)", n, telegram.InvitePoolSize-2)
	}
	if code, _ := w.claim(t, idgen.New(), "?click="+clickA); code != http.StatusNotFound {
		t.Fatalf("unknown channel = %d, want 404", code)
	}
	if code, _ := w.claim(t, "not-an-id", "?click="+clickA); code != http.StatusNotFound {
		t.Fatalf("malformed id = %d, want 404", code)
	}
	// Phase 30.29's form, /tgj/{botId}: the bot's first channel.
	if code, loc := w.claim(t, w.bot.ID, "?click="+clickA); code != http.StatusFound || loc != linkA {
		t.Fatalf("by bot id = %d %q, want A's link %q", code, loc, linkA)
	}

	// Clicks at the same moment never share a link.
	var mu sync.Mutex
	seen := map[string]bool{linkA: true, linkB: true}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, loc := w.claim(t, w.ch.ID, "?click="+idgen.New())
			mu.Lock()
			defer mu.Unlock()
			if seen[loc] {
				t.Errorf("link %q handed out twice", loc)
			}
			seen[loc] = true
		})
	}
	wg.Wait()
}

// A join through a click's link is TG_JOIN on that click — no /start
// needed — once; the same link passed on to someone else falls back to
// their own /start.
func TestInviteJoin(t *testing.T) {
	clicks := attribution.NewMemoryResolver()
	w := newInviteFixture(t, clicks)
	w.fillPool(t)
	click := attribution.Click{ClickID: idgen.New(), OrganizationID: w.org, CampaignID: "camp-tg", Country: "PL"}
	clicks.Record(click)
	_, link := w.claim(t, w.ch.ID, "?click="+click.ClickID)
	channel := w.ch.TgChatID
	const dave, erin = 2001, 2002

	joined := func(user int64, link string) map[string]any {
		u := member(channel, user, "left", "member")
		u["chat_member"].(map[string]any)["invite_link"] = map[string]any{"invite_link": link, "creator": map[string]any{"id": 1}}
		return u
	}
	if rec := w.post(t, w.secret, joined(dave, link)); rec.Code != http.StatusOK {
		t.Fatalf("join = %d", rec.Code)
	}
	w.post(t, w.secret, joined(dave, link)) // Telegram re-sent it
	w.post(t, w.secret, joined(erin, link)) // the link passed on: erin has no /start
	got := w.events.types()
	if len(got) != 1 || got[0] != event.TgJoin || w.events.events[0].ClickID != click.ClickID ||
		w.events.events[0].CampaignID != "camp-tg" || w.events.events[0].Country != "PL" {
		t.Fatalf("events = %+v, want one TG_JOIN on the link's click", w.events.events)
	}

	// Someone else's link (the shared one, another admin's) goes the
	// /start way: erin started from the click, so the join is hers.
	other := attribution.Click{ClickID: idgen.New(), OrganizationID: w.org, CampaignID: "camp-start"}
	clicks.Record(other)
	w.post(t, w.secret, start(erin, "/start "+other.ClickID))
	w.post(t, w.secret, joined(erin, "https://t.me/joinchan"))
	if got := w.events.types(); len(got) != 3 || got[2] != event.TgJoin || w.events.events[2].ClickID != other.ClickID {
		t.Fatalf("events = %v, want TG_START and TG_JOIN on erin's /start click", got)
	}
}

// A link claimed moments ago whose click is not in ClickHouse yet: 503,
// so Telegram re-sends the join. Later it is a click that never arrived:
// acknowledged, nothing recorded.
func TestInviteJoinWaitsForTheClick(t *testing.T) {
	w := newInviteFixture(t, attribution.NewMemoryResolver())
	w.fillPool(t)
	_, link := w.claim(t, w.ch.ID, "?click="+idgen.New())
	u := member(w.ch.TgChatID, 3001, "left", "member")
	u["chat_member"].(map[string]any)["invite_link"] = map[string]any{"invite_link": link}
	if rec := w.post(t, w.secret, u); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("join before the click is stored = %d, want 503", rec.Code)
	}
	if _, err := w.pool.Exec(context.Background(), `UPDATE telegram_invite_links SET claimed_at = now() - interval '1 hour' WHERE invite_link = $1`, link); err != nil {
		t.Fatal(err)
	}
	if rec := w.post(t, w.secret, u); rec.Code != http.StatusOK || len(w.events.types()) != 0 {
		t.Fatalf("join of a click that never arrived = %d, events %v; want 200 and nothing", rec.Code, w.events.types())
	}
}

// The worker revokes links claimed long ago, keeps the rest, tops the pool
// up again, works one organization's bots when scoped and carries on past
// a bot Telegram refuses.
func TestInvitePoolRevokesAndIsScoped(t *testing.T) {
	w := newInviteFixture(t, attribution.NewMemoryResolver())
	ctx := context.Background()
	w.fillPool(t)
	_, old := w.claim(t, w.ch.ID, "?click="+idgen.New())
	_, fresh := w.claim(t, w.ch.ID, "?click="+idgen.New())
	if _, err := w.pool.Exec(ctx, `UPDATE telegram_invite_links SET claimed_at = $2 WHERE invite_link = $1`,
		old, time.Now().Add(-telegram.InviteLinkTTL-time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Another organization's bot, also with a channel.
	other := w.seedOrg(t)
	token, me := w.api.addBot("other_bot")
	w.api.chats["@otherchan"] = telegram.Chat{ID: -1004444444, Type: "channel", Title: "Other", Username: "otherchan"}
	w.api.admins[me.ID] = true
	if _, err := w.svc.Connect(ctx, other, "", telegram.ConnectInput{Token: token, Channel: "@otherchan"}); err != nil {
		t.Fatal(err)
	}

	created, revoked, err := w.invitePool(w.org).RunOnce(ctx)
	if err != nil || created != 2 || revoked != 1 {
		t.Fatalf("RunOnce = %d created, %d revoked, %v; want 2 and 1", created, revoked, err)
	}
	if len(w.api.revoked) != 1 || w.api.revoked[0] != old {
		t.Fatalf("revoked = %v, want only %q", w.api.revoked, old)
	}
	var n int
	if err := w.pool.QueryRow(ctx, `SELECT count(*) FROM telegram_invite_links WHERE invite_link = ANY($1)`, []string{old, fresh}).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows left of old and fresh = %d, %v; want fresh only", n, err)
	}
	if err := w.pool.QueryRow(ctx, `SELECT count(*) FROM telegram_invite_links WHERE organization_id = $1`, other).Scan(&n); err != nil || n != 0 {
		t.Fatalf("other organization's links = %d, %v; want none from a scoped run", n, err)
	}
	if bots, err := w.svc.List(ctx, other); err != nil || len(bots) != 1 || bots[0].Channels[0].FreeInviteLinks != 0 {
		t.Fatalf("other organization's list = %+v, %v", bots, err)
	}

	// Telegram refusing (rights taken away) is logged, not fatal.
	if _, err := w.pool.Exec(ctx, `DELETE FROM telegram_invite_links WHERE bot_id = $1 AND click_id IS NULL`, w.bot.ID); err != nil {
		t.Fatal(err)
	}
	w.api.failInvites = true
	if created, _, err := w.invitePool(w.org).RunOnce(ctx); err != nil || created != 0 {
		t.Fatalf("refused = %d, %v; want 0 and no error", created, err)
	}
}

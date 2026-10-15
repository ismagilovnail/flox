package telegram_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/telegram"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

// fakeTelegram is the Bot API: bots by token, chats by "@name" or id,
// which bots are admins that can invite, and what was called.
type fakeTelegram struct {
	mu       sync.Mutex
	bots     map[string]telegram.User
	chats    map[string]telegram.Chat
	admins   map[int64]bool // bot id → admin with invite rights in every chat
	webhooks map[string]map[string]any
	deleted  []string
	invites  int
	revoked  []string
	// failInvites makes createChatInviteLink refuse (the bot lost its
	// admin rights).
	failInvites bool
}

func newFakeTelegram(t *testing.T) (*fakeTelegram, *httptest.Server) {
	f := &fakeTelegram{bots: map[string]telegram.User{}, chats: map[string]telegram.Chat{}, admins: map[int64]bool{}, webhooks: map[string]map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/bot"), "/", 2)
		token, method := parts[0], parts[1]
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		answer := func(result any) { _ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result}) }
		fail := func(code int, desc string) {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": code, "description": desc})
		}
		bot, ok := f.bots[token]
		if !ok {
			fail(401, "Unauthorized")
			return
		}
		switch method {
		case "getMe":
			answer(bot)
		case "setWebhook":
			f.webhooks[token] = p
			answer(true)
		case "deleteWebhook":
			f.deleted = append(f.deleted, token)
			answer(true)
		case "getChat":
			c, ok := f.chats[fmt.Sprint(p["chat_id"])]
			if !ok {
				fail(400, "Bad Request: chat not found")
				return
			}
			answer(c)
		case "getChatMember":
			if f.admins[bot.ID] {
				answer(map[string]any{"status": "administrator", "can_invite_users": true})
			} else {
				answer(map[string]any{"status": "member"})
			}
		case "createChatInviteLink":
			if f.failInvites {
				fail(400, "Bad Request: not enough rights to manage chat invite links")
				return
			}
			f.invites++
			if p["name"] == "FLOX" { // the channel's shared link, made on connect
				answer(map[string]any{"invite_link": "https://t.me/+private"})
				return
			}
			answer(map[string]any{"invite_link": fmt.Sprintf("https://t.me/+inv%d-%d", bot.ID, f.invites)})
		case "revokeChatInviteLink":
			f.revoked = append(f.revoked, fmt.Sprint(p["invite_link"]))
			answer(map[string]any{"invite_link": p["invite_link"]})
		default:
			fail(404, "Not Found")
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

// addBot registers a bot with a random Telegram id (tg_bot_id is unique
// across the shared dev database) and returns its token.
func (f *fakeTelegram) addBot(username string) (string, telegram.User) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := telegram.User{ID: rand.Int64N(1 << 40), IsBot: true, Username: username}
	token := fmt.Sprintf("%d:%s", u.ID, strings.Repeat("A", 35))
	f.bots[token] = u
	return token, u
}

type fixture struct {
	pool   *pgxpool.Pool
	repo   *telegram.Repository
	api    *fakeTelegram
	svc    *telegram.Service
	org    string
	logger *slog.Logger
	apiURL string
}

var key = mustKey()

func mustKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	api, srv := newFakeTelegram(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := telegram.NewRepository(pool)
	f := &fixture{pool: pool, repo: repo, api: api, logger: logger, apiURL: srv.URL,
		svc: telegram.NewService(repo, telegram.NewHTTPBotAPI(srv.URL), key, "https://t.example", logger)}
	f.org = f.seedOrg(t)
	return f
}

func (f *fixture) seedOrg(t *testing.T) string {
	t.Helper()
	org := idgen.New()
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO organizations (id, name) VALUES ($1, $2)`, org, "TG Org "+org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testpg.DeleteOrganization(context.Background(), f.pool, org) })
	return org
}

func fieldErr(err error, field string) bool {
	var apiErr *apierror.Error
	return errors.As(err, &apiErr) && apiErr.Fields[field] != ""
}

func status(err error) int {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}

func TestConnect(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	token, me := f.api.addBot("lucky_bot")
	f.api.chats["@luckychan"] = telegram.Chat{ID: -1001234567, Type: "channel", Title: "Lucky", Username: "luckychan"}
	f.api.admins[me.ID] = true
	b, err := f.svc.Connect(ctx, f.org, "", telegram.ConnectInput{Token: token, WelcomeText: "Hi!", Channel: "@luckychan"})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if b.Username != "lucky_bot" || b.DeepLink != "https://t.me/lucky_bot?start={click_id}" || len(b.Channels) != 1 ||
		b.Channels[0].TgChatID != -1001234567 || b.Channels[0].InviteLink != "https://t.me/luckychan" || b.Channels[0].Title != "Lucky" ||
		b.Channels[0].ClickInviteLink != "https://t.example/tgj/"+b.Channels[0].ID+"?click={click_id}" {
		t.Fatalf("bot = %+v (channels %+v)", b, b.Channels)
	}
	hook := f.api.webhooks[token]
	if hook["url"] != "https://t.example/tg/"+b.ID || len(fmt.Sprint(hook["secret_token"])) < 32 ||
		!strings.Contains(fmt.Sprint(hook["allowed_updates"]), "chat_member") {
		t.Fatalf("webhook = %v", hook)
	}
	// The token is stored encrypted, never as is.
	var stored string
	if err := f.pool.QueryRow(ctx, `SELECT token_encrypted FROM telegram_bots WHERE id = $1`, b.ID).Scan(&stored); err != nil || strings.Contains(stored, token) {
		t.Fatalf("stored token = %q, %v", stored, err)
	}

	// The same bot again (here or anywhere): 409, and its webhook is
	// removed rather than left pointing at a bot that was not stored.
	if _, err := f.svc.Connect(ctx, f.seedOrg(t), "", telegram.ConnectInput{Token: token}); status(err) != 409 {
		t.Fatalf("connect twice: %v, want 409", err)
	}

	// A private channel: the bot creates the invite link.
	token2, me2 := f.api.addBot("private_bot")
	f.api.chats["-1009876543"] = telegram.Chat{ID: -1009876543, Type: "channel", Title: "Private"}
	f.api.admins[me2.ID] = true
	b2, err := f.svc.Connect(ctx, f.org, "", telegram.ConnectInput{Token: token2, Channel: "-1009876543"})
	if err != nil || len(b2.Channels) != 1 || b2.Channels[0].InviteLink != "https://t.me/+private" || f.api.invites != 1 {
		t.Fatalf("private channel = %+v, %v (invites %d)", b2, err, f.api.invites)
	}

	list, err := f.svc.List(ctx, f.org)
	if err != nil || len(list) != 2 || list[0].ID != b2.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}

	// Another organization neither sees nor removes them.
	other := f.seedOrg(t)
	if theirs, _ := f.svc.List(ctx, other); len(theirs) != 0 {
		t.Fatalf("other organization lists %d bots", len(theirs))
	}
	if err := f.svc.Delete(ctx, other, b.ID); status(err) != 404 {
		t.Fatalf("other organization deletes: %v, want 404", err)
	}
	deletedBefore := len(f.api.deleted)
	if err := f.svc.Delete(ctx, f.org, b.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(f.api.deleted) != deletedBefore+1 || f.api.deleted[len(f.api.deleted)-1] != token {
		t.Fatalf("webhook not removed on delete: %v", f.api.deleted)
	}
}

func TestConnectRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	token, _ := f.api.addBot("not_admin_bot")
	f.api.chats["@somechan"] = telegram.Chat{ID: -1001111111, Type: "channel", Title: "Some"}
	f.api.chats["@agroup"] = telegram.Chat{ID: -1002222222, Type: "group", Title: "Group"}
	unknown := "12345678:" + strings.Repeat("B", 35)

	for name, tc := range map[string]struct {
		in    telegram.ConnectInput
		field string
	}{
		"malformed token":   {telegram.ConnectInput{Token: "nope"}, "token"},
		"unknown token":     {telegram.ConnectInput{Token: unknown}, "token"},
		"malformed channel": {telegram.ConnectInput{Token: token, Channel: "luckychan"}, "channel"},
		"no such channel":   {telegram.ConnectInput{Token: token, Channel: "@missingchan"}, "channel"},
		"not a channel":     {telegram.ConnectInput{Token: token, Channel: "@agroup"}, "channel"},
		"bot not an admin":  {telegram.ConnectInput{Token: token, Channel: "@somechan"}, "channel"},
		"welcome too long":  {telegram.ConnectInput{Token: token, WelcomeText: strings.Repeat("я", telegram.MaxWelcomeText+1)}, "welcomeText"},
	} {
		if _, err := f.svc.Connect(ctx, f.org, "", tc.in); !fieldErr(err, tc.field) {
			t.Errorf("%s: %v, want a %s error", name, err, tc.field)
		}
	}
	if len(f.api.webhooks) != 0 {
		t.Fatalf("a refused bot got a webhook: %v", f.api.webhooks)
	}

	unconfigured := telegram.NewService(f.repo, telegram.NewHTTPBotAPI("http://127.0.0.1:1"), key, "", f.logger)
	if _, err := unconfigured.Connect(ctx, f.org, "", telegram.ConnectInput{Token: token}); status(err) != 503 {
		t.Fatalf("no webhook base URL: %v, want 503", err)
	}
	// Telegram unreachable: the error never carries the token (it is in
	// the Bot API URL).
	down := telegram.NewService(f.repo, telegram.NewHTTPBotAPI("http://127.0.0.1:1"), key, "https://t.example", f.logger)
	_, err := down.Connect(ctx, f.org, "", telegram.ConnectInput{Token: token})
	if status(err) != 503 || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), strings.Repeat("A", 35)) {
		t.Fatalf("unreachable Telegram: %v, want 503 without the token", err)
	}
}

// --- webhook ---

type sink struct {
	mu     sync.Mutex
	events []event.Event
}

func (s *sink) Enqueue(e event.Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return true
}

func (s *sink) types() []event.Type {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []event.Type
	for _, e := range s.events {
		out = append(out, e.Type)
	}
	return out
}

type failingResolver struct{}

func (failingResolver) ByClickID(context.Context, string, string) (attribution.Click, error) {
	return attribution.Click{}, errors.New("clickhouse down")
}

func (failingResolver) ByExternalClickID(context.Context, string, string) ([]attribution.Click, error) {
	return nil, errors.New("clickhouse down")
}

type webhookFixture struct {
	*fixture
	bot    telegram.Bot
	ch     telegram.Channel // the bot's channel
	secret string
	router chi.Router
	events *sink
}

func newWebhookFixture(t *testing.T, clicks attribution.ClickResolver) *webhookFixture {
	f := newFixture(t)
	token, me := f.api.addBot("join_bot")
	f.api.chats["@joinchan"] = telegram.Chat{ID: -1003333333, Type: "channel", Title: "Join us", Username: "joinchan"}
	f.api.admins[me.ID] = true
	b, err := f.svc.Connect(context.Background(), f.org, "", telegram.ConnectInput{Token: token, WelcomeText: "Welcome!", Channel: "@joinchan"})
	if err != nil {
		t.Fatal(err)
	}
	w := &webhookFixture{fixture: f, bot: b, ch: b.Channels[0], secret: fmt.Sprint(f.api.webhooks[token]["secret_token"]), router: chi.NewRouter(), events: &sink{}}
	telegram.NewWebhook(f.repo, clicks, w.events, f.logger).Register(w.router)
	return w
}

func (w *webhookFixture) post(t *testing.T, secret string, update any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPost, "/tg/"+w.bot.ID, bytes.NewReader(body))
	req.Header.Set(telegram.SecretHeader, secret)
	rec := httptest.NewRecorder()
	w.router.ServeHTTP(rec, req)
	return rec
}

func start(user int64, text string) map[string]any {
	return map[string]any{"update_id": 1, "message": map[string]any{
		"chat": map[string]any{"id": user, "type": "private"}, "from": map[string]any{"id": user, "is_bot": false}, "text": text}}
}

func member(chat, user int64, oldStatus, newStatus string) map[string]any {
	return map[string]any{"update_id": 2, "chat_member": map[string]any{
		"chat":            map[string]any{"id": chat, "type": "channel"},
		"old_chat_member": map[string]any{"status": oldStatus, "user": map[string]any{"id": user}},
		"new_chat_member": map[string]any{"status": newStatus, "user": map[string]any{"id": user}}}}
}

func TestWebhook(t *testing.T) {
	clicks := attribution.NewMemoryResolver()
	w := newWebhookFixture(t, clicks)
	click := attribution.Click{ClickID: idgen.New(), OrganizationID: w.org, CampaignID: "camp-1", FlowID: "flow-1", Country: "DE",
		Subs: event.Subs{0: "ad-7"}}
	clicks.Record(click)
	theirs := attribution.Click{ClickID: idgen.New(), OrganizationID: idgen.New(), CampaignID: "theirs"}
	clicks.Record(theirs)
	const alice, bob, carol = 1001, 1002, 1003
	channel := w.ch.TgChatID

	// The secret is Telegram's proof; a wrong one is nobody.
	if rec := w.post(t, "wrong", start(alice, "/start "+click.ClickID)); rec.Code != http.StatusNotFound {
		t.Fatalf("wrong secret = %d, want 404", rec.Code)
	}
	if rec := w.post(t, "", start(alice, "/start "+click.ClickID)); rec.Code != http.StatusNotFound {
		t.Fatalf("no secret = %d, want 404", rec.Code)
	}

	// /start from a click's deep link: TG_START with the click's context,
	// answered with the welcome and the channel button.
	rec := w.post(t, w.secret, start(alice, "/start "+click.ClickID))
	var reply map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("start = %d %s", rec.Code, rec.Body.String())
	}
	if reply["method"] != "sendMessage" || reply["text"] != "Welcome!" || reply["chat_id"] != float64(alice) ||
		!strings.Contains(rec.Body.String(), "https://t.me/joinchan") || !strings.Contains(rec.Body.String(), "Join us") {
		t.Fatalf("reply = %s", rec.Body.String())
	}
	w.post(t, w.secret, start(alice, "/start "+click.ClickID)) // again: no second TG_START
	w.post(t, w.secret, start(bob, "/start "+theirs.ClickID))  // another organization's click
	w.post(t, w.secret, start(bob, "/start "+idgen.New()))     // no such click
	w.post(t, w.secret, start(bob, "/start"))                  // no payload: welcome only
	group := start(carol, "/start "+click.ClickID)
	group["message"].(map[string]any)["chat"] = map[string]any{"id": -5, "type": "group"}
	w.post(t, w.secret, group) // not a private chat

	// Joining the channel: TG_JOIN on alice's click, once; bob joined
	// without a known click; another chat is not the bot's channel.
	w.post(t, w.secret, member(channel, alice, "left", "member"))
	w.post(t, w.secret, member(channel, alice, "left", "member")) // Telegram re-sent it
	w.post(t, w.secret, member(channel, bob, "left", "member"))
	w.post(t, w.secret, member(-1009999999, alice, "left", "member"))
	w.post(t, w.secret, member(channel, alice, "member", "administrator")) // promoted, not a join

	got := w.events.types()
	if len(got) != 2 || got[0] != event.TgStart || got[1] != event.TgJoin {
		t.Fatalf("events = %v, want TG_START then TG_JOIN", got)
	}
	for _, e := range w.events.events {
		if e.ClickID != click.ClickID || e.OrganizationID != w.org || e.CampaignID != "camp-1" || e.FlowID != "flow-1" ||
			e.Country != "DE" || e.Subs[0] != "ad-7" || e.EventAt.IsZero() {
			t.Fatalf("%s = %+v, want the click's context", e.Type, e)
		}
	}

	// A join request to the channel is approved.
	rec = w.post(t, w.secret, map[string]any{"update_id": 3, "chat_join_request": map[string]any{
		"chat": map[string]any{"id": channel, "type": "channel"}, "from": map[string]any{"id": carol}}})
	if !strings.Contains(rec.Body.String(), `"method":"approveChatJoinRequest"`) || !strings.Contains(rec.Body.String(), fmt.Sprint(carol)) {
		t.Fatalf("join request reply = %s", rec.Body.String())
	}

	// alice comes back through another ad: a new TG_START; already in the
	// channel, she does not join again.
	second := attribution.Click{ClickID: idgen.New(), OrganizationID: w.org, CampaignID: "camp-2"}
	clicks.Record(second)
	w.post(t, w.secret, start(alice, "/start "+second.ClickID))
	if got := w.events.types(); len(got) != 3 || got[2] != event.TgStart || w.events.events[2].CampaignID != "camp-2" {
		t.Fatalf("events after a second click = %v", got)
	}

	// Garbage is acknowledged, not retried forever.
	req := httptest.NewRequest(http.MethodPost, "/tg/"+w.bot.ID, strings.NewReader("{not json"))
	req.Header.Set(telegram.SecretHeader, w.secret)
	rr := httptest.NewRecorder()
	w.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("garbage = %d, want 200", rr.Code)
	}
}

// ClickHouse not answering: 503, so Telegram re-sends the update, and
// nothing is marked done meanwhile.
func TestWebhookRetriesWhenClicksAreUnavailable(t *testing.T) {
	w := newWebhookFixture(t, failingResolver{})
	if rec := w.post(t, w.secret, start(1, "/start "+idgen.New())); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("resolver down = %d, want 503", rec.Code)
	}
	var n int
	if err := w.pool.QueryRow(context.Background(), `SELECT count(*) FROM telegram_users WHERE bot_id = $1`, w.bot.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("telegram_users = %d, %v, want nothing stored", n, err)
	}
	if len(w.events.types()) != 0 {
		t.Fatal("events recorded with the resolver down")
	}
}

func TestWebhookUnknownBot(t *testing.T) {
	w := newWebhookFixture(t, attribution.NewMemoryResolver())
	req := httptest.NewRequest(http.MethodPost, "/tg/"+idgen.New(), strings.NewReader("{}"))
	req.Header.Set(telegram.SecretHeader, w.secret)
	rec := httptest.NewRecorder()
	w.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown bot = %d, want 404", rec.Code)
	}
}

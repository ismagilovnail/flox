package telegram_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/integrations"
	"github.com/ismagilovnail/flox/apps/internal/telegram"
)

// A new token of the same bot (revoked and reissued in @BotFather)
// replaces the old one: the webhook is set again with it and a new secret,
// the old secret stops working, channels and the /start history stay.
// Another bot's token, a malformed or refused one, another organization:
// refused, nothing changed.
func TestChangeToken(t *testing.T) {
	clicks := attribution.NewMemoryResolver()
	w := newWebhookFixture(t, clicks)
	ctx := context.Background()
	click := attribution.Click{ClickID: idgen.New(), OrganizationID: w.org, CampaignID: "camp-token"}
	clicks.Record(click)
	const user = 5001
	w.post(t, w.secret, start(user, "/start "+click.ClickID))

	var oldToken string
	var me telegram.User
	for tok, u := range w.api.bots {
		oldToken, me = tok, u
	}
	newToken := fmt.Sprintf("%d:%s", me.ID, strings.Repeat("B", 35))
	w.api.bots[newToken] = me
	otherToken, other := w.api.addBot("other_bot")

	unchanged := func(when string) {
		t.Helper()
		if tok := storedToken(t, w); tok != oldToken {
			t.Fatalf("%s: token changed", when)
		}
	}
	if _, err := w.svc.ChangeToken(ctx, w.org, w.bot.ID, otherToken); !fieldErr(err, "token") {
		t.Fatalf("another bot's token = %v, want a token field error", err)
	} else if _, set := w.api.webhooks[otherToken]; set {
		t.Fatalf("another bot's webhook was set (%s)", other.Username)
	}
	unchanged("another bot")
	if _, err := w.svc.ChangeToken(ctx, w.org, w.bot.ID, "nope"); !fieldErr(err, "token") {
		t.Fatalf("malformed = %v, want a token field error", err)
	}
	if _, err := w.svc.ChangeToken(ctx, w.org, w.bot.ID, fmt.Sprintf("%d:%s", me.ID, strings.Repeat("C", 35))); !fieldErr(err, "token") {
		t.Fatalf("refused by Telegram = %v, want a token field error", err)
	}
	if _, err := w.svc.ChangeToken(ctx, w.seedOrg(t), w.bot.ID, newToken); status(err) != http.StatusNotFound {
		t.Fatalf("another organization = %v, want 404", err)
	}
	unchanged("refusals")

	b, err := w.svc.ChangeToken(ctx, w.org, w.bot.ID, " "+newToken+" ")
	if err != nil || len(b.Channels) != 1 || b.Channels[0].ID != w.ch.ID || b.Username != me.Username {
		t.Fatalf("ChangeToken = %+v, %v; want the bot with its channel", b, err)
	}
	hook := w.api.webhooks[newToken]
	newSecret := fmt.Sprint(hook["secret_token"])
	if hook == nil || hook["url"] != "https://t.example/tg/"+w.bot.ID || newSecret == w.secret || newSecret == "" {
		t.Fatalf("webhook for the new token = %v", hook)
	}
	if tok := storedToken(t, w); tok != newToken {
		t.Fatal("the new token was not stored")
	}

	if rec := w.post(t, w.secret, member(w.ch.TgChatID, user, "left", "member")); rec.Code != http.StatusNotFound {
		t.Fatalf("old secret = %d, want 404", rec.Code)
	}
	if rec := w.post(t, newSecret, member(w.ch.TgChatID, user, "left", "member")); rec.Code != http.StatusOK {
		t.Fatalf("new secret = %d", rec.Code)
	}
	if got := w.events.types(); len(got) != 2 || got[1] != event.TgJoin || w.events.events[1].ClickID != click.ClickID {
		t.Fatalf("events = %v, want the join on the /start made before the change", got)
	}
}

func storedToken(t *testing.T, w *webhookFixture) string {
	t.Helper()
	var enc string
	if err := w.pool.QueryRow(context.Background(), `SELECT token_encrypted FROM telegram_bots WHERE id = $1`, w.bot.ID).Scan(&enc); err != nil {
		t.Fatal(err)
	}
	tok, err := integrations.Decrypt(key, enc)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// Reports name channels for whoever reads them: the organization's own
// channels only.
func TestChannelNames(t *testing.T) {
	w := newWebhookFixture(t, attribution.NewMemoryResolver())
	ctx := context.Background()
	names, err := w.svc.ChannelNames(ctx, w.org)
	if err != nil || len(names) != 1 || names[0].ID != w.ch.ID || names[0].Title != "Join us" || names[0].BotUsername != "join_bot" {
		t.Fatalf("ChannelNames = %+v, %v", names, err)
	}
	if names, err := w.svc.ChannelNames(ctx, w.seedOrg(t)); err != nil || len(names) != 0 {
		t.Fatalf("another organization = %+v, %v", names, err)
	}
}

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
	"github.com/ismagilovnail/flox/apps/internal/telegram"
)

// A bot gets channels after connecting, up to MaxChannels, each checked
// with Telegram like the first; its welcome text can change; another
// organization can do none of it.
func TestChannelsAndEditing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	token, me := f.api.addBot("multi_bot")
	f.api.admins[me.ID] = true
	f.api.chats["@firstchan"] = telegram.Chat{ID: -1005550001, Type: "channel", Title: "First", Username: "firstchan"}
	f.api.chats["-1005550002"] = telegram.Chat{ID: -1005550002, Type: "supergroup", Title: "Private"}

	b, err := f.svc.Connect(ctx, f.org, "", telegram.ConnectInput{Token: token, WelcomeText: "Hi"})
	if err != nil || len(b.Channels) != 0 {
		t.Fatalf("Connect = %+v, %v; want no channels", b, err)
	}
	if b, err = f.svc.AddChannel(ctx, f.org, b.ID, " @firstchan "); err != nil || len(b.Channels) != 1 ||
		b.Channels[0].TgChatID != -1005550001 || b.Channels[0].InviteLink != "https://t.me/firstchan" || b.Channels[0].ID == "" {
		t.Fatalf("AddChannel public = %+v, %v", b, err)
	}
	if b, err = f.svc.AddChannel(ctx, f.org, b.ID, "-1005550002"); err != nil || len(b.Channels) != 2 ||
		b.Channels[1].InviteLink != "https://t.me/+private" || b.Channels[1].Title != "Private" {
		t.Fatalf("AddChannel private = %+v, %v", b, err)
	}
	if _, err := f.svc.AddChannel(ctx, f.org, b.ID, "@firstchan"); status(err) != http.StatusConflict {
		t.Fatalf("same channel again = %v, want 409", err)
	}
	for in, field := range map[string]string{"firstchan": "channel", "@missingchan": "channel"} {
		if _, err := f.svc.AddChannel(ctx, f.org, b.ID, in); !fieldErr(err, field) {
			t.Fatalf("AddChannel(%q) = %v, want a %s field error", in, err, field)
		}
	}

	// Up to MaxChannels.
	for i := len(b.Channels); i < telegram.MaxChannels; i++ {
		id := fmt.Sprintf("-10066600%02d", i)
		f.api.chats[id] = telegram.Chat{ID: -1006660000 - int64(i), Type: "channel", Title: id}
		if _, err := f.svc.AddChannel(ctx, f.org, b.ID, id); err != nil {
			t.Fatalf("channel %d: %v", i+1, err)
		}
	}
	f.api.chats["-1007770000"] = telegram.Chat{ID: -1007770000, Type: "channel", Title: "One too many"}
	if _, err := f.svc.AddChannel(ctx, f.org, b.ID, "-1007770000"); !fieldErr(err, "channel") {
		t.Fatalf("channel %d = %v, want a channel field error", telegram.MaxChannels+1, err)
	}

	if b, err = f.svc.UpdateWelcome(ctx, f.org, b.ID, "  Welcome back  "); err != nil || b.WelcomeText != "Welcome back" {
		t.Fatalf("UpdateWelcome = %q, %v", b.WelcomeText, err)
	}
	if _, err := f.svc.UpdateWelcome(ctx, f.org, b.ID, strings.Repeat("x", telegram.MaxWelcomeText+1)); !fieldErr(err, "welcomeText") {
		t.Fatalf("too long = %v, want a welcomeText field error", err)
	}

	// Another organization sees none of it and can change none of it.
	other := f.seedOrg(t)
	if _, err := f.svc.UpdateWelcome(ctx, other, b.ID, "mine"); status(err) != http.StatusNotFound {
		t.Fatalf("other org UpdateWelcome = %v, want 404", err)
	}
	if _, err := f.svc.AddChannel(ctx, other, b.ID, "-1007770000"); status(err) != http.StatusNotFound {
		t.Fatalf("other org AddChannel = %v, want 404", err)
	}
	if _, err := f.svc.RemoveChannel(ctx, other, b.ID, b.Channels[0].ID); status(err) != http.StatusNotFound {
		t.Fatalf("other org RemoveChannel = %v, want 404", err)
	}
	if bots, err := f.svc.List(ctx, other); err != nil || len(bots) != 0 {
		t.Fatalf("other org List = %v, %v", bots, err)
	}

	removed := b.Channels[0].ID
	if b, err = f.svc.RemoveChannel(ctx, f.org, b.ID, removed); err != nil || len(b.Channels) != telegram.MaxChannels-1 || b.Channels[0].ID == removed {
		t.Fatalf("RemoveChannel = %d channels, %v", len(b.Channels), err)
	}
	if _, err := f.svc.RemoveChannel(ctx, f.org, b.ID, removed); status(err) != http.StatusNotFound {
		t.Fatalf("removed again = %v, want 404", err)
	}
	if b, err = f.svc.Get(ctx, f.org, b.ID); err != nil || b.WelcomeText != "Welcome back" || len(b.Channels) != telegram.MaxChannels-1 {
		t.Fatalf("Get = %+v, %v", b, err)
	}
}

// With two channels: /start offers both; joining either is TG_JOIN, once
// per click; a join request to either is approved; each channel has its
// own pool, and a link counts only in its own channel.
func TestWebhookSeveralChannels(t *testing.T) {
	clicks := attribution.NewMemoryResolver()
	w := newInviteFixture(t, clicks)
	ctx := context.Background()
	w.api.chats["@secondchan"] = telegram.Chat{ID: -1003333334, Type: "channel", Title: "Second", Username: "secondchan"}
	b, err := w.svc.AddChannel(ctx, w.org, w.bot.ID, "@secondchan")
	if err != nil {
		t.Fatal(err)
	}
	first, second := b.Channels[0], b.Channels[1]
	click := attribution.Click{ClickID: idgen.New(), OrganizationID: w.org, CampaignID: "camp-2ch"}
	clicks.Record(click)
	const frank, gina = 4001, 4002

	rec := w.post(t, w.secret, start(frank, "/start "+click.ClickID))
	if body := rec.Body.String(); !strings.Contains(body, "https://t.me/joinchan") || !strings.Contains(body, "https://t.me/secondchan") {
		t.Fatalf("welcome = %s, want a button per channel", body)
	}
	w.post(t, w.secret, member(second.TgChatID, frank, "left", "member"))
	w.post(t, w.secret, member(first.TgChatID, frank, "left", "member")) // the other channel, same click
	if got := w.events.types(); len(got) != 2 || got[1] != event.TgJoin {
		t.Fatalf("events = %v, want TG_START and one TG_JOIN", got)
	}
	rec = w.post(t, w.secret, map[string]any{"update_id": 5, "chat_join_request": map[string]any{
		"chat": map[string]any{"id": second.TgChatID, "type": "channel"}, "from": map[string]any{"id": gina}}})
	if !strings.Contains(rec.Body.String(), `"approveChatJoinRequest"`) || !strings.Contains(rec.Body.String(), fmt.Sprint(second.TgChatID)) {
		t.Fatalf("join request to the second channel = %s", rec.Body.String())
	}

	// Pools per channel.
	w.fillPool(t)
	for range 5 {
		if _, _, err := w.invitePool(w.org).RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	bots, _ := w.svc.List(ctx, w.org)
	if bots[0].Channels[0].FreeInviteLinks != telegram.InvitePoolSize || bots[0].Channels[1].FreeInviteLinks != telegram.InvitePoolSize {
		t.Fatalf("free = %d, %d; want %d each", bots[0].Channels[0].FreeInviteLinks, bots[0].Channels[1].FreeInviteLinks, telegram.InvitePoolSize)
	}
	other := attribution.Click{ClickID: idgen.New(), OrganizationID: w.org, CampaignID: "camp-link"}
	clicks.Record(other)
	_, link := w.claim(t, second.ID, "?click="+other.ClickID)
	if bots, _ := w.svc.List(ctx, w.org); bots[0].Channels[1].FreeInviteLinks != telegram.InvitePoolSize-1 || bots[0].Channels[0].FreeInviteLinks != telegram.InvitePoolSize {
		t.Fatalf("claim through the second channel took from %+v", bots[0].Channels)
	}
	joined := func(chat int64) map[string]any {
		u := member(chat, gina, "left", "member")
		u["chat_member"].(map[string]any)["invite_link"] = map[string]any{"invite_link": link}
		return u
	}
	w.post(t, w.secret, joined(first.TgChatID)) // the link reported in another channel: not its join
	if got := w.events.types(); len(got) != 2 {
		t.Fatalf("events = %v, want nothing new for a link outside its channel", got)
	}
	w.post(t, w.secret, joined(second.TgChatID))
	if got := w.events.types(); len(got) != 3 || w.events.events[2].ClickID != other.ClickID {
		t.Fatalf("events = %v, want TG_JOIN on the link's click", got)
	}
}

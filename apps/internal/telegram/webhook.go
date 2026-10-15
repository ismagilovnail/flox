package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

// EventSink is where the webhook's events go (apps/tracker's event buffer).
type EventSink interface {
	Enqueue(e event.Event) bool
}

// Webhook serves POST /tg/{botId} on apps/tracker: the updates Telegram
// sends a connected bot. It records TG_START and TG_JOIN on the visit's
// click, and answers in the HTTP response (Telegram runs a Bot API method
// returned there), so the tracker never calls Telegram itself.
type Webhook struct {
	repo   *Repository
	clicks attribution.ClickResolver
	events EventSink
	logger *slog.Logger
	now    func() time.Time
}

func NewWebhook(repo *Repository, clicks attribution.ClickResolver, events EventSink, logger *slog.Logger) *Webhook {
	return &Webhook{repo: repo, clicks: clicks, events: events, logger: logger, now: time.Now}
}

// SecretHeader carries the webhook secret set with setWebhook.
const SecretHeader = "X-Telegram-Bot-Api-Secret-Token"

func (h *Webhook) Register(r chi.Router) {
	r.Post("/tg/{botId}", h.serve)
}

// serve answers 200 to everything it has handled or chooses to ignore —
// Telegram retries anything else — and 503 when a click cannot be looked
// up yet (ClickHouse down), so the update comes back.
func (h *Webhook) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	b, ok, err := h.repo.webhookBot(ctx, chi.URLParam(r, "botId"))
	if err != nil {
		h.logger.Error("telegram webhook: reading bot", "error", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	got := hashSecret(r.Header.Get(SecretHeader))
	if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(b.SecretHash)) != 1 {
		http.NotFound(w, r)
		return
	}
	var upd Update
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&upd); err != nil {
		w.WriteHeader(http.StatusOK) // not an update we can read; a retry would not help
		return
	}
	reply, err := h.handle(ctx, b, upd)
	if errors.Is(err, errClickPending) {
		h.logger.Info("telegram webhook: join through an invite link before its click is stored, retried", "bot_id", b.ID)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		h.logger.Error("telegram webhook", "bot_id", b.ID, "error", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if reply == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reply)
}

// handle records what upd means and returns the Bot API call to answer
// with (nil: none).
func (h *Webhook) handle(ctx context.Context, b webhookBot, upd Update) (map[string]any, error) {
	switch {
	case upd.Message != nil:
		return h.message(ctx, b, upd.Message)
	case upd.ChatMember != nil:
		return nil, h.chatMember(ctx, b, upd.ChatMember)
	case upd.ChatJoinRequest != nil:
		// The bot manages its channels: anyone asking to join one is let in,
		// and the chat_member update that follows records the join.
		if c, ok := b.channel(upd.ChatJoinRequest.Chat.ID); ok {
			return map[string]any{"method": "approveChatJoinRequest", "chat_id": c.TgChatID, "user_id": upd.ChatJoinRequest.From.ID}, nil
		}
	}
	return nil, nil
}

// message handles /start in a private chat: "/start <click_id>" from a
// click's deep link records TG_START (once per click) and every /start is
// answered with the welcome message.
func (h *Webhook) message(ctx context.Context, b webhookBot, m *Message) (map[string]any, error) {
	if m.Chat.Type != "private" || m.From == nil {
		return nil, nil
	}
	cmd, payload, _ := strings.Cut(strings.TrimSpace(m.Text), " ")
	if cmd != "/start" {
		return nil, nil
	}
	if payload = strings.TrimSpace(payload); clickPattern.MatchString(payload) {
		click, found, err := h.click(ctx, b, payload)
		if err != nil {
			return nil, err
		}
		if found {
			isNew, err := h.repo.recordStart(ctx, b, m.From.ID, click.ClickID, h.now())
			if err != nil {
				return nil, err
			}
			if isNew {
				h.record(event.TgStart, b, click)
			}
		}
	}
	return h.welcome(b, m.Chat.ID), nil
}

// chatMember records TG_JOIN when someone becomes a member of one of the
// bot's channels: on the click of the per-click invite link they joined
// through, else on the click of their last /start. One TG_JOIN per click,
// whichever channel — the event does not name one, so a second channel
// joined from the same click is not a second join.
func (h *Webhook) chatMember(ctx context.Context, b webhookBot, u *ChatMemberUpdated) error {
	c, ok := b.channel(u.Chat.ID)
	if !ok || !isMember(u.NewChatMember.Status) || isMember(u.OldChatMember.Status) {
		return nil
	}
	if u.InviteLink != nil {
		done, err := h.inviteJoin(ctx, b, c, u.InviteLink.InviteLink)
		if err != nil || done {
			return err
		}
	}
	userID := u.NewChatMember.User.ID
	clickID, joined, err := h.repo.startedClick(ctx, b, userID)
	if err != nil || clickID == "" || joined {
		return err // joined without /start: no click to attribute it to
	}
	click, found, err := h.click(ctx, b, clickID)
	if err != nil || !found {
		return err
	}
	isNew, err := h.repo.recordJoin(ctx, b, userID, clickID, h.now())
	if err != nil {
		return err
	}
	if isNew {
		h.record(event.TgJoin, b, click)
	}
	return nil
}

// inviteJoin records TG_JOIN on the click link was claimed for, once per
// link. done: the join is the link's (recorded now or before); false
// leaves it to the /start path — not one of the bot's claimed links, or
// its click is unknown.
func (h *Webhook) inviteJoin(ctx context.Context, b webhookBot, c webhookChannel, link string) (bool, error) {
	claim, found, err := h.repo.invitedClick(ctx, c, link)
	if err != nil || !found {
		return false, err
	}
	if claim.Joined {
		// The link's click already joined: someone it was passed on to
		// may still have a /start of their own.
		return false, nil
	}
	click, found, err := h.click(ctx, b, claim.ClickID)
	if err != nil {
		return false, err
	}
	if !found {
		if h.now().Sub(claim.ClaimedAt) < clickLag {
			// Claimed moments ago: the click is still on its way to
			// ClickHouse. Telegram re-sends the update.
			return false, errClickPending
		}
		return false, nil
	}
	isNew, err := h.repo.recordInviteJoin(ctx, c, link, h.now())
	if err != nil {
		return false, err
	}
	if isNew {
		h.record(event.TgJoin, b, click)
	}
	return true, nil
}

var errClickPending = errors.New("telegram: invite link's click not in ClickHouse yet")

func isMember(status string) bool {
	return status == "member" || status == "administrator" || status == "creator"
}

// click finds clickID among the bot's organization's clicks. Not found is
// an answer (nothing to record); any other error means "try again".
func (h *Webhook) click(ctx context.Context, b webhookBot, clickID string) (attribution.Click, bool, error) {
	click, err := h.clicks.ByClickID(ctx, b.OrganizationID, clickID)
	if errors.Is(err, attribution.ErrClickNotFound) {
		return attribution.Click{}, false, nil
	}
	if err != nil {
		return attribution.Click{}, false, err
	}
	return click, true, nil
}

func (h *Webhook) record(t event.Type, b webhookBot, click attribution.Click) {
	ev := event.Event{Type: t, EventAt: h.now().UTC(), OrganizationID: b.OrganizationID, ClickID: click.ClickID}
	click.CopyTo(&ev)
	if !h.events.Enqueue(ev) {
		h.logger.Error("telegram webhook: event buffer full, event dropped", "type", t, "click_id", click.ClickID)
	}
}

// welcome is the answer to /start: the welcome text, with a button to each
// channel. Neither: no answer.
func (h *Webhook) welcome(b webhookBot, chatID int64) map[string]any {
	var rows [][]map[string]string
	for _, c := range b.Channels {
		if c.InviteLink == "" {
			continue
		}
		label := c.Title
		if label == "" {
			label = c.InviteLink
		}
		rows = append(rows, []map[string]string{{"text": label, "url": c.InviteLink}})
	}
	text := b.WelcomeText
	if text == "" && len(rows) > 0 {
		text = rows[0][0]["url"]
	}
	if text == "" {
		return nil
	}
	msg := map[string]any{"method": "sendMessage", "chat_id": chatID, "text": text}
	if len(rows) > 0 {
		msg["reply_markup"] = map[string]any{"inline_keyboard": rows}
	}
	return msg
}

// Package telegram is an organization's own Telegram bots (docs/telegram.md;
// §43 TG_START, TG_JOIN): apps/api connects a bot (its token, the channel it
// administers) and points its webhook at apps/tracker, which records
// TG_START when someone opens the bot from a click's deep link
// (t.me/<bot>?start=<click_id>) and TG_JOIN when that person then joins the
// bot's channel.
package telegram

import (
	"regexp"
	"time"
)

// Bot is a connected bot as the API shows it. The token never leaves the
// server.
type Bot struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	WelcomeText string    `json:"welcomeText"`
	Channel     *Channel  `json:"channel"`
	CreatedAt   time.Time `json:"createdAt"`
	// DeepLink is what to put as a destination URL: the {click_id} macro
	// makes each visit open the bot with its own click.
	DeepLink string `json:"deepLink"`
	// ClickInviteLink (a bot with a channel, on a configured deployment):
	// a destination URL sending each click to its own invite link of the
	// channel, so a join is attributed without a /start. FreeInviteLinks is
	// the pool left (InvitePool keeps it at InvitePoolSize).
	ClickInviteLink string `json:"clickInviteLink,omitempty"`
	FreeInviteLinks int    `json:"freeInviteLinks"`
}

// Channel is the channel a bot administers: joining it is TG_JOIN.
type Channel struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	InviteLink string `json:"inviteLink"`
}

// ConnectInput connects a bot. Channel is optional: "@username" or the
// numeric id ("-100…") of a channel the bot is an admin of.
type ConnectInput struct {
	Token       string
	WelcomeText string
	Channel     string
}

// MaxWelcomeText bounds the welcome message (Telegram allows 4096).
const MaxWelcomeText = 2000

var (
	tokenPattern   = regexp.MustCompile(`^\d{5,15}:[A-Za-z0-9_-]{30,50}$`)
	channelPattern = regexp.MustCompile(`^(@[A-Za-z][A-Za-z0-9_]{4,31}|-100\d{5,15})$`)
	// clickPattern is a click id (a ULID) as a /start payload carries it.
	clickPattern = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
)

func deepLink(username string) string {
	return "https://t.me/" + username + "?start={click_id}"
}

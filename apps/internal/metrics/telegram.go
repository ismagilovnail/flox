package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// TelegramInviteFallbackTotal: clicks the tracker's /tgj/ redirect sent to
// a channel's shared link because the bot's invite-link pool was empty —
// their joins are attributed only through a /start (docs/telegram.md).
var TelegramInviteFallbackTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: namespace,
	Name:      "telegram_invite_fallback_total",
	Help:      "Clicks sent to a Telegram channel's shared link because the per-click invite pool was empty.",
})

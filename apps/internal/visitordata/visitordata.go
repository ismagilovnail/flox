// Package visitordata answers a visitor's data request (audit 2026-10-15,
// GDPR-1; docs/visitor-data.md): an organization, as the controller of its
// visitors' data, finds what FLOX holds about one visitor — by click id or
// by IP address — and erases it.
//
// Erasure deletes the visitor's own data: their clicks and visits (IP,
// user agent, referrer, geo, device), their push subscription, their
// Telegram user, the pixel sends that carried their IP and user agent. It
// keeps the organization's business records — conversions, the postback
// ledger, outgoing postbacks — which name the visitor only by a
// pseudonymous click id: they are what invoices and disputes rest on.
// Daily aggregates keep their counts (no personal data in them).
package visitordata

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// MaxClickIDs bounds one request: an IP address shared by a whole office
// or a mobile carrier's NAT can stand for thousands of people, and a
// request is about one person.
const MaxClickIDs = 500

// maxRows bounds each list in a report.
const maxRows = 1000

type Click struct {
	At         time.Time `json:"at"`
	Type       string    `json:"type"`
	ClickID    string    `json:"clickId"`
	CampaignID string    `json:"campaignId"`
	Country    string    `json:"country"`
	City       string    `json:"city"`
	Device     string    `json:"device"`
	OS         string    `json:"os"`
	Browser    string    `json:"browser"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"userAgent"`
	Referrer   string    `json:"referrer"`
}

type Event struct {
	At      time.Time `json:"at"`
	Type    string    `json:"type"`
	ClickID string    `json:"clickId"`
}

type Conversion struct {
	At       time.Time `json:"at"`
	Type     string    `json:"type"`
	ClickID  string    `json:"clickId"`
	Revenue  float64   `json:"revenue"`
	Currency string    `json:"currency"`
}

type PushSubscription struct {
	ClickID   string    `json:"clickId"`
	CreatedAt time.Time `json:"createdAt"`
	Status    string    `json:"status"`
	Country   string    `json:"country"`
	Device    string    `json:"device"`
	Language  string    `json:"language"`
	// Endpoint is the push service's host only: the full URL is a
	// credential for sending to this browser.
	EndpointHost string `json:"endpointHost"`
}

type TelegramUser struct {
	ClickID   string    `json:"clickId"`
	TgUserID  int64     `json:"tgUserId"`
	StartedAt time.Time `json:"startedAt"`
}

// Report is what FLOX holds about the visitor. Truncated: the IP matched
// more than MaxClickIDs clicks; only the first are listed.
type Report struct {
	ClickIDs          []string           `json:"clickIds"`
	Truncated         bool               `json:"truncated"`
	Clicks            []Click            `json:"clicks"`
	Events            []Event            `json:"events"`
	Conversions       []Conversion       `json:"conversions"`
	PushSubscriptions []PushSubscription `json:"pushSubscriptions"`
	TelegramUsers     []TelegramUser     `json:"telegramUsers"`
	PixelDeliveries   int                `json:"pixelDeliveries"`
}

// Erased counts what Erase deleted. ClickHouse rows are not counted: a
// lightweight delete does not report them.
type Erased struct {
	PushSubscriptions int64 `json:"pushSubscriptions"`
	TelegramUsers     int64 `json:"telegramUsers"`
	TelegramJoins     int64 `json:"telegramJoins"`
	TelegramInvites   int64 `json:"telegramInvites"`
	PixelDeliveries   int64 `json:"pixelDeliveries"`
}

type Service struct {
	db *pgxpool.Pool
	ch driver.Conn
}

func NewService(db *pgxpool.Pool, ch driver.Conn) *Service { return &Service{db: db, ch: ch} }

// Find reports what FLOX holds about the visitor of clickID, or of every
// click from ip — exactly one of the two. Scoped to orgID (CLAUDE.md #5).
// An IP finds clicks only while IPs are kept (RETENTION_IP_DAYS).
func (s *Service) Find(ctx context.Context, orgID, clickID, ip string) (Report, error) {
	clickID, ip = strings.TrimSpace(clickID), strings.TrimSpace(ip)
	if (clickID == "") == (ip == "") {
		return Report{}, apierror.Validation("give a click id or an IP address", map[string]string{"clickId": "one of clickId, ip"})
	}
	if s.ch == nil {
		return Report{}, apierror.Unavailable("ClickHouse is not configured")
	}
	r := Report{ClickIDs: []string{clickID}}
	if ip != "" {
		parsed := net.ParseIP(ip)
		if parsed == nil {
			return Report{}, apierror.Validation("not an IP address", map[string]string{"ip": "invalid"})
		}
		ids, err := s.clickIDsByIP(ctx, orgID, parsed.String())
		if err != nil {
			return Report{}, err
		}
		if len(ids) > MaxClickIDs {
			ids, r.Truncated = ids[:MaxClickIDs], true
		}
		r.ClickIDs = ids
	}
	if len(r.ClickIDs) == 0 {
		return emptyReport(), nil
	}
	if err := s.fill(ctx, orgID, &r); err != nil {
		return Report{}, err
	}
	return r, nil
}

func emptyReport() Report {
	return Report{ClickIDs: []string{}, Clicks: []Click{}, Events: []Event{}, Conversions: []Conversion{},
		PushSubscriptions: []PushSubscription{}, TelegramUsers: []TelegramUser{}}
}

func (s *Service) clickIDsByIP(ctx context.Context, orgID, ip string) ([]string, error) {
	rows, err := s.ch.Query(ctx, `
		SELECT click_id FROM click_events
		WHERE organization_id = ? AND ip = ? AND click_id != ''
		GROUP BY click_id ORDER BY min(event_at)
		LIMIT ?`, orgID, ip, MaxClickIDs+1)
	if err != nil {
		return nil, fmt.Errorf("visitordata: finding clicks by IP: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("visitordata: reading clicks: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Service) fill(ctx context.Context, orgID string, r *Report) error {
	ids := r.ClickIDs
	r.Clicks, r.Events, r.Conversions = []Click{}, []Event{}, []Conversion{}
	rows, err := s.ch.Query(ctx, `
		SELECT event_at, type, click_id, campaign_id, country, city, device, os, browser, ip, user_agent, referrer
		FROM click_events WHERE organization_id = ? AND click_id IN ?
		ORDER BY event_at LIMIT ?`, orgID, ids, maxRows)
	if err != nil {
		return fmt.Errorf("visitordata: reading clicks: %w", err)
	}
	for rows.Next() {
		var c Click
		if err := rows.Scan(&c.At, &c.Type, &c.ClickID, &c.CampaignID, &c.Country, &c.City, &c.Device, &c.OS, &c.Browser, &c.IP, &c.UserAgent, &c.Referrer); err != nil {
			rows.Close()
			return fmt.Errorf("visitordata: reading clicks: %w", err)
		}
		r.Clicks = append(r.Clicks, c)
	}
	rows.Close()

	rows, err = s.ch.Query(ctx, `
		SELECT event_at, type, click_id FROM tracking_events
		WHERE organization_id = ? AND click_id IN ? ORDER BY event_at LIMIT ?`, orgID, ids, maxRows)
	if err != nil {
		return fmt.Errorf("visitordata: reading events: %w", err)
	}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.At, &e.Type, &e.ClickID); err != nil {
			rows.Close()
			return fmt.Errorf("visitordata: reading events: %w", err)
		}
		r.Events = append(r.Events, e)
	}
	rows.Close()

	rows, err = s.ch.Query(ctx, `
		SELECT event_at, type, click_id, revenue, currency FROM conversion_events
		WHERE organization_id = ? AND click_id IN ? ORDER BY event_at LIMIT ?`, orgID, ids, maxRows)
	if err != nil {
		return fmt.Errorf("visitordata: reading conversions: %w", err)
	}
	for rows.Next() {
		var c Conversion
		if err := rows.Scan(&c.At, &c.Type, &c.ClickID, &c.Revenue, &c.Currency); err != nil {
			rows.Close()
			return fmt.Errorf("visitordata: reading conversions: %w", err)
		}
		r.Conversions = append(r.Conversions, c)
	}
	rows.Close()

	r.PushSubscriptions = []PushSubscription{}
	prow, err := s.db.Query(ctx, `
		SELECT click_id, created_at, status, country, device, language, endpoint
		FROM push_subscriptions WHERE organization_id = $1 AND click_id = ANY($2) ORDER BY created_at`, orgID, ids)
	if err != nil {
		return fmt.Errorf("visitordata: reading push subscriptions: %w", err)
	}
	for prow.Next() {
		var p PushSubscription
		var endpoint string
		if err := prow.Scan(&p.ClickID, &p.CreatedAt, &p.Status, &p.Country, &p.Device, &p.Language, &endpoint); err != nil {
			prow.Close()
			return fmt.Errorf("visitordata: reading push subscriptions: %w", err)
		}
		if u, err := urlHost(endpoint); err == nil {
			p.EndpointHost = u
		}
		r.PushSubscriptions = append(r.PushSubscriptions, p)
	}
	prow.Close()

	r.TelegramUsers = []TelegramUser{}
	trow, err := s.db.Query(ctx, `
		SELECT click_id, tg_user_id, started_at FROM telegram_users
		WHERE organization_id = $1 AND click_id = ANY($2) ORDER BY started_at`, orgID, ids)
	if err != nil {
		return fmt.Errorf("visitordata: reading telegram users: %w", err)
	}
	for trow.Next() {
		var u TelegramUser
		if err := trow.Scan(&u.ClickID, &u.TgUserID, &u.StartedAt); err != nil {
			trow.Close()
			return fmt.Errorf("visitordata: reading telegram users: %w", err)
		}
		r.TelegramUsers = append(r.TelegramUsers, u)
	}
	trow.Close()

	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM pixel_deliveries WHERE organization_id = $1 AND click_id = ANY($2)`,
		orgID, ids).Scan(&r.PixelDeliveries); err != nil {
		return fmt.Errorf("visitordata: counting pixel deliveries: %w", err)
	}
	return nil
}

// Erase deletes the visitor's data for clickIDs (see the package doc for
// what stays). Scoped to orgID. ClickHouse first, waited for: a failure
// there leaves Postgres untouched, and the request can be repeated.
func (s *Service) Erase(ctx context.Context, orgID string, clickIDs []string) (Erased, error) {
	ids := make([]string, 0, len(clickIDs))
	for _, id := range clickIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || len(ids) > MaxClickIDs {
		return Erased{}, apierror.Validation(fmt.Sprintf("give 1 to %d click ids", MaxClickIDs), map[string]string{"clickIds": "1-500"})
	}
	if s.ch == nil {
		return Erased{}, apierror.Unavailable("ClickHouse is not configured")
	}
	wait := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2, "lightweight_deletes_sync": 2}))
	for _, table := range []string{"click_events", "tracking_events", "click_lookup"} {
		if err := s.ch.Exec(wait, "DELETE FROM "+table+" WHERE organization_id = ? AND click_id IN ?", orgID, ids); err != nil {
			return Erased{}, fmt.Errorf("visitordata: erasing from %s: %w", table, err)
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Erased{}, fmt.Errorf("visitordata: beginning: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var out Erased
	for _, step := range []struct {
		table string
		n     *int64
	}{
		{"push_subscriptions", &out.PushSubscriptions},
		{"telegram_joins", &out.TelegramJoins},
		{"telegram_invite_links", &out.TelegramInvites},
		{"telegram_users", &out.TelegramUsers},
		{"pixel_deliveries", &out.PixelDeliveries},
	} {
		tag, err := tx.Exec(ctx, `DELETE FROM `+step.table+` WHERE organization_id = $1 AND click_id = ANY($2)`, orgID, ids)
		if err != nil {
			return Erased{}, fmt.Errorf("visitordata: erasing from %s: %w", step.table, err)
		}
		*step.n = tag.RowsAffected()
	}
	if err := tx.Commit(ctx); err != nil {
		return Erased{}, fmt.Errorf("visitordata: committing: %w", err)
	}
	return out, nil
}

func urlHost(raw string) (string, error) {
	i := strings.Index(raw, "://")
	if i < 0 {
		return "", fmt.Errorf("no scheme")
	}
	host := raw[i+3:]
	if j := strings.IndexAny(host, "/?#"); j >= 0 {
		host = host[:j]
	}
	return host, nil
}

type Handler struct {
	svc          *Service
	requireWrite func(http.Handler) http.Handler
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// Register mounts /privacy/visitor. Both routes need settings.write: a
// lookup shows visitors' IP addresses. The caller applies
// tenant.Middleware; the audit trail records the erase.
func (h *Handler) Register(r chi.Router) {
	r.With(h.requireWrite).Get("/privacy/visitor", h.find)
	r.With(h.requireWrite).Post("/privacy/visitor/erase", h.erase)
}

func (h *Handler) find(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	rep, err := h.svc.Find(r.Context(), orgID, r.URL.Query().Get("clickId"), r.URL.Query().Get("ip"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, rep)
}

func (h *Handler) erase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClickIDs []string `json:"clickIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	orgID, _ := tenant.OrgID(r.Context())
	out, err := h.svc.Erase(r.Context(), orgID, req.ClickIDs)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, out)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

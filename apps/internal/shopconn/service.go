package shopconn

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/audit"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// OwnSiteTemplate is the template key of an own-site conversion source
// (audit UNI-1): its event mapping has the purchase, repeat_purchase and
// refund statuses the webhooks are recorded as.
const OwnSiteTemplate = "own_site"

// Raw statuses the webhooks are recorded as (the own-site mapping).
const (
	StatusPurchase       = "purchase"
	StatusRepeatPurchase = "repeat_purchase"
	StatusRefund         = "refund"
)

// Platform is what differs between the stores' settings.
type Platform struct {
	// Name is shop_connections.platform and the path segment of both the
	// settings (/networks/{id}/{Name}) and the tracker's webhook URL
	// (/{Name}/{networkId}).
	Name string
	// Title names the platform in messages ("Shopify").
	Title string
	// NormalizeDomain takes what was typed, or what a webhook says it is
	// from, to the form stored and compared.
	NormalizeDomain func(string) string
	// ValidDomain checks a normalized domain; DomainHint is its field error.
	ValidDomain func(string) bool
	DomainHint  string
}

// View is Settings with where to send the webhooks.
type View struct {
	Settings
	// WebhookURL: TRACKER_PUBLIC_URL + /{platform}/{networkId}; "" when the
	// tracker's public URL is not configured.
	WebhookURL string `json:"webhookUrl"`
	// Available false: SHOP_WEBHOOK_ENCRYPTION_KEY is not set, nothing can
	// be connected.
	Available bool `json:"available"`
}

type Service struct {
	store      *Store
	platform   Platform
	trackerURL string
	newID      func() string
}

func NewService(store *Store, platform Platform, trackerURL string) *Service {
	return &Service{store: store, platform: platform, trackerURL: strings.TrimRight(trackerURL, "/"), newID: idgen.New}
}

func (s *Service) view(networkID string, st Settings) View {
	v := View{Settings: st, Available: s.store.key != nil}
	if s.trackerURL != "" {
		v.WebhookURL = s.trackerURL + "/" + s.platform.Name + "/" + networkID
	}
	return v
}

func (s *Service) ownSite(ctx context.Context, orgID, networkID string) error {
	key, found, err := s.store.networkTemplate(ctx, orgID, networkID)
	if err != nil {
		return err
	}
	if !found {
		return apierror.NotFound("conversion source not found")
	}
	if key != OwnSiteTemplate {
		return apierror.Validation("not an own site", map[string]string{
			"networkId": "a " + s.platform.Title + " store connects to an own-site conversion source, whose statuses it reports",
		})
	}
	return nil
}

func (s *Service) Get(ctx context.Context, orgID, networkID string) (View, error) {
	if _, found, err := s.store.networkTemplate(ctx, orgID, networkID); err != nil {
		return View{}, err
	} else if !found {
		return View{}, apierror.NotFound("conversion source not found")
	}
	st, err := s.store.settings(ctx, orgID, networkID, s.platform.Name)
	if err != nil {
		return View{}, err
	}
	return s.view(networkID, st), nil
}

// Input is a write: the store's domain and, to connect or replace it, the
// key the store signs webhooks with ("" keeps the stored one).
type Input struct {
	ShopDomain string `json:"shopDomain"`
	Secret     string `json:"secret"`
}

func (s *Service) Put(ctx context.Context, orgID, networkID string, in Input) (View, error) {
	if err := s.ownSite(ctx, orgID, networkID); err != nil {
		return View{}, err
	}
	current, err := s.store.settings(ctx, orgID, networkID, s.platform.Name)
	if err != nil {
		return View{}, err
	}
	in.ShopDomain = s.platform.NormalizeDomain(in.ShopDomain)
	in.Secret = strings.TrimSpace(in.Secret)
	fields := map[string]string{}
	if !s.platform.ValidDomain(in.ShopDomain) {
		fields["shopDomain"] = s.platform.DomainHint
	}
	switch {
	case in.Secret == "" && !current.Connected:
		fields["secret"] = "the key " + s.platform.Title + " signs the webhooks with is needed to connect the store"
	case len(in.Secret) > 200 || strings.ContainsFunc(in.Secret, unicode.IsSpace):
		fields["secret"] = "the signing key: no spaces, at most 200 characters"
	}
	if len(fields) > 0 {
		return View{}, apierror.Validation("invalid "+s.platform.Title+" settings", fields)
	}
	if err := s.store.put(ctx, s.newID(), orgID, networkID, s.platform.Name, in.ShopDomain, in.Secret); err != nil {
		if errors.Is(err, ErrNoKey) {
			return View{}, apierror.Unavailable(s.platform.Title + " stores cannot be connected here: SHOP_WEBHOOK_ENCRYPTION_KEY is not set")
		}
		return View{}, err
	}
	return s.Get(ctx, orgID, networkID)
}

func (s *Service) Delete(ctx context.Context, orgID, networkID string) error {
	ok, err := s.store.remove(ctx, orgID, networkID, s.platform.Name)
	if err != nil {
		return err
	}
	if !ok {
		return apierror.NotFound("no " + s.platform.Title + " store connected to this conversion source")
	}
	return nil
}

// --- HTTP ---

type Handler struct {
	svc          *Service
	requireWrite func(http.Handler) http.Handler
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// Register mounts GET/PUT/DELETE /{id}/{platform} on the /networks router
// — as plain routes, not a /{id} sub-router, which would take
// /networks/{id} itself from the network handler.
func (h *Handler) Register(r chi.Router) {
	path := "/{id}/" + h.svc.platform.Name
	r.Get(path, h.get)
	r.With(h.requireWrite).Put(path, h.put)
	r.With(h.requireWrite).Delete(path, h.delete)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	v, err := h.svc.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var in Input
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	v, err := h.svc.Put(r.Context(), orgID, chi.URLParam(r, "id"), in)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: h.svc.platform.Name + "_connection.updated", EntityType: "network", EntityID: chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.Delete(r.Context(), orgID, chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: h.svc.platform.Name + "_connection.deleted", EntityType: "network", EntityID: chi.URLParam(r, "id")})
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

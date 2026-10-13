// Package routingstore loads a campaign's routing configuration out of
// Postgres and into the pure, dependency-free types internal/routing
// evaluates.
//
// It exists as its own package specifically so internal/routing can stay
// free of any database driver (§38: "keep routing independent from HTTP
// handlers"; the same reasoning applies to storage). The tracker (through
// internal/routingcache) and the API's /routing/simulate use the same
// loader, so a simulated route and a real one read the same configuration.
package routingstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/routing"
)

var ErrNotFound = errors.New("tracking link not found")

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }

// TrackingLink is the resolved (domain, slug) → campaign mapping.
type TrackingLink struct {
	TrackingLinkID string
	OrganizationID string
	CampaignID     string
	CampaignStatus string
	// TrafficSourceID is the campaign's traffic source, stamped onto every
	// event of the click for the report builder's `source` dimension.
	TrafficSourceID string
}

// ResolveTrackingLink resolves a request's host + slug to a campaign.
//
// Host is part of the lookup, not decoration: tracking_links is unique on
// (domain_id, slug), so a slug alone is deliberately NOT globally unique —
// two organizations may each own the slug "summer" on their own domain.
// Looking up by slug alone would be a cross-tenant data leak.
func (s *Store) ResolveTrackingLink(ctx context.Context, host, slug string) (TrackingLink, error) {
	var link TrackingLink
	err := s.db.QueryRow(ctx, `
		SELECT tl.id, tl.organization_id, tl.campaign_id, c.status, c.traffic_source_id
		FROM tracking_links tl
		JOIN domains   d ON d.id = tl.domain_id
		JOIN campaigns c ON c.id = tl.campaign_id
		WHERE lower(d.domain) = lower($1)
		  AND d.status = 'active' -- verified (tech audit T6); at most one per name
		  AND tl.slug = $2
		  AND tl.status = 'active'`,
		host, slug,
	).Scan(&link.TrackingLinkID, &link.OrganizationID, &link.CampaignID, &link.CampaignStatus, &link.TrafficSourceID)

	if errors.Is(err, pgx.ErrNoRows) {
		return TrackingLink{}, ErrNotFound
	}
	if err != nil {
		return TrackingLink{}, fmt.Errorf("resolving tracking link: %w", err)
	}
	return link, nil
}

// CampaignRouting is a campaign's routing configuration plus the one
// sticky flag that is deliberately absent from routing.RoutingConfig.
//
// stickyFlowKeepClickId governs whether a returning visitor's original
// click_id is reused for attribution — it has no effect whatsoever on
// which flow is selected, which is why internal/routing does not model
// it (documented there). The tracker needs it, so it is returned
// alongside the config rather than smuggled into it.
type CampaignRouting struct {
	Config                routing.RoutingConfig
	StickyFlowKeepClickID bool
	// FlowLandings maps a flow id to its enabled landing stage — only
	// flows whose landing stage is enabled and points at a landing appear.
	// Kept beside Config, not inside routing.Flow, for the same reason as
	// StickyFlowKeepClickID: it changes what the tracker serves after a
	// flow is chosen, never which flow is chosen.
	FlowLandings map[string]FlowLanding
	// FlowPwas maps a flow id to its enabled PWA stage, same contract as
	// FlowLandings.
	FlowPwas map[string]FlowPwa
	// FlowPostlandings maps a flow id to its enabled postlanding stage,
	// same contract as FlowLandings.
	FlowPostlandings map[string]FlowPostlanding
	// OfferNetworks maps each flow offer's id to its network, as the offer
	// row says now: the tracker stamps a click with the network of the
	// offer it was routed to (the `network` report dimension).
	OfferNetworks map[string]string
}

// FlowPostlanding is a flow's postlanding stage: always an external page
// the tracker redirects to (docs/postlanding.md, "Serving visitors").
type FlowPostlanding struct {
	PostlandingID string
	Status        string // postlandings.status
	URL           string
}

// FlowPwa is the per-flow part of a PWA stage the tracker needs to decide
// whether to show it; the page itself is read by LoadPwaPage.
type FlowPwa struct {
	PwaID  string
	Type   string // flows.pwa_type: "internal" | "external" | "ios_app"
	Status string // pwas.status
}

// PwaPage is what the tracker renders a PWA install page and its web app
// manifest from. Deliberately not the whole pwa row: the store-listing
// fields (ratings, review counts, downloads) are never shown to visitors
// — see apps/tracker/pwa.go.
type PwaPage struct {
	Name               string
	ShortName          string
	ThemeColor         string
	BackgroundColor    string
	IconURL            string
	Developer          string
	Description        string
	Screenshots        []string
	BounceInAppWebview bool
	// ThemeAuto: follow the visitor's device light/dark theme.
	ThemeAuto bool
	// ShowHeader: render the top bar (brand mark, title, search).
	ShowHeader bool
	// ShowBottomNav: render the bottom bar with the menu buttons.
	ShowBottomNav bool
	// PushEnabled: the installed app asks for notifications on launch.
	PushEnabled bool
	// HeaderTitle/HeaderLogoURL brand the top bar; empty = default.
	HeaderTitle, HeaderLogoURL string
	Status                     string
	// Listing facts. The visitor page shows only the plain-text ones; the
	// operator-typed numbers (rating, reviews, downloads, size, age) are read
	// for the store-style template, which the tracker renders only on the
	// constructor's signed test links (see apps/tracker/pwa.go).
	WhatsNew            string
	Version             string
	Category            string
	InteractiveElements string
	ContainsAds         bool
	InAppPurchases      bool
	UpdatedDate         string // YYYY-MM-DD, empty when unset

	Rating         float64
	RatingCount    int
	DownloadsLabel string
	SizeMB         float64
	AgeRating      string
	RatingStars5   float64
	RatingStars4   float64
	RatingStars3   float64
	RatingStars2   float64
	RatingStars1   float64
}

// PwaComment is one hand-entered comment of a PWA (pwa_comments).
type PwaComment struct {
	Geo, Lang, Author string
	AvatarURL         string
	Rating, Likes     int
	Date              string // YYYY-MM-DD, empty = none
	Text, Reply       string
}

// LoadPwaComments reads a PWA's comments in entry order, org-scoped. Only
// the constructor's test links render them (see apps/tracker/pwa.go).
func (s *Store) LoadPwaComments(ctx context.Context, orgID, pwaID string) ([]PwaComment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT geo, lang, author, avatar_url, rating, likes, COALESCE(to_char(comment_date, 'YYYY-MM-DD'), ''), body, reply
		FROM pwa_comments
		WHERE organization_id = $1 AND pwa_id = $2
		ORDER BY position`, orgID, pwaID)
	if err != nil {
		return nil, fmt.Errorf("loading pwa comments: %w", err)
	}
	defer rows.Close()
	var out []PwaComment
	for rows.Next() {
		var c PwaComment
		if err := rows.Scan(&c.Geo, &c.Lang, &c.Author, &c.AvatarURL, &c.Rating, &c.Likes, &c.Date, &c.Text, &c.Reply); err != nil {
			return nil, fmt.Errorf("scanning pwa comment: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FlowLanding is the small, per-flow part of a landing the tracker needs
// to decide what to serve. The (possibly large) page source is only read
// by LoadLandingPage, and only when an internal landing is actually being
// served.
type FlowLanding struct {
	LandingID string
	Type      string // "internal" | "external"
	Status    string // "active" | "paused" | "archived"
	URL       string // external landings only
}

// LandingPage is an internal landing's renderable source.
type LandingPage struct {
	Content      string
	BundlePrefix string
}

// LoadRoutingConfig assembles the campaign's full routing configuration:
// stream sets in priority order, each with its recursive AND/OR filter
// tree and its weighted flows.
//
// Every query is scoped by organization_id as well as campaign_id
// (§36-TENANCY: enforcement lives in the repository layer, not the
// caller) — even though campaign_id alone would functionally suffice,
// because a repository that always filters on org cannot leak across
// tenants if a caller is ever wrong about which campaign it holds.
func (s *Store) LoadRoutingConfig(ctx context.Context, orgID, campaignID string) (CampaignRouting, error) {
	cfg := routing.RoutingConfig{CampaignID: campaignID}
	var keepClickID bool
	flowLandings := map[string]FlowLanding{}
	flowPwas := map[string]FlowPwa{}
	flowPostlandings := map[string]FlowPostlanding{}
	offerNetworks := map[string]string{}

	var updatedAt time.Time
	err := s.db.QueryRow(ctx, `
		SELECT fallback_url, sticky_flow, sticky_flow_skip_inactive, sticky_flow_keep_click_id, updated_at
		FROM campaigns
		WHERE id = $1 AND organization_id = $2`,
		campaignID, orgID,
	).Scan(&cfg.FallbackURL, &cfg.StickyFlow, &cfg.StickyFlowSkipInactive, &keepClickID, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CampaignRouting{}, ErrNotFound
	}
	if err != nil {
		return CampaignRouting{}, fmt.Errorf("loading campaign: %w", err)
	}
	newest := updatedAt

	sets, setIDs, setUpdated, err := s.loadStreamSets(ctx, orgID, campaignID)
	if err != nil {
		return CampaignRouting{}, err
	}
	if setUpdated.After(newest) {
		newest = setUpdated
	}

	if len(setIDs) > 0 {
		filters, err := s.loadFilterTrees(ctx, orgID, setIDs)
		if err != nil {
			return CampaignRouting{}, err
		}
		flows, flowUpdated, err := s.loadFlows(ctx, orgID, setIDs, flowLandings, flowPwas, flowPostlandings, offerNetworks)
		if err != nil {
			return CampaignRouting{}, err
		}
		if flowUpdated.After(newest) {
			newest = flowUpdated
		}
		for i := range sets {
			if root, ok := filters[sets[i].ID]; ok {
				sets[i].RootFilter = root
			}
			sets[i].Flows = flows[sets[i].ID]
		}
	}

	cfg.StreamSets = sets
	// §39 calls for versioned configuration. There is no explicit version
	// column; the newest updated_at across the campaign and its routing
	// objects is a real, monotonic version for this campaign's config —
	// it changes exactly when the configuration changes, which is what a
	// cache-invalidation or "which config produced this decision" check
	// needs. Milliseconds, so an edit and a read in the same second still
	// produce different versions.
	cfg.ConfigVersion = newest.UnixMilli()
	return CampaignRouting{Config: cfg, StickyFlowKeepClickID: keepClickID, FlowLandings: flowLandings, FlowPwas: flowPwas, FlowPostlandings: flowPostlandings, OfferNetworks: offerNetworks}, nil
}

func (s *Store) loadStreamSets(ctx context.Context, orgID, campaignID string) ([]routing.StreamSet, []string, time.Time, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, priority, status, fallback_url, updated_at
		FROM stream_sets
		WHERE campaign_id = $1 AND organization_id = $2
		ORDER BY priority`,
		campaignID, orgID,
	)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("loading stream sets: %w", err)
	}
	defer rows.Close()

	var (
		sets    []routing.StreamSet
		ids     []string
		newest  time.Time
		updated time.Time
	)
	for rows.Next() {
		var set routing.StreamSet
		if err := rows.Scan(&set.ID, &set.Name, &set.Priority, &set.Status, &set.FallbackURL, &updated); err != nil {
			return nil, nil, time.Time{}, fmt.Errorf("scanning stream set: %w", err)
		}
		if updated.After(newest) {
			newest = updated
		}
		sets = append(sets, set)
		ids = append(ids, set.ID)
	}
	return sets, ids, newest, rows.Err()
}

type groupRow struct {
	id       string
	setID    string
	parentID *string
	joiner   routing.Joiner
}

// loadFilterTrees rebuilds each stream set's recursive AND/OR tree from
// the flat filter_groups/filter_conditions rows, in two queries total
// rather than one query per node.
func (s *Store) loadFilterTrees(ctx context.Context, orgID string, setIDs []string) (map[string]routing.FilterGroup, error) {
	groupRows, err := s.db.Query(ctx, `
		SELECT id, stream_set_id, parent_group_id, joiner
		FROM filter_groups
		WHERE stream_set_id = ANY($1) AND organization_id = $2
		ORDER BY position, id`,
		setIDs, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("loading filter groups: %w", err)
	}
	defer groupRows.Close()

	var groups []groupRow
	for groupRows.Next() {
		var g groupRow
		if err := groupRows.Scan(&g.id, &g.setID, &g.parentID, &g.joiner); err != nil {
			return nil, fmt.Errorf("scanning filter group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := groupRows.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return map[string]routing.FilterGroup{}, nil
	}

	groupIDs := make([]string, 0, len(groups))
	for _, g := range groups {
		groupIDs = append(groupIDs, g.id)
	}

	condRows, err := s.db.Query(ctx, `
		SELECT filter_group_id, field, operator, value, value_to
		FROM filter_conditions
		WHERE filter_group_id = ANY($1) AND organization_id = $2
		ORDER BY position, id`,
		groupIDs, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("loading filter conditions: %w", err)
	}
	defer condRows.Close()

	condsByGroup := map[string][]routing.FilterCondition{}
	for condRows.Next() {
		var groupID string
		var c routing.FilterCondition
		if err := condRows.Scan(&groupID, &c.Field, &c.Operator, &c.Value, &c.ValueTo); err != nil {
			return nil, fmt.Errorf("scanning filter condition: %w", err)
		}
		condsByGroup[groupID] = append(condsByGroup[groupID], c)
	}
	if err := condRows.Err(); err != nil {
		return nil, err
	}

	childGroups := map[string][]groupRow{}
	roots := map[string]groupRow{}
	for _, g := range groups {
		if g.parentID == nil {
			roots[g.setID] = g
		} else {
			childGroups[*g.parentID] = append(childGroups[*g.parentID], g)
		}
	}

	var build func(g groupRow) routing.FilterGroup
	build = func(g groupRow) routing.FilterGroup {
		node := routing.FilterGroup{Joiner: g.joiner}
		for _, c := range condsByGroup[g.id] {
			node.Children = append(node.Children, c)
		}
		for _, child := range childGroups[g.id] {
			node.Children = append(node.Children, build(child))
		}
		return node
	}

	trees := make(map[string]routing.FilterGroup, len(roots))
	for setID, root := range roots {
		trees[setID] = build(root)
	}
	return trees, nil
}

// loadFlows resolves each flow's destination, including whether an offer
// destination's offer is still active — the check internal/routing needs
// for §58's "inactive offers" case. The offer's URL comes from its first
// offer_link.
//
// It also fills landings with each flow's enabled landing stage, joined in
// the same query — org-scoped on the landing side too, so a landing id
// from another organization can never be served even if one were ever
// written into a flow.
func (s *Store) loadFlows(ctx context.Context, orgID string, setIDs []string, landings map[string]FlowLanding, pwas map[string]FlowPwa, postlandings map[string]FlowPostlanding, offerNetworks map[string]string) (map[string][]routing.Flow, time.Time, error) {
	rows, err := s.db.Query(ctx, `
		SELECT f.stream_set_id, f.id, f.name, f.active, f.weight,
		       f.destination_kind, f.destination_url, f.destination_offer_id,
		       o.status, o.network_id, ol.url, f.updated_at,
		       l.id, l.type, l.status, l.url,
		       p.id, f.pwa_type, p.status,
		       pl.id, pl.status, pl.url
		FROM flows f
		LEFT JOIN offers o ON o.id = f.destination_offer_id AND o.organization_id = f.organization_id
		LEFT JOIN landings l ON f.landing_enabled AND l.id = f.landing_id AND l.organization_id = f.organization_id
		LEFT JOIN pwas p ON f.pwa_enabled AND p.id = f.pwa_id AND p.organization_id = f.organization_id
		LEFT JOIN postlandings pl ON f.postlanding_enabled AND pl.id = f.postlanding_id AND pl.organization_id = f.organization_id
		LEFT JOIN LATERAL (
		    SELECT url FROM offer_links
		    WHERE offer_id = f.destination_offer_id
		    ORDER BY created_at, id
		    LIMIT 1
		) ol ON true
		WHERE f.stream_set_id = ANY($1) AND f.organization_id = $2
		ORDER BY f.position, f.id`,
		setIDs, orgID,
	)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("loading flows: %w", err)
	}
	defer rows.Close()

	flows := map[string][]routing.Flow{}
	var newest time.Time
	for rows.Next() {
		var (
			setID       string
			f           routing.Flow
			kind        string
			destURL     *string
			offerID     *string
			offerStatus *string
			offerNet    *string
			offerURL    *string
			updated     time.Time

			landingID, landingType, landingStatus, landingURL *string
			pwaID, pwaType, pwaStatus                         *string
			postlandingID, postlandingStatus, postlandingURL  *string
		)
		if err := rows.Scan(&setID, &f.ID, &f.Name, &f.Active, &f.Weight, &kind, &destURL, &offerID, &offerStatus, &offerNet, &offerURL, &updated,
			&landingID, &landingType, &landingStatus, &landingURL,
			&pwaID, &pwaType, &pwaStatus,
			&postlandingID, &postlandingStatus, &postlandingURL); err != nil {
			return nil, time.Time{}, fmt.Errorf("scanning flow: %w", err)
		}
		if landingID != nil {
			landings[f.ID] = FlowLanding{
				LandingID: *landingID, Type: deref(landingType), Status: deref(landingStatus), URL: deref(landingURL),
			}
		}
		if updated.After(newest) {
			newest = updated
		}

		switch routing.DestinationKind(kind) {
		case routing.DestinationRedirect:
			f.Destination = routing.Destination{Kind: routing.DestinationRedirect, URL: deref(destURL)}
		case routing.DestinationOffer:
			f.Destination = routing.Destination{
				Kind:        routing.DestinationOffer,
				URL:         deref(offerURL),
				OfferActive: offerStatus != nil && *offerStatus == "active",
				OfferID:     deref(offerID),
			}
			if offerID != nil && offerNet != nil {
				offerNetworks[*offerID] = *offerNet
			}
		}

		if pwaID != nil {
			pwas[f.ID] = FlowPwa{PwaID: *pwaID, Type: deref(pwaType), Status: deref(pwaStatus)}
		}
		if postlandingID != nil {
			postlandings[f.ID] = FlowPostlanding{PostlandingID: *postlandingID, Status: deref(postlandingStatus), URL: deref(postlandingURL)}
		}

		flows[setID] = append(flows[setID], f)
	}
	return flows, newest, rows.Err()
}

// LoadLandingPage reads an internal landing's entry-file source and asset
// prefix — one primary-key lookup, org-scoped, run only when the tracker
// is about to serve that landing.
func (s *Store) LoadLandingPage(ctx context.Context, orgID, landingID string) (LandingPage, error) {
	var p LandingPage
	err := s.db.QueryRow(ctx, `
		SELECT content, bundle_prefix
		FROM landings
		WHERE id = $1 AND organization_id = $2 AND type = 'internal'`,
		landingID, orgID,
	).Scan(&p.Content, &p.BundlePrefix)
	if errors.Is(err, pgx.ErrNoRows) {
		return LandingPage{}, ErrNotFound
	}
	if err != nil {
		return LandingPage{}, fmt.Errorf("loading landing page: %w", err)
	}
	return p, nil
}

// LoadPwaPage reads one PWA's install-page fields — primary-key lookup,
// org-scoped, run only when a PWA page or manifest is being served.
func (s *Store) LoadPwaPage(ctx context.Context, orgID, pwaID string) (PwaPage, error) {
	var p PwaPage
	err := s.db.QueryRow(ctx, `
		SELECT name, short_name, theme_color, background_color, theme_auto, show_header, show_bottom_nav, push_enabled, header_title, header_logo_url, icon_url, developer, description,
		       screenshots, bounce_in_app_webview, status,
		       whats_new, version, category, interactive_elements, contains_ads, in_app_purchases,
		       COALESCE(to_char(updated_date, 'YYYY-MM-DD'), ''),
		       COALESCE(rating, 0)::float8, COALESCE(rating_count, 0), COALESCE(downloads_label, ''),
		       COALESCE(size_mb, 0)::float8, age_rating,
		       COALESCE(rating_stars_5, 0)::float8, COALESCE(rating_stars_4, 0)::float8, COALESCE(rating_stars_3, 0)::float8,
		       COALESCE(rating_stars_2, 0)::float8, COALESCE(rating_stars_1, 0)::float8
		FROM pwas
		WHERE id = $1 AND organization_id = $2`,
		pwaID, orgID,
	).Scan(&p.Name, &p.ShortName, &p.ThemeColor, &p.BackgroundColor, &p.ThemeAuto, &p.ShowHeader, &p.ShowBottomNav, &p.PushEnabled, &p.HeaderTitle, &p.HeaderLogoURL, &p.IconURL, &p.Developer, &p.Description,
		&p.Screenshots, &p.BounceInAppWebview, &p.Status,
		&p.WhatsNew, &p.Version, &p.Category, &p.InteractiveElements, &p.ContainsAds, &p.InAppPurchases, &p.UpdatedDate,
		&p.Rating, &p.RatingCount, &p.DownloadsLabel, &p.SizeMB, &p.AgeRating,
		&p.RatingStars5, &p.RatingStars4, &p.RatingStars3, &p.RatingStars2, &p.RatingStars1)
	if errors.Is(err, pgx.ErrNoRows) {
		return PwaPage{}, ErrNotFound
	}
	if err != nil {
		return PwaPage{}, fmt.Errorf("loading pwa page: %w", err)
	}
	return p, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// LoadPostlanding reads one postlanding's stage fields, for the routes
// that continue a journey from a signed link (landing click-through, PWA
// launch) rather than from a freshly loaded routing config: a postlanding
// paused or edited after the link was issued is seen as it is now.
func (s *Store) LoadPostlanding(ctx context.Context, orgID, postlandingID string) (FlowPostlanding, error) {
	p := FlowPostlanding{PostlandingID: postlandingID}
	err := s.db.QueryRow(ctx, `
		SELECT status, url FROM postlandings
		WHERE id = $1 AND organization_id = $2`,
		postlandingID, orgID,
	).Scan(&p.Status, &p.URL)
	if errors.Is(err, pgx.ErrNoRows) {
		return FlowPostlanding{}, ErrNotFound
	}
	if err != nil {
		return FlowPostlanding{}, fmt.Errorf("loading postlanding: %w", err)
	}
	return p, nil
}

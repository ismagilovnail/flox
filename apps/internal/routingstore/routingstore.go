// Package routingstore loads a campaign's routing configuration out of
// Postgres and into the pure, dependency-free types internal/routing
// evaluates.
//
// It exists as its own package specifically so internal/routing can stay
// free of any database driver (§38: "keep routing independent from HTTP
// handlers"; the same reasoning applies to storage). The tracker uses it
// on the hot path; the API's future /routing/simulate endpoint (Phase 27)
// will use the identical loader, so a simulated route and a real one can
// never be reading differently-shaped configuration.
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
		SELECT tl.id, tl.organization_id, tl.campaign_id, c.status
		FROM tracking_links tl
		JOIN domains   d ON d.id = tl.domain_id
		JOIN campaigns c ON c.id = tl.campaign_id
		WHERE lower(d.domain) = lower($1)
		  AND tl.slug = $2
		  AND tl.status = 'active'`,
		host, slug,
	).Scan(&link.TrackingLinkID, &link.OrganizationID, &link.CampaignID, &link.CampaignStatus)

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
		flows, flowUpdated, err := s.loadFlows(ctx, orgID, setIDs)
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
	return CampaignRouting{Config: cfg, StickyFlowKeepClickID: keepClickID}, nil
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
func (s *Store) loadFlows(ctx context.Context, orgID string, setIDs []string) (map[string][]routing.Flow, time.Time, error) {
	rows, err := s.db.Query(ctx, `
		SELECT f.stream_set_id, f.id, f.name, f.active, f.weight,
		       f.destination_kind, f.destination_url,
		       o.status, ol.url, f.updated_at
		FROM flows f
		LEFT JOIN offers o ON o.id = f.destination_offer_id
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
			offerStatus *string
			offerURL    *string
			updated     time.Time
		)
		if err := rows.Scan(&setID, &f.ID, &f.Name, &f.Active, &f.Weight, &kind, &destURL, &offerStatus, &offerURL, &updated); err != nil {
			return nil, time.Time{}, fmt.Errorf("scanning flow: %w", err)
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
			}
		}

		flows[setID] = append(flows[setID], f)
	}
	return flows, newest, rows.Err()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

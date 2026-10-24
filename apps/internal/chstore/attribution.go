package chstore

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

// ClickResolver implements attribution.ClickResolver against click_events.
//
// Eventual consistency: a click reaches click_events only after
// apps/tracker enqueues it and apps/worker claims and flushes it
// (internal/eventqueue, up to eventPollIdle = 2s behind in the worst
// case). A conversion postback arriving within that window for a
// brand-new click could see OutcomeUnknownClick even though the click
// genuinely happened — inherent to routing attribution through an
// asynchronously-flushed analytics store, not a bug in either resolver or
// store. In practice postback delay (real user time between a click and a
// registration/deposit) dwarfs this window, and a backlog stretches it to
// hours: such a conversion is recorded unknown_click and its click looked
// up again by the worker (internal/reattribute, tech audit T1).
type ClickResolver struct {
	conn driver.Conn
	// lookupBackfill names the backfill that completes click_lookup.
	lookupBackfill string

	// lookupReady is set once click_lookup's backfill is recorded; until
	// then a miss there falls back to scanning click_events, so clicks from
	// before the table existed still attribute. Checked at most every
	// lookupCheckEvery while false.
	lookupReady   atomic.Bool
	lookupChecked atomic.Int64 // unix nanos of the last check
}

const lookupCheckEvery = 30 * time.Second

// NewClickResolver builds a resolver over the same connection EventStore
// writes through.
func NewClickResolver(conn driver.Conn) *ClickResolver {
	return &ClickResolver{conn: conn, lookupBackfill: clickLookupBackfill}
}

var _ attribution.ClickResolver = (*ClickResolver)(nil)

// SourceClickIDExpr is the traffic source's own click id of a click:
// external_click_id when the link carried one, else fbclid, else ttclid.
// Since schema/022 external_click_id holds only the first (audit DATA-7);
// older rows hold the copy, and this reads both alike. click_events'
// bloom filter idx_source_click_id is on exactly this expression, so a
// WHERE on it must spell it the same — notEmpty() included: the analyzer
// turns a comparison with the empty string into notEmpty(), and then no
// longer matches an index spelled with the comparison.
const SourceClickIDExpr = `if(notEmpty(external_click_id), external_click_id, if(notEmpty(fb_click_id), fb_click_id, tt_click_id))`

const clickColumns = `
	click_id, campaign_id, stream_set_id, flow_id, ` + SourceClickIDExpr + `, event_at,
	country, region, city, device, platform, os,
	utm_source, utm_medium, utm_campaign, utm_content, utm_term,
	sub1, sub2, sub3, sub4, sub5, sub6, sub7, sub8, sub9, sub10,
	landing_id, offer_id, pwa_id, traffic_source_id,
	ip, user_agent, fb_click_id, tt_click_id,
	postlanding_id, ad_click_param, ad_click_id`

// Both queries below filter to type = SOURCE_CLICK, excluding
// SOURCE_FILTER rows: a filtered click never reached a destination, so a
// network could never legitimately have received the {click_id} macro for
// it. Treating a filtered click as a resolvable "click" would let a
// postback attribute to traffic FLOX itself decided not to route — this is
// a lookup-eligibility decision, not new attribution policy; the matching
// logic itself (strongest-evidence-first, ambiguity handling) is entirely
// internal/attribution's own, unchanged.

func scanClick(r driver.Rows, organizationID string) (attribution.Click, error) {
	var c attribution.Click
	c.OrganizationID = organizationID
	var occurredAt time.Time
	if err := r.Scan(
		&c.ClickID, &c.CampaignID, &c.StreamSetID, &c.FlowID, &c.ExternalClickID, &occurredAt,
		&c.Country, &c.Region, &c.City, &c.Device, &c.Platform, &c.OS,
		&c.UTMSource, &c.UTMMedium, &c.UTMCampaign, &c.UTMContent, &c.UTMTerm,
		&c.Subs[0], &c.Subs[1], &c.Subs[2], &c.Subs[3], &c.Subs[4],
		&c.Subs[5], &c.Subs[6], &c.Subs[7], &c.Subs[8], &c.Subs[9],
		&c.LandingID, &c.OfferID, &c.PwaID, &c.TrafficSourceID,
		&c.IP, &c.UserAgent, &c.FBClickID, &c.TTClickID,
		&c.PostlandingID, &c.AdClickParam, &c.AdClickID,
	); err != nil {
		return attribution.Click{}, err
	}
	c.OccurredAt = occurredAt
	return c, nil
}

// ByClickID implements attribution.ClickResolver. A click_id can appear on
// more than one row when stickyFlowKeepClickId reuses it across a
// returning visitor's journey (§39-STICKY) — ORDER BY event_at ASC LIMIT 1
// resolves to the ORIGINAL click that started that journey, matching what
// "the click this conversion belongs to" means.
func (r *ClickResolver) ByClickID(ctx context.Context, organizationID, clickID string) (attribution.Click, error) {
	// click_lookup is sorted by (organization_id, click_id): one granule per
	// partition, instead of every granule of the organization in
	// click_events (tech audit CH1).
	click, err := r.queryOne(ctx, `
		SELECT `+clickColumns+`
		FROM click_lookup
		WHERE organization_id = ? AND click_id = ?
		ORDER BY event_at ASC
		LIMIT 1`, organizationID, clickID)
	if !errors.Is(err, attribution.ErrClickNotFound) || r.ready(ctx) {
		return click, err
	}
	return r.queryOne(ctx, `
		SELECT `+clickColumns+`
		FROM click_events
		WHERE organization_id = ? AND click_id = ? AND type = ?
		ORDER BY event_at ASC
		LIMIT 1`, organizationID, clickID, string(event.SourceClick))
}

func (r *ClickResolver) queryOne(ctx context.Context, query string, args ...any) (attribution.Click, error) {
	res, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return attribution.Click{}, fmt.Errorf("chstore: querying click by click_id: %w", err)
	}
	defer res.Close()

	if !res.Next() {
		if err := res.Err(); err != nil {
			return attribution.Click{}, fmt.Errorf("chstore: reading click by click_id: %w", err)
		}
		return attribution.Click{}, attribution.ErrClickNotFound
	}
	click, err := scanClick(res, args[0].(string))
	if err != nil {
		return attribution.Click{}, fmt.Errorf("chstore: scanning click: %w", err)
	}
	return click, nil
}

// ready reports whether click_lookup is complete (its backfill recorded).
// A failed check counts as not ready: the fallback is slower, never wrong.
func (r *ClickResolver) ready(ctx context.Context) bool {
	if r.lookupReady.Load() {
		return true
	}
	now := time.Now().UnixNano()
	if last := r.lookupChecked.Load(); now-last < int64(lookupCheckEvery) && last != 0 {
		return false
	}
	r.lookupChecked.Store(now)
	done, err := backfillDone(ctx, r.conn, r.lookupBackfill)
	if err == nil && done {
		r.lookupReady.Store(true)
	}
	return done
}

const maxExternalClickMatches = 100

// ByExternalClickID implements attribution.ClickResolver.
func (r *ClickResolver) ByExternalClickID(ctx context.Context, organizationID, externalClickID string) ([]attribution.Click, error) {
	res, err := r.conn.Query(ctx, `
		SELECT `+clickColumns+`
		FROM click_events
		WHERE organization_id = ? AND `+SourceClickIDExpr+` = ? AND type = ?
		ORDER BY event_at ASC
		LIMIT ?`,
		organizationID, externalClickID, string(event.SourceClick), maxExternalClickMatches,
	)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying clicks by external_click_id: %w", err)
	}
	defer res.Close()

	var out []attribution.Click
	for res.Next() {
		click, err := scanClick(res, organizationID)
		if err != nil {
			return nil, fmt.Errorf("chstore: scanning click: %w", err)
		}
		out = append(out, click)
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("chstore: reading clicks by external_click_id: %w", err)
	}
	return out, nil
}

package chstore

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

// ClickResolver implements attribution.ClickResolver against click_events
// — the resolver docs/attribution.md always said would "arrive with the
// worker (Phase 24) and the analytical schema (Phase 26)." Both now exist.
//
// Eventual consistency: a click reaches click_events only after
// apps/tracker enqueues it and apps/worker claims and flushes it
// (internal/eventqueue, up to eventPollIdle = 2s behind in the worst
// case). A conversion postback arriving within that window for a
// brand-new click could see OutcomeUnknownClick even though the click
// genuinely happened — inherent to routing attribution through an
// asynchronously-flushed analytics store, not a bug in either resolver or
// store. In practice postback delay (real user time between a click and a
// registration/deposit) dwarfs this window; nothing here compensates for
// it (e.g. a retry-after-delay), and none is added — recorded here rather
// than silently accepted.
type ClickResolver struct {
	conn driver.Conn
}

// NewClickResolver builds a resolver over the same connection EventStore
// writes through.
func NewClickResolver(conn driver.Conn) *ClickResolver {
	return &ClickResolver{conn: conn}
}

var _ attribution.ClickResolver = (*ClickResolver)(nil)

const clickColumns = `
	click_id, campaign_id, stream_set_id, flow_id, external_click_id, event_at,
	country, region, city, device, platform, os,
	utm_source, utm_medium, utm_campaign, utm_content, utm_term,
	sub1, sub2, sub3, sub4, sub5, sub6, sub7, sub8, sub9, sub10`

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
	res, err := r.conn.Query(ctx, `
		SELECT `+clickColumns+`
		FROM click_events
		WHERE organization_id = ? AND click_id = ? AND type = ?
		ORDER BY event_at ASC
		LIMIT 1`,
		organizationID, clickID, string(event.SourceClick),
	)
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
	click, err := scanClick(res, organizationID)
	if err != nil {
		return attribution.Click{}, fmt.Errorf("chstore: scanning click: %w", err)
	}
	return click, nil
}

// maxExternalClickMatches bounds a pathological external_click_id shared by
// an unbounded number of clicks. attribution.go treats "more than one
// match" as ambiguous regardless of the exact count, so capping changes
// nothing about the outcome — only guards against an unbounded scan.
const maxExternalClickMatches = 100

// ByExternalClickID implements attribution.ClickResolver.
func (r *ClickResolver) ByExternalClickID(ctx context.Context, organizationID, externalClickID string) ([]attribution.Click, error) {
	res, err := r.conn.Query(ctx, `
		SELECT `+clickColumns+`
		FROM click_events
		WHERE organization_id = ? AND external_click_id = ? AND type = ?
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

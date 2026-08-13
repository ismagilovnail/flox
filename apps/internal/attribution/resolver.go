package attribution

import (
	"context"
	"errors"
	"sync"
)

// ErrClickNotFound means no click of the requested organization has that id.
var ErrClickNotFound = errors.New("attribution: click not found")

// ClickResolver reads clicks back out of wherever they were persisted.
//
// Every method takes organizationID and MUST filter on it. Tenant isolation is
// enforced here, in the repository layer, rather than by the caller comparing
// ids afterwards (CLAUDE.md #5, §36-TENANCY) — a filter that is part of the
// query cannot be forgotten at one call site, and a resolver that never
// returns another tenant's click makes leaking one impossible upstream.
//
// The real implementation reads ClickHouse and arrives with the worker
// (Phase 24) and the analytical schema (Phase 26); §7 puts raw high-volume
// traffic events there, and §47 forbids serving them from Postgres. Until
// then MemoryResolver below stands in.
type ClickResolver interface {
	// ByClickID returns the single click with this id, or ErrClickNotFound.
	ByClickID(ctx context.Context, organizationID, clickID string) (Click, error)

	// ByExternalClickID returns every click of this organization carrying the
	// network identifier — plural on purpose.
	//
	// Returning a slice rather than one click is what makes §44's "do not
	// invent attribution" enforceable: network click ids are not unique, and
	// an interface that could only return one would have to pick a winner
	// somewhere below this line, out of sight of the rule. No match is an
	// empty slice, not an error — "nobody clicked that" is an ordinary answer.
	ByExternalClickID(ctx context.Context, organizationID, externalClickID string) ([]Click, error)
}

// MemoryResolver is the honest stand-in until clicks are queryable for real.
//
// The tracker currently hands events to eventbuf's LogSink, which is explicitly
// not durable storage, so there is nothing to query yet. Rather than pretend —
// by, say, quietly adding a clicks table to Postgres that §7 says does not
// belong there — this keeps clicks in a map and says so. It is what the
// conformance tests run against, and it is what a local end-to-end walkthrough
// of Phase 23's postback path can be wired to before Phase 24 lands.
//
// It is NOT production storage: per-process, unbounded, and gone on restart.
// Replacing it means implementing ClickResolver against ClickHouse; nothing in
// this package changes.
type MemoryResolver struct {
	mu sync.RWMutex

	// byClickID is keyed by (organization, click id). Scoping the KEY rather
	// than filtering after the lookup means a cross-tenant read cannot be
	// written by accident here either.
	byClickID map[orgKey]Click

	// byExternalID holds every click sharing one network identifier, because
	// duplicates are the case that matters — see ByExternalClickID.
	byExternalID map[orgKey][]Click
}

type orgKey struct {
	organizationID string
	id             string
}

// NewMemoryResolver builds an empty resolver.
func NewMemoryResolver() *MemoryResolver {
	return &MemoryResolver{
		byClickID:    make(map[orgKey]Click),
		byExternalID: make(map[orgKey][]Click),
	}
}

// Record stores a click. Clicks with no ExternalClickID are indexed by click
// id only — an empty network identifier is an absent one, not a value several
// clicks share (§42: missing subs and ids stay empty, never a placeholder).
func (r *MemoryResolver) Record(click Click) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.byClickID[orgKey{click.OrganizationID, click.ClickID}] = click

	if click.ExternalClickID != "" {
		k := orgKey{click.OrganizationID, click.ExternalClickID}
		r.byExternalID[k] = append(r.byExternalID[k], click)
	}
}

// ByClickID implements ClickResolver.
func (r *MemoryResolver) ByClickID(ctx context.Context, organizationID, clickID string) (Click, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	click, ok := r.byClickID[orgKey{organizationID, clickID}]
	if !ok {
		return Click{}, ErrClickNotFound
	}
	return click, nil
}

// ByExternalClickID implements ClickResolver.
func (r *MemoryResolver) ByExternalClickID(ctx context.Context, organizationID, externalClickID string) ([]Click, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	found := r.byExternalID[orgKey{organizationID, externalClickID}]
	if len(found) == 0 {
		return nil, nil
	}

	// Copy: the caller must not be able to mutate the stored slice, and a
	// resolver backed by a database would hand back a fresh slice anyway.
	out := make([]Click, len(found))
	copy(out, found)
	return out, nil
}

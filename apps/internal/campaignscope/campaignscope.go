// Package campaignscope is which campaigns a request may read (audit
// TEAM-1b; docs/team.md): every campaign of the organization, or — for a
// member without campaign.read_all, a Buyer — only the campaigns they own.
//
// apps/api's tenant middleware puts a resolver in every request's context;
// the scope is resolved on first use, once per request, so routes that
// never ask cost nothing. A context without a resolver (background work,
// tests of a single package) reads everything.
package campaignscope

import (
	"context"
	"sync"
)

// Scope is the campaigns a request may read. All: every one of the
// organization's; otherwise exactly IDs (possibly none).
type Scope struct {
	All bool
	IDs []string
	set map[string]bool
}

// Everything is the scope of an unrestricted reader.
var Everything = Scope{All: true}

// Only is the scope of a reader limited to ids.
func Only(ids []string) Scope {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	if ids == nil {
		ids = []string{}
	}
	return Scope{IDs: ids, set: set}
}

// Allows reports whether the campaign may be read. An empty id (a
// conversion with no campaign) is readable only with All.
func (s Scope) Allows(campaignID string) bool {
	return s.All || s.set[campaignID]
}

// Resolver works the scope out for the request's member.
type Resolver func(ctx context.Context) (Scope, error)

type key struct{}

type lazy struct {
	once    sync.Once
	resolve Resolver
	scope   Scope
	err     error
}

// WithResolver makes ctx's scope resolvable by From.
func WithResolver(ctx context.Context, r Resolver) context.Context {
	return context.WithValue(ctx, key{}, &lazy{resolve: r})
}

// With fixes ctx's scope, for tests and callers that already know it.
func With(ctx context.Context, s Scope) context.Context {
	return WithResolver(ctx, func(context.Context) (Scope, error) { return s, nil })
}

// From is ctx's scope: Everything without a resolver.
func From(ctx context.Context) (Scope, error) {
	l, ok := ctx.Value(key{}).(*lazy)
	if !ok {
		return Everything, nil
	}
	l.once.Do(func() { l.scope, l.err = l.resolve(ctx) })
	return l.scope, l.err
}

// IDs is ctx's scope as a query filter: nil for every campaign, else the
// readable ones (possibly none).
func IDs(ctx context.Context) ([]string, error) {
	s, err := From(ctx)
	if err != nil || s.All {
		return nil, err
	}
	return s.IDs, nil
}

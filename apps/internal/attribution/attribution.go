// Package attribution decides which click a conversion belongs to (§44).
//
// This is the join that decides who gets paid. A conversion arrives from a CPA
// network carrying identifiers the network was given at redirect time, and
// attribution's whole job is to turn those identifiers back into the click
// that produced them — or to say, plainly, that it cannot.
//
// The governing rule from §44 is "do not invent attribution when there is
// insufficient evidence", and it is a rule about money, not tidiness. An
// invented link credits a traffic source that did not earn the conversion,
// which means the buyer scales the wrong campaign with real budget. An honest
// "unattributed" is a support ticket; a confident wrong answer is a bad
// decision made repeatedly, and nothing in the reports ever flags it.
//
// Like internal/routing, this package is pure: no HTTP, no database driver, no
// clock of its own. It reads clicks through a ClickResolver, which the caller
// supplies — see resolver.go.
package attribution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// Conversion is the inbound claim: a CPA network telling us that a click it
// received turned into something.
//
// It carries only what attribution needs to do its job. Phase 23 (§45) extends
// this type with the money and deduplication fields — revenue, currency, the
// network's transaction id — rather than redefining it, the same way
// event.Event grows per phase.
type Conversion struct {
	// OrganizationID scopes the lookup. It comes from the credential the
	// postback authenticated with — the API key or the org-scoped postback
	// URL — and NEVER from the request body (CLAUDE.md #5, §36-TENANCY).
	// Attribution refuses to run without it rather than searching globally.
	OrganizationID string

	// ClickID is FLOX's own identifier, handed to the network as a macro at
	// redirect time. The strongest evidence there is.
	ClickID string

	// ExternalClickID is the network's own identifier (fbclid, ttclid, or
	// whatever the partner calls it). Weaker evidence: it is attacker-supplied
	// input on the click side, and it is not reliably unique.
	ExternalClickID string

	// Status is the CPA status being reported. Attribution deliberately does
	// not branch on it — a hold and a redeposit are attributed by exactly the
	// same evidence — but it travels with the conversion because Phase 23's
	// deduplication and progression rules need it.
	Status event.Type

	// OccurredAt is when the network says the conversion happened.
	OccurredAt time.Time
}

// Click is the originating click, as much of it as a conversion needs to
// inherit.
//
// The dimensions are copied onto the conversion rather than joined at query
// time: conversions are a tiny fraction of click volume, and joining every
// report against the click table is the difference between a dashboard that
// answers and one that times out.
type Click struct {
	ClickID         string
	OrganizationID  string
	CampaignID      string
	StreamSetID     string
	FlowID          string
	ExternalClickID string
	OccurredAt      time.Time

	// Classified dimensions, carried forward so a conversion can be sliced by
	// the same facets as the click that earned it.
	Country  string
	Region   string
	City     string
	Device   string
	Platform string
	OS       string
	Browser  string

	// Pass-through parameters (§42). Persisted exactly as they arrived on the
	// click — empty stays empty, never a fabricated placeholder.
	UTMSource   string
	UTMMedium   string
	UTMCampaign string
	UTMContent  string
	UTMTerm     string
	Subs        event.Subs
}

// Method names the evidence a conversion was attributed on. Stored with the
// conversion so a disputed payout can be re-argued from the record.
type Method string

const (
	// MethodNone means nothing was attributed.
	MethodNone Method = ""
	// MethodClickID is FLOX's own click id — a direct, unambiguous match.
	MethodClickID Method = "click_id"
	// MethodExternalClickID is the network's identifier, used only when it
	// matched exactly one click.
	MethodExternalClickID Method = "external_click_id"
)

// Outcome is why attribution ended the way it did. The set is closed and small
// so it can be a low-cardinality column and a dashboard filter: "show me
// today's unattributed postbacks, grouped by reason" is the question an
// operator actually asks.
type Outcome string

const (
	// OutcomeAttributed is a conversion joined to exactly one click.
	OutcomeAttributed Outcome = "attributed"

	// OutcomeNoIdentifier means the postback carried neither a click_id nor an
	// external_click_id. There is nothing to match on at all — usually a
	// misconfigured postback template in the network's dashboard, which is
	// worth surfacing as its own reason rather than as a generic miss.
	OutcomeNoIdentifier Outcome = "no_identifier"

	// OutcomeUnknownClick means the identifiers were present but matched no
	// click of this organization.
	//
	// A click that exists under a DIFFERENT organization also lands here, and
	// that is deliberate: reporting "belongs to another tenant" would confirm
	// the existence of another tenant's click id to anyone who can guess one.
	// The two cases are indistinguishable from outside on purpose.
	OutcomeUnknownClick Outcome = "unknown_click"

	// OutcomeAmbiguous means an external_click_id matched more than one click,
	// so there is no way to tell which one earned the conversion.
	//
	// This is not a rare edge case. Network click ids are not unique in
	// practice — the same fbclid reappears across a redirect chain, a prefetch
	// and a genuine second visit — so picking "the most recent" would look
	// reasonable and quietly credit the wrong click a fraction of the time.
	// §44 forbids exactly that.
	OutcomeAmbiguous Outcome = "ambiguous_external_click_id"
)

// Attributed reports whether this outcome produced a usable link.
func (o Outcome) Attributed() bool { return o == OutcomeAttributed }

// Attribution is the decision.
//
// An unattributed result is a first-class answer, not an error: the postback
// was received, understood and recorded, and the conversion simply has no
// click to hang on. Phase 23 stores it either way — an unattributed conversion
// that is invisible is indistinguishable from one that never arrived, and
// those need very different responses.
type Attribution struct {
	Outcome Outcome
	Method  Method

	// Click is populated only when Outcome is OutcomeAttributed.
	Click Click

	// TimeToConversion is OccurredAt minus the click's own timestamp.
	//
	// It is the number that separates a real traffic source from a cheating
	// one: conversions landing seconds after the click, at scale, are not
	// people. A NEGATIVE value means the conversion predates its click, which
	// is clock skew between us and the network, or a replayed postback — it is
	// reported rather than suppressed, because silently clamping it to zero
	// would hide the anomaly it exists to reveal.
	TimeToConversion time.Duration

	// Reason is a human-readable summary for the postback log, in the same
	// spirit as routing's explainability (§72): every decision has to be
	// answerable after the fact, especially the ones that cost someone money.
	Reason string
}

// ErrNoOrganization means the caller did not scope the lookup.
//
// This is refused rather than defaulted, because the only alternative — a
// search across all organizations — is a cross-tenant data leak that would
// look like a working feature (CLAUDE.md #5).
var ErrNoOrganization = errors.New("attribution: conversion has no organization id")

// AttributionService is §44's exact interface.
type AttributionService interface {
	AttributeConversion(ctx context.Context, conversion Conversion) (Attribution, error)
}

// Service is the concrete AttributionService.
type Service struct {
	clicks ClickResolver
}

var _ AttributionService = (*Service)(nil)

// NewService builds the service over a click source.
func NewService(clicks ClickResolver) *Service {
	return &Service{clicks: clicks}
}

// AttributeConversion resolves a conversion to its click.
//
// Evidence is tried strongest-first and the search stops at the first
// identifier that produces a definite answer:
//
//  1. click_id — our own identifier, handed to the network by us. A match here
//     is not a guess.
//  2. external_click_id — the network's identifier, and only when it matches
//     exactly one click. Several matches is ambiguity, and ambiguity is not
//     resolved by preferring one; it is reported.
//
// A present-but-unmatched click_id does NOT fall through to the external id.
// If the network echoed back an identifier we minted and we cannot find it,
// something is wrong with that claim, and quietly re-matching it on a weaker,
// attacker-supplied field would paper over precisely the case worth seeing.
func (s *Service) AttributeConversion(ctx context.Context, conversion Conversion) (Attribution, error) {
	orgID := strings.TrimSpace(conversion.OrganizationID)
	if orgID == "" {
		return Attribution{}, ErrNoOrganization
	}

	clickID := strings.TrimSpace(conversion.ClickID)
	externalID := strings.TrimSpace(conversion.ExternalClickID)

	switch {
	case clickID != "":
		return s.byClickID(ctx, orgID, clickID, conversion)
	case externalID != "":
		return s.byExternalClickID(ctx, orgID, externalID, conversion)
	default:
		return Attribution{
			Outcome: OutcomeNoIdentifier,
			Method:  MethodNone,
			Reason:  "postback carried neither click_id nor external_click_id; nothing to match on",
		}, nil
	}
}

func (s *Service) byClickID(ctx context.Context, orgID, clickID string, conversion Conversion) (Attribution, error) {
	click, err := s.clicks.ByClickID(ctx, orgID, clickID)
	switch {
	case errors.Is(err, ErrClickNotFound):
		return Attribution{
			Outcome: OutcomeUnknownClick,
			Method:  MethodNone,
			Reason:  fmt.Sprintf("click_id %q matched no click of this organization", clickID),
		}, nil
	case err != nil:
		// A resolver failure is NOT "unattributed". Recording it as such would
		// permanently write off a conversion because a database blinked; the
		// caller must retry instead.
		return Attribution{}, fmt.Errorf("attribution: resolve click_id: %w", err)
	}

	return attributed(click, conversion, MethodClickID,
		fmt.Sprintf("matched click_id %q", clickID)), nil
}

func (s *Service) byExternalClickID(ctx context.Context, orgID, externalID string, conversion Conversion) (Attribution, error) {
	clicks, err := s.clicks.ByExternalClickID(ctx, orgID, externalID)
	if err != nil {
		return Attribution{}, fmt.Errorf("attribution: resolve external_click_id: %w", err)
	}

	switch len(clicks) {
	case 0:
		return Attribution{
			Outcome: OutcomeUnknownClick,
			Method:  MethodNone,
			Reason:  fmt.Sprintf("external_click_id %q matched no click of this organization", externalID),
		}, nil
	case 1:
		return attributed(clicks[0], conversion, MethodExternalClickID,
			fmt.Sprintf("matched external_click_id %q (unique)", externalID)), nil
	default:
		return Attribution{
			Outcome: OutcomeAmbiguous,
			Method:  MethodNone,
			Reason: fmt.Sprintf("external_click_id %q matched %d clicks; refusing to guess which one earned this conversion",
				externalID, len(clicks)),
		}, nil
	}
}

func attributed(click Click, conversion Conversion, method Method, reason string) Attribution {
	return Attribution{
		Outcome:          OutcomeAttributed,
		Method:           method,
		Click:            click,
		TimeToConversion: conversion.OccurredAt.Sub(click.OccurredAt),
		Reason:           reason,
	}
}

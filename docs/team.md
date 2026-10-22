# Buyers and their campaigns (TEAM-1)

Audit 2026-10-15, item TEAM-1. Part 1 covers campaign ownership, writes
limited to one's own campaigns, and the buyer dimension in reports. Part
2 (TEAM-1b) limits what a Buyer *reads* to their own campaigns.

## Ownership

`campaigns.owner_user_id` (migration 00097) holds the member a campaign
belongs to:

- **Creating** a campaign makes the creator its owner. A campaign created with
  an API key has no owner.
- **Duplicating** a campaign makes the member who duplicated it the owner of
  the copy.
- **Existing campaigns** got their creator from the audit trail
  (`POST /campaigns`) when that member is still active; the rest stay
  unowned. Dev fixtures give the demo campaign to `buyer@flox.local`.
- **Reassigning** uses `PUT /v1/campaigns/{id}/owner {"userId"}`. It needs
  `campaign.manage_all`, and the target must be an active member of the same
  organization. `""` unassigns the campaign.
- **Deleting a user** unassigns their campaigns (`ON DELETE SET NULL`).

The app shows the owner under the campaign's name. Members with
`campaign.manage_all` get a picker there. The campaign list has a Buyer
column and a filter: all, mine, no buyer, or a buyer.

## Who changes what

A new permission, `campaign.manage_all`, lets a member change and reassign
every campaign. Owner, Admin and Manager have it. A Buyer does not, so for a
Buyer `campaign.write` covers creating campaigns plus changing the campaigns
they own.

The following writes go through `campaign.write` and then
`campaign.Access.RequireOwned`:

- updating, deleting, duplicating, pausing or activating a campaign;
- creating, changing or reordering its stream sets;
- its tracking links;
- its spend entries and CSV import.

The check passes in three cases:

- the request uses an API key (keys act for the whole organization; only
  Owners and Admins make them);
- the member has `campaign.manage_all`;
- the member owns the campaign.

Since TEAM-1b, a campaign outside the member's read scope answers 404, the
same as on reads. For a Buyer that means another buyer's campaign and an
unowned one. `403 "this campaign belongs to another buyer"` is left for a
campaign the member can see but does not own. An unknown campaign falls
through to the handler's 404.

`TestBuyersChangeOnlyTheirCampaigns` walks the router. Every mutating route
under `/campaigns/{id}` or `/campaigns/{campaignId}` must answer another
buyer 404 (403 before TEAM-1b scoped reads), unless it is listed in
`notOwnerGated` with a reason. A route
added later is therefore covered automatically.

In the app, `useCanEditCampaign` hides the same controls. That is for UX
only; the API is the boundary.

## Report by buyer

`buyer` is a report dimension and filter, available in the report builder,
timeseries, breakdown and funnel. Its value is the owner's user id, with
`""` for an unowned campaign.

- **History follows the current owner.** Events carry no buyer, so a campaign's
  whole history counts for whoever owns it now. The ClickHouse schema does
  not change.
- **Grouping by buyer:** `internal/analytics` queries by campaign and adds the
  per-campaign accumulators up per owner before the derived metrics are
  computed. Unique clicks add up because they are counted per campaign.
  - Counts, revenue and spend are summed.
  - CR, ROI, EPC and the rest come from those sums, never from averaging the
    campaigns' values.
  - A buyer whose campaigns have no spend shows cost "—" (CLAUDE.md #6), the
    same rule the campaign-less total uses.
- **A filter `buyer = X`** becomes `campaign IN (X's campaigns)`, using
  `chstore.ReportFilter.In`; an empty set matches nothing. Push statistics
  and spend are narrowed to the same campaigns. By source or by country, the
  narrowed spend is unknown, so it is shown as "—".
- **Spend is narrowed by campaign filters:** a report filtered to some
  campaigns, with no dimension or by day, now counts only their spend.
  Before, the total counted every campaign.
- **Labels:** `GET /v1/analytics/buyers` (`analytics.read`) names the
  members who own campaigns. The team list needs `team.read`, which Buyers,
  Analysts and Viewers lack.

## Reading: a Buyer sees only their campaigns (TEAM-1b)

A second permission, `campaign.read_all` (migration 00098), lets a member read
every campaign and its data. Owner, Admin, Manager, Analyst and Viewer have
it; a Buyer does not. Without it, `campaign.read` covers only the campaigns
the member owns. Unowned campaigns are not theirs, and neither are
conversions or postbacks with no campaign. API keys read the whole
organization.

**How it works:** `internal/campaignscope` holds the request's scope.
apps/api's tenant middleware puts a resolver in every request's context, and
the scope is worked out on first use, once per request
(`campaign.Access.ScopeResolver`: a permission check, plus the owned ids for
a Buyer). A route that never asks costs nothing. Background work and
packages tested alone read everything.

| Read | How it is limited |
|---|---|
| `GET /campaigns`, `GET /campaigns/{id}` | the list holds only the scope; another campaign is 404 |
| `/campaigns/{campaignId}/…`: tracking links, stream sets, spend, simulator | `RequireVisible("campaignId")` on the group: 404 |
| `/tracking-links`, `/flows` | filtered to the scope |
| report, timeseries, breakdown, funnel, Overview | `campaign IN scope` added to the filters (`analytics.scopeFilters`); spend follows it |
| dimension values, flow summary | the ClickHouse query takes the campaign set: another buyer's flow reads as zeros |
| `analytics/campaigns/{id}/daily…` | 404 outside the scope |
| buyer values, `/analytics/buyers` | only owners of the scope's campaigns: a Buyer does not learn who the other buyers are |
| LTV cohorts | `LTVFilter.Campaigns` |
| `/conversions`, `/click-log` | `campaign_id IN` in list and count |
| `/postback-logs` | through the click: `click_id IN (click_lookup of the scope)`; a postback for an unknown click is hidden |
| `/conversions/{clickId}` | another buyer's click is 404, like an unknown one |
| `/postback-logs/check` | another buyer's click reads as `unknown_click`, without its outgoing URL |

The postback replays need `offer.write`, which a Buyer lacks.

**Not limited:** the team's shared assets stay readable — offers, networks,
traffic sources, landings, PWAs, domains, the content gallery, ad-account
spend settings, onboarding and the plan. Push (subscribers, broadcasts, stats
by campaign) and alerts and notifications stay team-wide as well, by
decision; a later item can scope them.

**The test.** `TestBuyersReadOnlyTheirCampaigns`: Ann owns a campaign with
a click, a conversion and a postback; Bob owns nothing.

- **Bob reads nothing of Ann's:** none of his answers to the reads above names
  Ann's campaign or click, and a read naming Ann's campaign or click in its
  path answers 404.
- **The check is not vacuous:** Ann's own answers do name them.
- **A Viewer still sees everything.**
- **No read under the campaign-data prefixes is forgotten:** each route there
  is either listed or explained, so a new one fails until decided.
- **Nothing leaks elsewhere:** every other parameterless read is swept for
  Ann's ids, except what the team shares on purpose.

With scoping turned off, 17 of the reads leaked and the test failed.

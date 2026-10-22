# Buyers and their campaigns (TEAM-1)

Audit 2026-10-15, item TEAM-1. Part 1 (this document): campaign
ownership, writes limited to one's own campaigns, and the buyer
dimension in reports. Part 2 (TEAM-1b) will limit what a buyer can *read*
to their own campaigns.

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

Otherwise the answer is `403 "this campaign belongs to another buyer"`, and
that includes an unowned campaign. An unknown campaign falls through to the
handler's 404.

`TestBuyersChangeOnlyTheirCampaigns` walks the router. Every mutating route
under `/campaigns/{id}` or `/campaigns/{campaignId}` must answer 403 to
another buyer, unless it is listed in `notOwnerGated` with a reason. A route
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

## Not yet (TEAM-1b)

A Buyer still reads every campaign and every report of the organization.
TEAM-1b scopes the reads: the campaign list, reports (through the same
`ReportFilter.In`), conversions, the click log, postback logs, LTV and the
simulator. It will also add a route-walk test like the tenant-isolation
one.

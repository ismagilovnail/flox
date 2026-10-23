# Competitors' prices (BIZ-4, part 1, 2026-10-23)

Audit 2026-10-15, item BIZ-4: compare FLOX's plans with competitors' official
prices. This is the part that needs no account: each vendor's public pricing
page, read on 2026-10-23. The other part is a test account on Adset.Pro to
compare onboarding, PWAs and reports by hand. It is the owner's, and still
open.

Prices change often. Re-read the pages before deciding on anything here.

## What each vendor counts

- **FLOX** counts **clicks**: every visit the tracker accepted, `SOURCE_CLICK`
  plus `SOURCE_FILTER` (docs/billing.md). Limits are soft. Over the limit,
  tracking goes on and new campaigns wait. There is no overage price.
- **BeMob, RedTrack, Adset.Pro** count **events**: clicks, conversions and,
  for BeMob, impressions and visits. Click-heavy traffic makes the two
  comparable. Every conversion and impression adds to theirs.
- **Keitaro, Binom (v2)** sell a self-hosted licence with no click limit. The
  client pays for their own server on top.

## Plans

Prices are as the pages show them, in "$" with no currency named. "Annual" is
the per-month price when billed yearly.

| Vendor, plan | Per month | Annual | Included per month | Over the limit | Raw data kept | Users |
|---|---:|---:|---|---|---|---|
| **FLOX Free** | 0 | — | 0.1M clicks | soft limit | 3 months | 2 |
| **FLOX Start** | $49 · 4,900 ₽ | no discount | 1M clicks | soft limit | 6 months | 5 |
| **FLOX Pro** | $149 · 14,900 ₽ | no discount | 5M clicks | soft limit | 12 months | 15 |
| **FLOX Business** | $399 · 39,900 ₽ | no discount | 20M clicks | soft limit | 24 months | ∞ |
| BeMob Free | 0 | — | 0.1M events | — | 1 month | — |
| BeMob Professional | $49 | not offered | 1M events | $0.05 / 1K | 1 month | 1 |
| BeMob Business | $249 | not offered | 10M events | $0.025 / 1K | 3 months | 2 + 1 |
| BeMob Enterprise | $499 | not offered | 30M events | $0.02 / 1K | 6 months | 3 + 1 |
| RedTrack Solo | $169 | $118 | 5M events | $0.04 / 1K | 12 months | 1 |
| RedTrack Team | $399 | $279 | 20M events | $0.035 / 1K | 24 months | 5 (+$29 each) |
| RedTrack Enterprise | $999 | $699 | 75M events | $0.025 / 1K | 36 months | 15 (+$29 each) |
| Adset.Pro Starter | 0 | — | pay as you go | $0.10 / 1K events | not stated | 3 |
| Adset.Pro Pro | $99 | — | "15,000" events, 3,000 PWA installs | $0.07 / 1K, $0.04 / install | not stated | 3 |
| Adset.Pro Business | $399 | — | "25,000" events, 15,000 installs | $0.06 / 1K, $0.04 / install | not stated | 15 |
| Adset.Pro Enterprise | $999 | — | "100,000" events, 50,000 installs | $0.05 / 1K, $0.03 / install | not stated | 30 |
| Keitaro Starter … Enterprise (self-hosted) | 40 / 72 / 104 / 200 / 400 | 10% off for 6 months, 20% off for 12 | unlimited clicks | — | your server | 1 / 1 / 5 / 50 / ∞ |
| Binom v2 (self-hosted) | $149 | $104 | unlimited | — | "lifetime" | ∞ |
| Binom Cloud | $299 | — | up to 2M per day | — | "lifetime" | ∞ |

### Unclear on the pages

- **Adset.Pro's event counts** (15,000 / 25,000 / 100,000 a month) look far
  too low for $99–999 next to thousands of PWA installs. They may be
  thousands, or the page text may have lost a unit. Check this with the test
  account. Adset.Pro's "Tracker only" prices did not come through: the page
  shows them behind a toggle.
- **Keitaro** shows bare numbers with "/month" and a "Yearly" label on two
  cards. The page does not say whether these are monthly prices or monthly
  equivalents of a yearly licence, nor the currency. There is no trial.
- **Binom v2** shows $149 and $104 under a "Save 30%" toggle; the lower one is
  most likely yearly.
- **RedTrack** showed a Black Friday banner (30% off yearly). It is the same
  30% as its usual annual discount, so the annual prices may or may not be a
  promotion.

## FLOX next to them

At each FLOX plan's volume:

- **1M clicks.**
  - FLOX Start: $49, kept 6 months.
  - BeMob Professional: $49 for 1M events, kept 1 month.
  - RedTrack has nothing below $118–169.
  - Keitaro: about $40–104 plus a server.
- **5M clicks.**
  - FLOX Pro: $149, 12 months.
  - RedTrack Solo: $169, or $118 billed yearly, 5M events, 12 months.
  - BeMob: $249 for 10M, 3 months.
- **20M clicks.**
  - FLOX Business: $399, 24 months.
  - RedTrack Team: the same $399, 20M events, 24 months, or $279 billed yearly.
  - BeMob Enterprise: $499 for 30M, 6 months.
  - Adset.Pro Business: also $399, with PWA installs counted separately.

So FLOX's draft prices sit at the market:

- **Per click:** at RedTrack's monthly price, BeMob's for small volume.
- **History:** longer than BeMob's, the same as RedTrack's.
- **Included at no extra charge:** PWA, push and Telegram, which RedTrack and
  BeMob do not have and which Adset.Pro meters by install.

## What the comparison shows

For the owner to decide. Nothing here is built.

1. **Annual discount.** RedTrack and Binom take 30% off for a year, and
   Keitaro 20%. FLOX's 1/3/12-month prepayment (BIZ-1) has no discount, so
   FLOX Business costs $120 a month more than RedTrack Team billed yearly.
2. **Overage instead of only a soft limit.** Every metered competitor sells
   the clicks above the plan:
   - BeMob $20–50 per million;
   - RedTrack $25–40;
   - Adset.Pro $50–100.

   FLOX's variable cost of a million clicks is about $1–3 a month at the
   placeholder hosting prices (docs/unit-economics.md). So overage would earn
   well, and would replace "new campaigns wait at 110%" with something
   clients already know.
3. **PWA installs are not metered in FLOX.** Adset.Pro charges $0.03–0.04 an
   install over its quota. If PWA serving costs FLOX something real (it is not
   measured: landings and PWAs were outside BIZ-2's costbench), a quota or
   price per install is the market's way.
4. **Self-hosted competitors** (Keitaro, Binom v2) win at high volume for a
   client who runs servers. FLOX is cloud only, so its case is everything in
   one place, with nothing to host.
5. **Free plan.** BeMob gives 0.1M events for a month of history. FLOX's Free
   gives the same clicks with 3 months of history.

## Sources (read 2026-10-23)

- https://bemob.com/pricing
- https://www.redtrack.io/pricing/
- https://adset.pro/pricing and https://adset.pro/
- https://keitaro.io/en/pricing, https://keitaro.io/en/pricing-business,
  https://docs.keitaro.io/en/faq.html, https://docs.keitaro.io/en/site/edition-plans.html
- https://binom.org/price

# System gallery content

The read-only catalog behind the Content Gallery's "System" section
(`../system.go`, `docs/content-gallery.md`). Every item is listed in
`catalog.json`; the catalog is checked at start-up and by
`TestSystemCatalog`.

- `creatives/` — banners for ads and feeds.
- `landings/<name>/` — landing bundles: `index.html` at the root, links
  as `{{ trackUrl }}` (see `docs/landings.md`). Quiz answers are buttons,
  because every `<a>` on a landing is sent to the offer.
- `pwa/<name>/` — PWA store-page icon and screenshots; the texts are in
  `catalog.json`.

All of it is generic sample content with invented names — no real app,
brand or store artwork. The PNGs come from `../gen/make_assets.py` (PIL,
DejaVu fonts); edit or replace them freely, then keep `catalog.json` in
step. Everything in this folder is embedded in the API binary — keep it
small.

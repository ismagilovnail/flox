# CLAUDE.md — FLOX

> This file is read at the start of every session. Keep it lean.
> The full spec is `FLOX-master-prompt-v3.md` — read it before implementing any
> phase. This file is the operating manual + progress tracker, not the spec.

---

## PROJECT

**FLOX** — production-grade SaaS traffic tracking & routing platform.
Tagline: **Track. Route. Optimize.**

Full requirements: **`FLOX-master-prompt-v3.md`** (authoritative; sections
referenced below as §N).

---

## CURRENT STATE — UPDATE THIS EVERY PHASE

```
CURRENT PHASE : CONTENT GALLERY — REAL MEDIA LIBRARY, IMAGES ONLY, + PWA PICKER/CROP WIRING (user-scoped, post-Phase-34)
STATUS        : done — user pointed at a reference tracker's real
                Content Gallery page (adset.pro/guide_ru/galereya-
                kontenta) and asked for the same. FLOX's own
                /content-gallery was a frontend-only mock of something
                else entirely (a "template gallery" of pre-built
                landing/PWA/postlanding starter content — see the
                deleted lib/mock/content-gallery.ts's own former doc
                comment); no object storage was wired up anywhere in
                the repo (config.S3Config existed since Phase 16,
                unused). Scoped with the user first — two questions:
                (a) replace the mock outright vs. add a separate new
                page, user chose replace; (b) scope of the first pass
                given video needs real transcoding infra, user chose
                images only, video explicitly deferred.
                Backend: migration 00027 adds content_gallery_folders
                (flat, no nesting — matches the reference) and
                content_gallery_files (folder_id ON DELETE SET NULL —
                deleting a folder moves files to root, never deletes
                them; status active/archived — soft-delete/restore,
                no hard delete — both rules matching the reference
                exactly). New apps/internal/storage (minio-go client;
                the dev/test docker-compose stacks' own minio
                containers already existed, nothing in Go code had
                ever connected to them) and apps/internal/
                contentgallery (handler/service/repository — multipart
                upload sniffed via http.DetectContentType, the
                client's own Content-Type header is never trusted).
                Bulk move/archive/restore capped at 50 ids, folder
                names at 50 chars, upload at 20MB — all three matching
                the reference's own stated limits.
                One real design correction mid-build: file URLs were
                first generated as presigned, hour-expiring S3 URLs —
                right for browsing the gallery itself, wrong the
                moment a picked file is saved as e.g. a PWA's iconUrl,
                which has to keep resolving for as long as that PWA
                stays installed. Switched to a public-read bucket
                policy (storage.Client.PublicURL, no signing, no
                expiry) — Content Gallery files are meant to end up
                embedded in things end users' browsers fetch
                unauthenticated anyway, same as the old mock's own
                cdn.floxlink.io URLs implied. Tenant scoping stays
                entirely in Postgres (content_gallery_files.
                organization_id), not the storage layer.
                A dev/test-only wrinkle the fix surfaced: apps/api/
                apps/worker reach minio at the in-network
                http://minio:9000, a browser can only reach the host-
                published http://localhost:9000 (:9002 in the test
                stack). New S3_PUBLIC_ENDPOINT config (defaults to
                S3_ENDPOINT) fixes this, set explicitly in both
                docker-compose files — a no-op in a real deployment
                where the S3-compatible endpoint is equally reachable
                from both sides.
                Frontend: features/content-gallery/* rebuilt around a
                shared GalleryBrowser (folder panel, upload cell,
                selection + bulk actions, Active/Inactive toggle, an
                "uploaded by" filter gated to Owner/Admin, pagination
                past 50 files) used by both the full /content-gallery
                page and a new reusable GalleryFilePickerDialog — first
                wired into pwa-form.tsx's icon field (a "browse
                gallery" button beside the existing URL input, which
                stays, for pasting an externally-hosted URL). lib/mock/
                content-gallery.ts + stores/content-gallery.ts deleted
                outright, along with the old template "Use this" hand-
                off (?gallery=<id> query param) in new-pwa-view.tsx/
                new-landing-view.tsx/new-postlanding-view.tsx — that
                concept doesn't exist in the replacement feature.
                NOT built, deliberately: video upload/transcoding; the
                reference's crop tool and dimension-compatibility
                indicators (green/yellow/red) — no PWA field states a
                required icon dimension to check against, and inventing
                one would be a fabricated affordance (CLAUDE.md #6's own
                reasoning, applied to a UI signal rather than a number).
                Verified: go build/vet/test -count=1 ./... clean,
                including 7 new integration tests against real Postgres
                + the real dev minio container (folder create/rename/
                delete-moves-files-to-root, upload/list/filter/
                paginate/archive/restore/move, upload validation —
                video rejected, oversize rejected, sniffed-content-
                mismatch rejected, bulk-op 50-id cap, cross-org
                isolation). Real end-to-end curl pass against the live
                containerized dev stack: signed up a real org, uploaded
                a real PNG, confirmed the returned public URL — fetched
                with zero cookies/auth, a fresh curl process — serves
                the exact same bytes back. tsc/eslint/vitest clean.
                Real browser pass (Claude-in-Chrome) against the actual
                reporting user's own live account: created a folder,
                uploaded a real file, selected it, archived it via the
                bulk bar, confirmed it moved to the Inactive tab,
                deleted the test folder (confirmed via its own dialog
                copy that files move to Unfiled rather than being
                deleted) — no console errors. Test artifacts cleaned up
                (the folder deleted, the one tiny test upload left
                archived); the account's own pre-existing real gallery
                content (uploaded independently by the user mid-
                session, confirming the feature was already in real
                use) was left untouched.
                Round 2, same phase: user asked for permanent deletion
                from the Inactive list too (the first pass deliberately
                stopped at archive/restore, matching the reference).
                Added Service.DeleteFiles/Repository.DeleteFiles, gated
                to already-archived files by the SQL WHERE clause
                itself (not just a service-layer check) — no code path
                can hard-delete an active file directly. New POST
                /content-gallery/files/delete; best-effort storage
                cleanup after the DB rows are gone. Frontend: a "Delete
                permanently" action in the bulk bar and each file's own
                menu, visible only on Inactive, behind its own confirm
                dialog (distinct copy from the folder-delete confirm —
                this one explicitly says it can't be undone). New
                integration test (an active-file id alongside an
                archived one is silently skipped, not deleted).
                Exercised for real, live, by the actual reporting user
                testing the feature in their own browser session while
                this was being built — confirmed via the API access
                log, not just this phase's own verification.
                Round 3, same phase: user asked for the PWA icon and
                screenshots fields to go through the Content Gallery
                (upload or pick an existing file) with crop support —
                both were open items on this phase's own NEXT list.
                New image-crop-dialog.tsx (react-easy-crop, new
                dependency): every image GalleryUploadCell accepts
                (jpeg/png/webp — GIF skips the dialog, canvas can't
                crop an animation without flattening it) opens a crop
                step first — reposition/zoom, three aspect presets
                (1:1/4:3/9:16), "Skip crop" uploads the original as-is.
                No fixed target dimension is enforced anywhere (no
                field declares one; inventing one would be fabricated).
                Two real bugs found and fixed via an actual browser
                session, not review: (1) the upload cell was gated to
                mode==="manage", so GalleryFilePickerDialog — used
                everywhere a form field picks an image — could only
                ever select an existing file, never upload a new one,
                directly contradicting this package's own doc-comment
                promise ("upload directly within file selection
                dialogs"); fixed by keying the upload cell (and empty
                state) off status==="active" regardless of mode, and
                hiding the Active/Inactive tabs + "uploaded by" filter
                in pick mode (both meaningless there — dead controls
                otherwise). (2) the crop dialog's first version created
                the blob URL in a useState lazy initializer and revoked
                it in a separate cleanup-only effect — looks like a
                correct create/destroy pairing but isn't: React 19 dev
                Strict Mode double-invokes every effect (mount ->
                cleanup -> mount again) specifically to catch this, and
                the cleanup-only effect's first simulated pass revoked
                the URL the initializer had already committed to state
                — the "recreate" half never happened, since a useState
                initializer only runs once. Symptom: the Cropper showed
                a dead blob (naturalWidth/Height stuck at 0, blank gray
                box) — caught live, not in review. Fixed by computing
                the URL via useMemo and revoking it explicitly from the
                two real exit paths (apply, skip) instead of an
                automatic effect cleanup; abandoning the dialog via
                Escape/backdrop/X leaks one blob URL (accepted —
                garbage-collected on tab close, this dialog isn't a hot
                path).
                pwa-form.tsx: the screenshots useFieldArray gained an
                inline "browse gallery" button per row (replace that
                row's URL) plus a new "Add from gallery" button (append
                from a picked file); the manual "Add screenshot" blank-
                row button stays for pasting external URLs. New i18n
                keys form.browseScreenshotAria/
                addScreenshotFromGalleryButton (en+ru).
                Verified in a real, live browser session against the
                actual reporting user's own account: opened the icon
                picker, uploaded a real image, cropped it at 4:3,
                confirmed the crop actually applied visually (not just
                that a request succeeded), selected it back into the
                icon field, and confirmed both the live preview panel's
                app icon and the raw manifest JSON preview showed the
                real public URL. Repeated for screenshots via "Add from
                gallery" — the live preview's screenshot slider picked
                it up. No console errors on a fresh page load. Test
                upload archived afterward; the account's own real
                gallery content left untouched. tsc/eslint/vitest clean
                across the whole apps/web tree.
                Round 4, same phase: user pointed at the icon field
                specifically and asked to drop the raw URL text box
                entirely — a pick-only surface. pwa-form.tsx's icon
                field is now one 96px square button: empty shows an
                upload glyph + "Choose image", set shows the real
                thumbnail with a "Change" overlay on hover; either
                state opens the same GalleryFilePickerDialog. The
                manual Input + its browse-icon button are gone — no way
                to paste an arbitrary external URL into this field
                anymore, the user's explicit ask, not an oversight.
                register("iconUrl") is gone too; the field is written
                only via form.setValue from the picker, which RHF
                tracks the same way without a bound DOM input (the
                resolver validates the full internal values tree, not
                just registered fields). iconUrlLabel shortened to
                "Icon"/"Иконка" (no URL shown anymore); new
                iconUrlChoose/iconUrlChange; removed the now-unused
                iconUrlPlaceholder/iconUrlBrowseAria; iconUrlInvalid's
                message changed from "enter a valid URL" to "choose an
                icon" (the old wording assumed a user could still type
                a malformed one). Verified: tsc/eslint/vitest clean;
                real browser pass against the same live account —
                selected an existing gallery file via the new
                thumbnail button, confirmed it renders as a real
                thumbnail (not a placeholder) and the live preview
                panel's app icon updated to match, no console errors.
                Round 5, same phase: two corrections from the user
                watching the reference tracker's real UI side by side
                with this build. (1) Crop belongs at attach-time, not
                upload-time — the reference uploads to the gallery as-
                is, crop only happens when a picked/uploaded file is
                actually attached to a field with a required shape.
                Reworked: gallery-upload-cell.tsx uploads straight
                through again everywhere (no crop step, matches every
                caller incl. the main gallery page); gallery-browser.tsx
                gained requiredAspect (pick mode only) — confirming a
                selection opens ImageCropDialog locked to that ratio
                before onPick, skip uploads the original untouched. A
                freshly uploaded file becomes the pick immediately (new
                onUploaded callback on GalleryUploadCell) since it isn't
                in the list query yet to select from. Applying a crop
                uploads the cropped result as its own new file — the
                source is never mutated. pwa-form.tsx's icon field
                passes requiredAspect={1} (square, the real Web App
                Manifest convention); screenshots pass none — Play Store
                screenshots genuinely vary in aspect, forcing one would
                be fabricated. Cropping an existing (not freshly
                uploaded) gallery file draws its remote URL onto canvas,
                needing CORS or the canvas is tainted — tried minio-go's
                SetBucketCors first, self-hosted MinIO doesn't implement
                it (confirmed live: NotImplemented); turned out
                unnecessary, MinIO already serves CORS off its own
                server-level api.cors_allow_origin admin config
                (defaults to "*", confirmed via a raw curl with an
                Origin header). Removed the bucket-CORS call from
                storage.Client.EnsureBucket entirely rather than keep
                one the server rejects to duplicate a default it already
                applies — documented there that real S3 in production
                would need CORS set through its own console/CLI if this
                behavior is wanted there.
                (2) The crop interaction itself was backwards — built
                first on react-easy-crop (pan/zoom the image behind a
                fixed frame). The user pointed at the reference's actual
                crop screenshot: the image stays static, the user drags/
                resizes a selection rectangle over it. Swapped libraries
                entirely: react-easy-crop out, react-image-crop in
                (ReactCrop + its own cropToCanvas helper, handles
                natural-vs-displayed scaling and devicePixelRatio so
                this package doesn't have to). image-crop-dialog.tsx
                rewritten around it — static <img> inside <ReactCrop
                aspect={...}>, centered 90% initial selection, "Required
                aspect ratio: 1.00" caption matching the reference's own
                wording, zoom slider and aspect-preset buttons gone
                (one fixed ratio now, not a chooser).
                Also, follow-up ask: a small × button on the icon
                thumbnail (visible only once set) clears iconUrl — a
                sibling button positioned over the picker button, not
                nested inside it (native button can't nest button).
                Verified: go build/vet/test -count=1 ./... clean (no
                behavior change to contentgallery/storage beyond the
                removed CORS call). tsc/eslint/vitest clean. Real
                browser pass against the same live account: opened the
                icon picker, selected an existing non-square gallery
                file, confirmed the dialog shows it static with a
                centered draggable/resizable square selection, dragged a
                corner handle to resize, clicked Apply, and confirmed
                via a fresh Image() probe the resulting file is a
                genuine 376×376 square with a brand-new object key —
                not the source file's own dimensions or key, and the
                source itself untouched in the gallery. No console
                errors. Test-derivative file archived afterward.
                Round 6, same phase: two more asks — replace the
                screenshots field's URL-list UI with a real upload area
                (the user described the reference's own copy: "Upload
                screenshots (1080×1920px recommended) / Supports: PNG,
                JPEG, WebP / No limit on quantity"), and make the live
                preview's screenshot row actually look and swipe like
                the real listing it models.
                pwa-form.tsx: URL Input rows replaced with an
                aspect-[9/16] thumbnail grid (hover × to remove) plus a
                GalleryUploadCell tile for direct upload — no picker
                dialog needed for the common case — with "Choose from
                gallery" kept for picking an existing file.
                GalleryUploadCell gained an optional className prop
                (merged via cn/tailwind-merge over its default
                aspect-square) so this portrait grid and the square
                Content Gallery grid share the one component. Video
                formats (MP4/WebM/MOV) the reference lists are
                deliberately left out of the shown copy — Content
                Gallery doesn't support video uploads yet, a separately-
                scoped real gap, and claiming support that doesn't exist
                would be a fabricated capability; copy only promises
                PNG/JPEG/WebP, which actually work. The count cap is
                gone both sides, matching "no limit on quantity" —
                removed apps/internal/pwa/service.go's screenshotsMax
                check and the matching frontend zod .max(12, ...); no
                test asserted the removed cap, per-item URL validation
                kept. pwa-preview-card.tsx's screenshot row: was
                object-contain + w-auto (card width varied per image's
                own aspect ratio, loose/inconsistent); now aspect-[9/16]
                + object-cover for a uniform card regardless of source
                shape, snap-x snap-mandatory + snap-start so it settles
                one card at a time, scrollbar hidden to match Google
                Play's own chrome. The swipe itself needed no new code
                — overflow-x-auto was already there, which is what makes
                a horizontal strip finger-scrollable on any touchscreen;
                the actual fix was the visual mismatch, not missing
                interactivity. Verified: go build/vet/test -count=1
                ./... clean (internal/pwa's own suite incl.); tsc/
                eslint/vitest clean. Real browser pass against the
                actual reporting user's own live PWA record (already
                carrying 3 real screenshots from their own earlier
                testing): confirmed via the DOM all three loaded with
                real non-zero natural dimensions, overflow-x:auto and
                scroll-snap-type:x mandatory were both active, content
                genuinely overflows, and setting scrollLeft
                programmatically actually moved the scroll position —
                the same mechanism a real finger swipe drives. Uploaded
                a new screenshot through the dropzone tile separately
                and confirmed it appended as a real thumbnail and
                appeared in the live preview immediately. No console
                errors.
                Round 6 fix, same phase: user reported the screenshot
                row rendering squished ("криво"). Root cause: the
                screenshot row is a flex item inside pwa-preview-card's
                flex-col card (capped at max-h-[70vh], overflow-y-auto);
                setting overflow-x-auto on it (needed for swipe) makes
                its own automatic min-height resolve to 0 per the CSS
                flexbox/overflow spec (non-visible overflow on any axis
                collapses the automatic minimum), so once a PWA's card
                content exceeded 70vh the flex algorithm was free to
                crush this row toward zero instead of leaving it to the
                outer scroll container. Fixed with an explicit
                h-[204px] shrink-0 on the row itself. Follow-up in the
                same exchange: user asked to look up the real Google
                Play screenshot height and use it exactly, with no
                vertical scroll in the row. True native-app measurement
                wasn't reachable (no viewport emulation in this
                sandboxed browser), but live-inspecting play.google.com
                revealed the real technique: one fixed height, each
                image's width following its own natural aspect ratio —
                not a uniform cropped box. That's not what the squish
                fix had shipped alongside it: aspect-[9/16] +
                object-cover (forcing a uniform box) had been added
                while chasing the squish bug, an unrequested change.
                git diff against the pre-session baseline showed the
                original code was already h-[204px] w-auto ...
                object-contain (natural width, no forced box) — a
                value from an earlier, already reference-validated
                phase. Reverted to that shape; the existing 204px
                already matches Google Play's own real technique once
                the erroneous forced box was undone. Added explicit
                overflow-y-hidden on the row (previously only implicit)
                per the user's explicit "no vertical scroll" ask.
                Verified: tsc/eslint clean; real browser pass against
                the same live record — three real screenshots (one
                landscape, two portrait) now render at three different
                widths sharing one 204px height, no cropping/
                distortion, overflow-y:hidden confirmed active, no
                console errors.
                Round 7, same phase: user pasted the reference's own
                rendered screenshot-gallery HTML —
                .embla/.embla__viewport/.embla__container/.embla__slide
                sliding via transform: translate3d(...) — the exact DOM
                shape of Embla Carousel (MIT, real open-source, also
                what shadcn/ui's own carousel wraps), not native
                overflow-x scroll. Added embla-carousel-react and
                rebuilt the row as its own ScreenshotsCarousel component
                (useEmblaCarousel({dragFree: true, containScroll:
                "trimSnaps"})) — matches the reference's actual
                technique, not just its look; dragFree matches how the
                reference's own mixed-width screenshots feel to drag
                (travels however far dragged, no hard snap-per-slide).
                Restructuring surfaced a real sizing bug: the prior
                round's h-[204px] alongside pb-5 on the same element
                rendered images at 184px, not 204 (border-box padding
                eating into the explicit height) — fixed by moving
                px-4/pb-5 onto a plain unpadded outer wrapper, keeping
                the Embla-ref'd viewport (the element that actually
                needs the squish-guard) unpadded. Added loading="lazy"
                per img, matching the reference's own markup. Verified:
                tsc/eslint clean; real browser pass against the same
                live record confirmed all three screenshots measure
                exactly 204px (not 184), viewport clips with
                overflow:hidden, and a real synthetic mouse drag (a raw
                JS-dispatched PointerEvent didn't register with Embla's
                own gesture recognition in an earlier attempt — real
                synthetic mouse input via the browser tool did) actually
                moved the container's transform from translateX(0) to
                translateX(-118px), confirmed both via getComputedStyle
                and visually. No console errors.
                Round 8, same phase: user pointed at the raw theme/
                background color-picker pair plus a reference tracker's
                landing-page "Цветовая схема" (Auto/Green/Blue/Blue-
                modern/Purple) + separate "Тема" (light/dark/auto)
                dropdowns, asking for dropdowns here too, wired up for
                real. PWA only has two raw fields (theme_color/
                background_color), not a whole design system, so the
                two reference dropdowns became one: a new
                PWA_COLOR_SCHEMES list of six named (theme, background)
                pairs (Зелёная/Синяя/Синяя современная/Фиолетовая/
                Тёмная/Светлая), each a SelectItem with two swatch dots,
                replacing the raw picker section. Picking a scheme sets
                both fields via form.setValue. A seventh "Своя" (Custom)
                option isn't a preset — it reveals the original raw
                picker pair unchanged, so any exact hex stays reachable.
                The dropdown's selected value is derived
                (matchColorScheme(theme, background) against the six
                presets, falling back to "custom"), not independent
                state — a saved PWA with non-preset colors correctly
                opens on "Custom" with its real colors intact. One
                forceCustomColors boolean handles the edge derived state
                can't: picking "Custom" when current colors already
                exactly match a preset (e.g. fresh defaults) — without
                it the dropdown would immediately snap back since
                nothing about the values would have changed. Verified:
                tsc/eslint clean; real browser pass against a live PWA
                — all six presets render with correct swatch colors,
                selecting "Фиолетовая" changed the real manifest JSON
                preview's theme_color to #7c3aed (not a cosmetic label),
                selecting "Своя" afterward correctly revealed the
                original raw pickers. No console errors.
                Round 8 fix, same phase: user clarified they wanted the
                reference's own two SEPARATE dropdowns, not one combined
                picker — "Цветовая схема" (accent) driving theme_color
                alone, a second "Тема" (light/dark) driving
                background_color alone. Split PWA_COLOR_SCHEMES into
                PWA_ACCENT_COLORS (Зелёная/Синяя/Синяя современная/
                Фиолетовая, matching the reference exactly) and
                PWA_BACKGROUND_THEMES (Светлая/Тёмная — the reference's
                own third "Авто" option deliberately left out, since a
                required manifest color field has no real "auto" value
                to map it to). Each dropdown now has its own independent
                "Custom" fallback and its own matchPreset-derived value
                + forceCustom* boolean (same pattern as round 8's first
                pass, duplicated per field instead of shared). Verified:
                tsc/eslint clean; real browser pass confirmed selecting
                "Светлая" changed background_color to #ffffff while
                theme_color stayed untouched at #16a34a — the two are
                genuinely independent now. No console errors.
                Round 9, same phase: user reported picking a color
                scheme/theme did nothing visible in "Живое превью." Not
                a bug — PwaPreviewCard (the Google Play listing mockup)
                never accepted themeColor/backgroundColor as props at
                all, because the real Play Store listing genuinely never
                uses an app's own colors (always its own green
                branding), so wiring them in there would've been
                inaccurate. Real gap: nothing in the form showed these
                two fields' actual spec-defined effect at all. Added
                PwaLaunchPreview (pwa-preview-card.tsx), rendered above
                the listing mockup: a simulated status bar tinted with
                theme_color + a splash-screen block tinted with
                background_color (icon centered, name below) — exactly
                what the Web App Manifest spec uses these two fields
                for (OS chrome tint, pre-paint launch background), not a
                fabricated field. Readable text color picked via a small
                isLightColor perceived-brightness check against whatever
                background_color is set. Caption states plainly neither
                field affects the listing preview below it. Verified:
                tsc/eslint clean; real browser pass confirmed the launch
                preview's status bar visibly turned blue immediately
                after selecting "Синяя" — the concrete live feedback
                that was missing. No console errors.
                Round 10, same phase: user rejected round 9 outright —
                "Убери ее я ни это имел ввиду чтобы ты сделал" (remove
                it, that's not what I meant). PwaLaunchPreview was a
                plausible reading of "ничего не меняется" but not the
                actual ask: the user's real complaint was that changing
                a color scheme has no visible effect on the Google Play
                listing mockup itself, not that a second, separate
                status-bar/splash widget was missing. Removed
                PwaLaunchPreview and every trace of it — component,
                isLightColor/HEX_RE helpers, its i18n keys
                (preview.statusBarLabel/launchPreviewAppNameFallback/
                launchPreviewCaption, en+ru), its usage in pwa-form.tsx
                — confirmed clean via grep.
                Round 11, same phase: "Мне надо чтобы при выборе
                допустим цветовой схемы Синяя чтобы все зеленые цвета
                сменились на синию и так далее" — the actual fix round
                9 was reaching for: make the listing mockup's own
                accent color follow the selected scheme. This does
                contradict round 9's own reasoning (a real Google Play
                listing always renders Google's fixed green, never an
                app's declared color) — resolved by treating it
                explicitly as a deliberate FLOX-specific preview
                convenience, not a claim about real Play Store
                behavior, and saying so in a doc comment on the new
                prop rather than silently reversing the earlier call.
                pwa-preview-card.tsx: PwaPreviewCard gained an optional
                `accentColor` prop (fed `manifest.themeColor` from
                pwa-form.tsx); every hardcoded `REF_PRIMARY` (#00A173,
                the reference's fixed brand green) render-site use —
                install button, developer name + verified-style accent,
                rating star, per-star chart bars, Share/Wishlist icons,
                active-nav-pill — now reads a computed `accent` that
                validates `accentColor` against a hex pattern and falls
                back to REF_PRIMARY when unset/invalid, so a PWA with no
                theme_color set (or an invalid one) renders exactly as
                before. PartialStar/StarBar (the rating-breakdown
                helpers) gained their own `accent` prop threaded down
                from the same value.
                Verified: tsc --noEmit/eslint clean on both files;
                vitest run clean (21/21); real browser pass against a
                live PWA — filled in real rating/rating-count/star-
                breakdown values (previously placeholder-only), selected
                "Синяя современная" and confirmed via zoomed screenshot
                that the install button, developer name, rating star,
                and all five per-star bars turned blue; switched to
                "Фиолетовая" and confirmed the same elements turned
                purple without a reload — genuinely reactive, not a
                one-time coincidence; reloaded the page and confirmed
                the PWA's real saved state (green, unaffected — none of
                the test edits were submitted) came back with no console
                errors on load.
                Round 12, same phase: "тоже самое сделай с темой при
                выборе темная тема фон должен смениться на темную и
                наоборот" — same treatment for the second ("Тема")
                dropdown: selecting Тёмная/Светлая should flip the
                mockup's own background, not just the two fields it
                writes. Same tension as round 11 (a real Google Play
                listing is always light, never driven by an app's
                declared background) — resolved the same way: an
                explicit, documented FLOX-specific preview convenience,
                not a claim about real store behavior.
                pwa-preview-card.tsx: PwaPreviewCard gained an optional
                `backgroundColor` prop (fed `manifest.backgroundColor`).
                A new `isDarkColor` perceived-brightness check (ITU-R
                BT.601 weights) decides light vs. dark from the color's
                own value, not just an exact match against the two
                dropdown presets — so a custom dark hex Picked via
                "Своя" also flips the mockup correctly, not only the two
                named presets. Every color that used to be a hardcoded
                Tailwind slate/white class (card background, all body/
                muted/subtle text, borders, the rating-bar track, the
                empty-star fill, hover backgrounds) now reads one of
                seven `--pv-*` CSS custom properties set once on the
                card's own root element and consumed via Tailwind
                arbitrary-value classes (`text-[var(--pv-text)]` etc.)
                — chosen over threading a `dark` boolean prop through
                every sub-component (PartialStar, StarBar, the About
                sheet) since CSS custom properties already cascade to
                every descendant for free, including the About sheet
                overlay, with no extra prop plumbing.
                Found and fixed one real bug from round 11 along the
                way, not just this round's own gap: the active-nav-pill
                highlight's translucent background was still the
                literal hardcoded class `bg-[#00A173]/10` — round 11's
                own accent work only ever set the pill's *text* color
                dynamically (`style={{color: accent}}`), never touched
                this background class, so the highlight tint stayed the
                old fixed green under every other scheme the whole time
                since round 11 shipped. Fixed with a new `withAlpha`
                helper (hex -> `rgba(...)` at a given opacity) so the
                tint now reads the live `accent` value like everything
                else round 11 touched.
                Verified: tsc --noEmit/eslint clean on both files;
                vitest run clean (21/21, unrelated). Real browser pass
                against the same live PWA: selected "Тёмная" and
                confirmed via screenshot the entire mockup — header bar,
                icon/name block, stat row, screenshot gallery surrounds,
                About section, Data safety, Reviews, What's new, footer,
                and bottom nav — switched to a dark background with
                light text, while the install button/accent elements
                correctly stayed on their own independent accent color;
                opened the "About this app" sheet and confirmed it also
                rendered dark (the CSS-variable cascade reaching the
                overlay with no extra wiring); switched back to
                "Светлая" and confirmed a full, exact revert; zoomed on
                the bottom nav's active "Игры" pill and confirmed its
                background tint now matches the current accent color
                rather than the old fixed green. Reloaded the page
                afterward and confirmed the PWA's real saved state
                (light, untouched — no test edits were submitted) came
                back correctly, with no console errors on load.
                Round 13, same phase: user pointed at a screenshot of
                the two dropdowns both showing "Своя" with their raw
                color-swatch + hex-input rows underneath and said
                "Убери эти палитры они теперь не нужны" (remove these
                palettes, they're not needed anymore). Removed the raw
                picker escape hatch entirely rather than just hiding it
                — same "pick-only, no manual entry path" call already
                made for the icon field back in round 4, now applied to
                these two fields too, per the user's own explicit
                instruction (this reverses round 5's original "never
                remove the ability to set an arbitrary color" reasoning
                for these two fields specifically — that was this
                feature's own earlier design assumption, not a user
                directive, and the user has now directed otherwise).
                Removed: the `custom`/"Своя" SelectItem from both
                dropdowns, both `<input type="color">` + hex `Input`
                blocks and their error displays, `forceCustomAccent`/
                `forceCustomTheme` state, and the "custom" branch in
                both change handlers. `matchPreset` simplified to
                return `""` (no dropdown selection shown) instead of
                `"custom"` when a stored color doesn't match either
                preset list — the field's real value is untouched
                either way; only the dropdown itself renders blank
                until the operator picks one of the fixed presets.
                Removed now-unused i18n keys (en+ru):
                colorScheme.custom, backgroundTheme.custom,
                themeColorLabel, themeColorPickerAria,
                backgroundColorLabel, backgroundColorPickerAria (the
                latter four were already unused before this round, a
                leftover from the round-8 dropdown refactor, confirmed
                via repo-wide grep before deleting).
                Same message, second ask: "по умолчанию оставь
                цветовая схема зеленая и тема светлая" — the form's own
                `defaultValues` (new-PWA path) already defaulted
                `themeColor` to green (`#16a34a`, matching the accent
                preset), but `backgroundColor` defaulted to
                `#0a0a0a` (dark). Changed to `#ffffff` so a brand-new
                PWA opens on Зелёная/Светлая, matching the ask exactly;
                edit mode is unaffected (real record values always
                override these via the form's own `...defaultValues`
                spread).
                Verified: tsc --noEmit/eslint clean; vitest run clean
                (21/21, unrelated); JSON.parse confirmed both locale
                files stayed valid after the key removals. Real browser
                pass: opened the same live PWA (whose stored colors,
                #27914e/#c39797, match neither preset list) and
                confirmed both dropdowns now render with no visible
                selection and no palette rows beneath them, opening the
                accent dropdown shows exactly the four presets with no
                "Своя" entry, and the PWA's own actual color values were
                left untouched (the live preview's install button still
                showed the original green, not silently reset). Opened
                a fresh /pwa/new and confirmed via zoomed screenshot
                that Цветовая схема shows "Зелёная" and Тема shows
                "Светлая" by default, with the listing mockup itself
                rendering light/green from first load. No console
                errors either page.
                Round 14, same phase: user asked what "URL запуска"
                (start_url) does, then said to remove it for the
                operator entirely — "это PWA привязывается к кампании
                и в потоки" (this PWA gets attached to a campaign and
                into flows), i.e. real routing after install/open is
                resolved through campaign/stream-set/flow attachment,
                not a manually-typed manifest path with no connection
                to that routing. Confirmed via grep that start_url was
                never actually consumed by apps/tracker or any manifest-
                serving endpoint — it's stored and shown in the client-
                side manifest-JSON preview only, so removing the
                operator input changes nothing about real request
                handling. Kept `startUrl` in the Zod schema/manifest
                preview/backend (still a real, valid Web App Manifest
                field) but removed its `<Label>`/`<Input>`/error-display
                JSX, replacing it with `<input type="hidden"
                {...register("startUrl")} />` — kept registered rather
                than dropped outright so RHF's submission still includes
                it without relying on defaultValues-without-registration
                semantics. New-PWA path keeps its existing `"/install"`
                default (now permanently fixed, no longer operator-
                editable); edit mode is unaffected — a record's real
                saved value passes straight through untouched via the
                form's own `...defaultValues` spread, same mechanism
                round 13's palette removal relied on. Removed the two
                now-unused i18n keys (`startUrlLabel`/
                `startUrlPlaceholder`, en+ru); `validation.
                startUrlRequired` stays (still referenced by the schema
                itself, now practically unreachable via the UI but kept
                as a data-integrity safety net, not dead code by any
                narrower definition).
                Verified: tsc --noEmit/eslint clean; vitest run clean
                (21/21, unrelated); JSON.parse confirmed both locale
                files stayed valid. Real browser pass: confirmed "URL
                запуска" no longer appears anywhere in either the
                create or edit form; created a real new PWA end-to-end
                (icon via the gallery picker + crop, name/short name
                filled in, submitted) and confirmed via a direct
                authenticated fetch against the real API
                (GET /pwas/{id}) that the persisted record's startUrl
                came back exactly "/install" despite no visible input
                ever setting it — the hidden-field wiring genuinely
                works, not just compiles. No console errors on either
                page. Test PWA archived afterward, not left active.
                Round 15, same phase: user asked to remove the "Статус"
                dropdown from the form and move its buttons into the
                card instead. Exact precedent already shipped for
                Campaigns: campaign-form.tsx has carried no visible
                status field at all since its own equivalent round (only
                `status` in schema/defaultValues, submitted transparently
                on every save), with CampaignRowActions rendered
                separately as Pause/Duplicate/Archive buttons in the
                page header next to the Save button, via an
                `variant="icons" | "inline"` split (icon buttons for the
                table row, labeled buttons for the detail-page header).
                Mirrored exactly: removed pwa-form.tsx's Статус
                Label/Controller/Select block (status stays in schema/
                defaultValues only, same as startUrl before it — no
                hidden input needed here since campaign-form.tsx's own
                status field proves RHF submits unregistered-but-
                defaultValues-seeded fields correctly); removed the now-
                unused STATUS_OPTIONS const and statusLabel i18n key
                (en+ru, confirmed unused elsewhere by grep — the other
                "form.statusLabel" hits across the repo all belong to
                other features' own i18n namespaces). pwa-row-actions.tsx
                gained the same `variant` prop/split as
                CampaignRowActions (icons unchanged; new inline branch:
                labeled Pause-or-Resume/Duplicate/Archive buttons, Edit
                dropped since you're already on the edit page); Duplicate
                now navigates to the new PWA's own edit page on success
                (previously stayed put) — matching CampaignRowActions'
                own duplicate behavior exactly, not a new UX decision.
                PwaForm gained an optional `headerActions?: ReactNode`
                prop rendered via a new `<CardAction>` in its own
                CardHeader (omitted on the new-PWA page — no existing
                record to act on yet); pwa-detail-view.tsx passes
                `<PwaRowActions pwa={pwa} variant="inline" />`.
                Real bug caught live, not in review: the first browser
                pass showed the header's Pause button doing nothing
                visible, then a second click fired what looked like a
                full form-submit toast ("PWA обновлено") instead of the
                pause toast. Root cause: unlike CampaignRowActions
                (rendered as a page-header sibling, entirely outside
                CampaignForm's own `<form>`), PwaRowActions here renders
                *inside* PwaForm's CardHeader, which sits inside the
                form element — and shadcn's Button/IconButton pass
                `type` straight through to a native `<button>` with no
                default, so every action button without an explicit
                `type="button"` fell back to the HTML default of
                `type="submit"` and triggered the surrounding form's
                submit handler on click. Fixed by adding `type="button"`
                to all eight buttons in pwa-row-actions.tsx (both
                variants, plus the archive-confirm dialog's own two
                buttons, which are Radix-portalled outside the form DOM-
                wise and so weren't actually broken — added anyway,
                defensively, since correctness here shouldn't depend on
                portal semantics). Confirmed via the browser tool's own
                network-request log that the fix genuinely rewired
                click -> POST /pwas/{id}/pause instead of a form submit,
                not just that the toast text looked right.
                Verified: tsc --noEmit/eslint clean; vitest run clean
                (21/21, unrelated); JSON.parse confirmed both locale
                files stayed valid. Real browser pass against a real
                duplicated test PWA: clicked Pause (network log showed
                POST .../pause 200, a direct GET confirmed status
                "paused", button flipped to "Возобновить" with no form-
                submit side effect), clicked Resume (GET confirmed back
                to "active"), clicked Archive -> confirmed in the real
                dialog -> confirmed (GET confirmed "archived"); separately
                confirmed Duplicate's own click earlier had correctly
                created a real new PWA and navigated to its edit page.
                Confirmed the list page's own "icons" variant is
                unaffected (edit/pause/duplicate/archive icons still
                render correctly for an active row, edit/duplicate only
                for archived rows). Confirmed the new-PWA page shows
                neither a Статус field nor any header buttons (nothing to
                act on yet). No console errors on any of the three pages
                touched. Both test PWAs (the round-14 startUrl test and
                this round's duplicate) left archived, not deleted or
                re-activated.
LAST COMMIT   : fix(pwa): listing preview follows accent/theme, drop
                status field for header actions — 826d621, preceded by
                two same-session commits also covering this phase's
                earlier rounds: 8655b3e "fix" (rounds 3-9: crop-at-
                attach-time rework, react-image-crop, Embla screenshot
                carousel, PwaLaunchPreview's original add) and 23b16fc
                "fix" (round 1-2: the Content Gallery backend +
                frontend rewrite itself — migration 00027, apps/
                internal/storage, apps/internal/contentgallery, the
                full features/content-gallery/* rewrite, permanent-
                delete UI). Between them these three commits cover
                every round documented above.
UNCOMMITTED   : none — the working tree is clean as of 826d621.
                NOTE: same as every
                prior phase this session, the migrate container bakes
                migrations into its image — `docker compose build
                migrate` before 00027 will apply inside a rebuilt stack
                (already applied directly via host `go tool goose`
                against the live dev Postgres, so the running dev stack
                has it regardless). NOTE: apps/web's containerized dev
                service has its own node_modules Docker volume, separate
                from the host's — react-image-crop was installed on the
                host AND via `docker exec infra-web-1 npm install` so
                the container actually resolves it; a stack rebuilt from
                docker-compose.dev.yml fresh (`docker compose build
                web`) will pick it up from package.json normally.
NEXT          : confirm scope before starting. Real candidates: (a)
                video upload + transcoding (needs an ffmpeg/worker
                pipeline — real new infra, not a small add-on); (b) the
                reference's dimension-compatibility indicators (green/
                yellow/red match icons) — still not built, would need a
                real "required dimensions" concept on a field, which
                none currently declares beyond the icon's own fixed 1:1;
                (c) wire the file-picker/crop combo into any other image
                field that gets added later (icon + screenshots are the
                only two image URL fields PWA has today, both now
                wired); (d) the pre-existing list from the superseded
                block below, still open.

```

<details>
<summary>Superseded — PWA create page preview (kept for history, see above for current state)</summary>

```
CURRENT PHASE : PWA CREATE PAGE — LIVE STORE-LISTING PREVIEW (user-scoped, post-Phase-34)
STATUS        : done — user shared a reference tracker's PWA builder
                screenshot (settings left, a live Google-Play-listing
                phone mockup right, plus a "parse assets from a Google
                Play URL" feature) and asked for the same. Scoped with
                the user first: two candidates existed (visual redesign
                vs. Google Play scraping — the latter is a genuinely new
                backend capability, no scraping/external-fetch anything
                existed anywhere in the repo). User chose the preview
                first, and to add real form fields behind it rather than
                showing fabricated stats (CLAUDE.md #6) — Google Play
                parsing itself is NOT built this phase, deferred pending
                explicit go-ahead.
                Backend: migration 00023 adds developer/description/
                rating/rating_count/downloads_label/size_mb to pwas
                (rating/rating_count/size_mb nullable — "never entered"
                stays distinct from a fabricated 0/"—", same reasoning
                cost/ROI fields already use). apps/internal/pwa's model/
                repository/service/handler carry the six fields through
                Create/Update/Duplicate; service.go validates ranges
                (rating 0-5, non-negative count/size) via a new
                validateListingFields, all fields optional so blank never
                fails validation.
                Frontend: pwa-form.tsx restructured into a
                lg:grid-cols-[1fr_300px] two-column layout — existing
                Web-App-Manifest fields plus six new "store listing"
                inputs on the left (Card, unchanged otherwise), a sticky
                new features/pwa/pwa-preview-card.tsx (a Google-Play-
                listing phone mockup — icon/name/developer/rating/
                downloads/size/install button, deliberately light-themed
                regardless of host dark mode, matching a screenshot of a
                different app) on the right, live-updating via the
                form's existing useWatch. Numeric fields use
                register(name, { setValueAs }) rather than
                z.preprocess — the latter made resolver input/output
                types diverge, which @hookform/resolvers' Resolver<Input,
                Context, Output> rejected against a single-type
                useForm<PwaFormValues> (documented in pwa-form.tsx).
                Round 2, same session: user pasted the reference
                tracker's actual rendered HTML for the preview (not the
                loader script itself — declined to embed
                cdn.adset.pro/pwa-app.js, an uncontrolled third-party
                script, as a supply-chain risk) and, after seeing the
                compact card, asked for the rest of the real listing
                page too: screenshot gallery, "what's new," and the
                per-star rating breakdown (5★..1★). Scoped again before
                building: confirmed these three want real backend
                fields (not fabricated), and that the footer links
                (Store/Gift cards/redeem/family library/Terms/Privacy/
                Developers) and bottom nav bar (Игры/Приложения/Книги/
                Детям) are pure decoration, matching the real Google
                Play chrome — never wired to anything.
                Migration 00024 adds screenshots (text[], ordered gallery
                URLs — same shape postlanding.Events already uses),
                whats_new (text), and rating_stars_5..1 (numeric(5,2),
                each independently nullable — a star bucket never
                entered stays "—", not a fabricated 0%). apps/internal/
                pwa's model/repository/service/handler/Duplicate carry
                all eight new fields; validateListingFields refactored
                to take a struct (listingFields) now that it covers this
                many optional fields, plus a new trimAll helper so a
                blank row from the frontend's add/remove screenshot list
                never persists as an empty-string URL.
                pwa-form.tsx: screenshots is a useFieldArray (matching
                offer-form.tsx's `links` pattern) of {url} objects,
                converted to/from the API's flat string[] at the two
                call sites (new-pwa-view.tsx, pwa-detail-view.tsx); a
                5-column rating-breakdown input row; a "what's new"
                textarea. pwa-preview-card.tsx grew from a compact stat
                card into the full scrollable listing page (max-h-[70vh]
                overflow-y-auto, sticky header/nav) — screenshot gallery
                (hidden entirely when empty, not a placeholder box),
                About/description, update date, a generic Data Safety
                paragraph (own short text, not Google's verbatim legal
                copy — copyright caution), Ratings & reviews with live
                per-star bars, What's New (hidden when empty), then the
                decorative footer + bottom nav.
                Verified: go build/vet/test -count=1 ./... clean
                (including all listing-field and gallery-field round-
                trip/range-validation/Duplicate-copies-fields tests
                against real Postgres — TestScreenshotsWhatsNewAnd
                RatingBreakdownRoundTrip, TestCreateRejectsOutOfRange
                GalleryFields, TestDuplicateCopiesGalleryFields);
                tsc/eslint/vitest clean on apps/web (one pre-existing
                unrelated TS error in the untracked app/(app)/pwa/[id]/
                page.tsx — not this phase's file, left alone). Real
                browser pass both rounds: typed into every field
                (including adding a screenshot row) and confirmed via JS
                DOM assertions that every new section actually rendered
                (screenshot img, what's-new text, About, Data Safety,
                Reviews, footer Terms link, nav Games label, 5 rating
                bars) — then a full create → redirect → edit-page-reload
                round trip confirmed via a direct Postgres query that
                every field (screenshots array, whats_new,
                rating_stars_5/1) persisted correctly; the verification
                PWA row was deleted afterward, not left behind.
                Round 3, same session: user asked for the preview to be
                "real phone" width — card widened 280px -> 390px (an
                iPhone 14/15-class CSS width), border/radius scaled up to
                match, sidebar column widened 300px -> 440px to fit it
                without clipping. Confirmed in browser: fits cleanly, no
                overflow.
                Round 4, same session: user pasted the reference's raw
                <head> HTML twice, asking to load cdn.adset.pro/
                pwa-app.js (a rival tracker's own rendering engine) and
                Google Translate's element.js (a live third-party
                widget) so the preview would "look the same." Declined
                both, explicitly, twice — embedding either is a supply-
                chain/privacy risk this package doesn't need: the actual
                rendered HTML the user pasted first was already used to
                match the visual structure directly in round 2, so the
                loader script itself added nothing. User then confirmed
                the actual ask was "same functionality, our own code" —
                consistent with what was already built. Applied the one
                legitimate finding from that HTML: "Google Sans" isn't
                actually served by Google Fonts (it's Google's internal-
                only typeface) — the reference's own real rendered font
                is its declared fallback chain (-apple-system,
                BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif), now
                set directly on the card via inline style, zero external
                font requests. Also built, natively, the one real feature
                from the original screenshot not yet covered: the
                "Арабский текст" RTL toggle — a Switch in the preview
                Card's header (local useState, no persistence needed)
                setting a new `dir` prop through to PwaPreviewCard's root
                div. No machine translation (that would need the same
                declined Google Translate widget) — just layout
                mirroring, which the existing flex-row layout already
                gets for free from the browser's own dir-aware flexbox
                behavior (row direction follows text direction when no
                explicit flex-direction is set) — confirmed in browser
                (dir="rtl" applied, header/buttons/text visibly mirror).
                Round 5, same session: user pasted the reference's HTML
                a third time asking to load cdn.adset.pro/pwa-app.js so
                "styles/colors match too." Declined the script again,
                same reasoning as round 4 — instead extracted the exact
                color tokens from the pasted CSS directly. The
                reference's :root custom properties turned out to be
                literally Tailwind's own slate scale (text-primary
                #0f172a=slate-900, text-muted #475569=slate-600,
                bg-tertiary #f1f5f9=slate-100, etc.) — pwa-preview-
                card.tsx's neutral-* utilities swapped to slate-*
                throughout. The one non-stock color, --color-primary
                #00A173 green (not the emerald-600 the card used
                before), is now a REF_PRIMARY constant applied via
                inline style on the install button, developer name +
                verified badge, rating star, per-star chart bars,
                Share/Wishlist icons, and a new active-nav-pill
                highlight (the reference highlights "Игры"/Games).
                REF_VERIFIED_BADGE_BLUE (#4285f4, Google's real badge
                blue) replaces lucide's BadgeCheckIcon with the
                reference's own verified-badge SVG path; lucide's
                generic PlayIcon replaced with the real four-color
                Google Play triangle, inlined as SVG paths copied from
                the pasted markup (a static vector, not a fetched
                asset). Install button corrected from a full pill to
                the reference's actual border-radius:8px at its real
                36px height. Rating number now renders through
                Intl.NumberFormat instead of a hardcoded .toFixed(1),
                so it shows "4,7" (comma) under a Russian locale like
                the reference. Verified: tsc/eslint clean; real browser
                pass filled every store-listing field and confirmed via
                screenshot/zoom that header glyph, developer badge,
                install button, rating star, chart bars, active-nav
                highlight, and comma-formatted rating all render
                correctly; no console errors on a fresh page load.
                Round 6, same session: user sent three screenshots of
                the reference's own rendered listing (their product —
                English "New PWA"/"Developer" placeholders with Russian
                chrome) and asked to match it as closely as possible.
                Deltas found and fixed: typography was uniformly too
                small (reference's CSS is 22px title/16px developer/
                16px stat values/18px section headings/14px body vs the
                16/14/10/14/12 the card used; reviews score now its
                real 56px); the stat row was structurally wrong (the
                reference shows the downloads VALUE over the word
                "Скачиваний", and only the size stat leads with an
                icon — the card had a download icon over the value and
                no downloads label at all), now restructured with the
                reference's own framed-arrow size glyph replacing
                lucide's disk-drive icon and the ⓘ after the review
                count; an unset icon now renders nothing at all rather
                than an invented letter avatar (matches the reference
                AND CLAUDE.md #6); the reviews block gained the
                "Оценки и отзывы проверены" subtitle plus the
                five-star row under the score, fractionally filled the
                way the reference clips its own star SVG, with bars at
                their real 10px and the trailing percent column now
                only rendering "—" when a bucket is unset; data safety
                gained the "›" affordance; the footer gained the
                "Детям и семье" group. Two real i18n bugs fixed on the
                way: footer giftCards/redeemCode held each other's
                values in both locales (wrong render order), and
                developers said "О разработчике" where Play says
                "Разработчики". NOT built, deliberately: the
                reference's four itemized data-safety rows (shared/
                collected data types, encryption, deletion) are
                per-app factual claims with no backing field in `pwas`
                — replicating them would ship exactly the fabricated
                content this feature has refused from the start. New
                i18n keys (en+ru): preview.downloadsLabel,
                preview.reviewsVerified, preview.footer.
                kidsFamilyTitle, preview.footer.parentGuide.
                Verified: tsc/eslint clean; real browser pass seeded a
                PWA through the real API with every listing field
                populated, opened its edit page, and confirmed by
                zoomed screenshot that header/stat row/install button/
                gallery/reviews/footer all render as the reference
                screenshots show, no console errors — the seeded row
                was deleted afterward, not left behind.
                Round 7, same session: user sent two more reference
                screenshots — the screenshot slider, and the sheet that
                opens from the arrow next to "О приложении" (icon/name
                header, full description, a "Подробная информация"
                table: обновлено/размер/скачивания/текущая версия/
                возраст/разработчик/категория/интерактивные элементы).
                Four of those eight rows had no column behind them, so
                they became REAL fields rather than hardcoded text
                (same call the user made when this feature started).
                Migration 00025 adds version/age_rating/category/
                interactive_elements to pwas — free text, not enums
                (the reference shows operator-chosen strings like
                "1.0.0"/"12+"/"Casino"; a fixed vocabulary would fight
                whatever the real listing says), with "" meaning never
                entered exactly like developer/downloads_label/
                whats_new, so there's no explicit-blank state to keep
                the way the nullable numeric columns need. Carried
                through model/repository/service/handler/Duplicate;
                validateListingFields caps all four at 60 chars.
                pwa-form.tsx gained a "Подробная информация" section
                with the four inputs; pwa-preview-card.tsx's About
                heading gained the reference's arrow, opening a
                light-themed sheet INSIDE the phone frame (dimmed
                backdrop, bottom nav still visible above it via z-30,
                as the reference renders it) with icon/name header,
                close button, untruncated description and the 2-column
                detail table. A row whose field is blank is dropped
                entirely rather than padded with "—" (unlike the stat
                row, where a dash reads against the numbers beside it).
                Verified: go build/vet/test -count=1 ./... clean
                against real Postgres including 3 new tests (detail
                fields round-trip + default to empty, over-long
                rejected, Duplicate copies them); tsc/eslint/vitest
                clean; real browser pass seeded a PWA with every field
                populated and confirmed all eight rows render in the
                reference's order/pairing with the gallery slides at
                204px, then a second PWA with those fields blank showed
                only the "Обновлено" row and no gallery — both seeded
                rows deleted afterward, no console errors. NOTE: one
                full `go test ./...` run right after an unrelated
                Docker daemon restart reported a FAIL that did not
                reproduce on two clean re-runs (exit 0).
                Round 8, same session: user pointed back at the About
                sheet — "есть еще данные". Two of them turned out to be
                places the preview was asserting things on its own: the
                card printed a HARDCODED "Без рекламы и платного
                контента" for every PWA, and the "Дата обновления"
                block printed new Date(), i.e. claimed every PWA was
                updated today, forever. Migration 00026 adds
                contains_ads/in_app_purchases (booleans, as Play's own
                listing treats them) and updated_date (nullable date).
                The date crosses the API as a plain YYYY-MM-DD string
                (a listing shows a date, not a timestamp), read back
                via to_char and validated with time.Parse in the
                service so a malformed value is a 422, not a Postgres
                error; a blank string from a cleared date input
                normalizes to NULL via blankDateToNil rather than
                failing the ::date cast. Frontend: two Switches + a
                date input in the form's details section; the
                developer line now renders "Есть реклама · Покупки в
                приложении" from the flags and only falls back to the
                ad-free line when both are genuinely off; the sheet's
                developer cell lists them under the name exactly as the
                reference does; the update date renders the operator's
                own value through Intl and the whole block disappears
                when unset. Verified: go build/vet/test -count=1 ./...
                clean incl. 3 new tests (round-trip + Duplicate, blank
                date → NULL, malformed date rejected); tsc/eslint/
                vitest clean; browser pass confirmed both the populated
                and the empty case, no console errors, seeded rows
                deleted.
LAST COMMIT   : feat(pwa): live store-listing preview on the PWA
                create/edit page — 8efe889
UNCOMMITTED   : rounds 5-8's own work (pwa-preview-card.tsx color-token
                + structural match + About detail sheet, migrations
                00025/00026 and apps/internal/pwa's seven new listing
                fields with tests, pwa-form/new-pwa-view/
                pwa-detail-view and lib/api/pwa.ts wiring, en/ru
                pwa.json keys, CHANGELOG entries) — not yet committed,
                waiting on the user. NOTE: the migrate container bakes
                migrations into its image, so a new one needs
                `docker compose build migrate` before it will apply.
                NOTE: pwa-form.tsx/
                new-pwa-view.tsx/pwa-detail-view.tsx had never been
                committed to git before this phase touched them (they
                were part of the pre-existing, unrelated Sheet-to-page
                conversion sitting uncommitted in the tree since before
                this session) — the commit above necessarily includes
                that baseline content alongside this phase's own
                additions, since git had no prior version of these three
                files to diff against. pwa-columns.tsx/pwa-list.tsx/
                pwa-row-actions.tsx (also touched by that same
                unrelated conversion) were left alone and remain
                uncommitted, along with the rest of that conversion's
                other-feature files (domains/landings/networks/offers/
                pixels/postlanding/traffic-sources) and the untracked
                app/(app)/pwa/[id]/page.tsx with its own pre-existing TS
                error — none of that is this phase's work.
NEXT          : confirm scope before starting. Real candidates: (a)
                Google Play URL parsing/scraping — deliberately deferred,
                would need a new scraper (no official public API for
                arbitrary app metadata; unofficial HTML scraping of
                play.google.com is what most trackers do, through
                apps/internal/urlsafety for SSRF safety per §54), a new
                endpoint, and frontend wiring for the "Отправить" button
                the reference screenshot shows; (b) age-rating badge
                (ARC/12+) — present in the reference HTML the user pasted
                but never asked about explicitly, not built; (c) the
                pre-existing list from the superseded block below, still
                open.

```

<details>
<summary>Superseded — Geo/ASN provider (kept for history, see above for current state)</summary>

```
CURRENT PHASE : GEO/ASN PROVIDER — LIVE ON DB-IP LITE (user-scoped, post-Phase-34)
STATUS        : done — follow-up to the Click Log phase below, which
                surfaced that classifier.NoopGeoProvider/NoopASNProvider
                were still the only providers wired up (country/city/ASN
                always empty). Added apps/internal/classifier/maxmind: a
                real GeoProvider/ASNProvider implementation against local
                .mmdb files (github.com/oschwald/geoip2-golang, new
                dependency) — the interfaces themselves needed no changes
                (CLAUDE.md #11). New config GEOIP_CITY_DB_PATH/
                GEOIP_ASN_DB_PATH (.env.example, apps/internal/config),
                wired into apps/tracker/main.go replacing classifier.
                New(nil, nil, nil) — best-effort at startup exactly like
                the existing ClickHouse/Redis handling (CLAUDE.md #9):
                missing/bad path logs a warning and falls back to Noop,
                never blocks the redirect path. A per-IP lookup miss is
                likewise not an error. infra/docker-compose.dev.yml's
                tracker service bind-mounts infra/geoip/ (gitignored) at
                /geoip and points both env vars at it.
                First pass tried MaxMind GeoLite2, the usual free option
                — blocked: MaxMind's signup refused this account both via
                VPN and direct connection (user is in Russia). Switched
                the dev stack's default to DB-IP Lite instead: same
                geoip2-golang library reads its .mmdb format natively (no
                code changes needed, confirmed via getDBType's own
                DBIP-City-Lite/DBIP-ASN-Lite recognition), downloadable
                directly over HTTPS with zero registration, CC BY 4.0
                (free incl. commercial use, attribution required if ever
                shown outside the org — see docs/geoip.md). Real DB-IP
                Lite databases are live in infra/geoip/ right now
                (dbip-city-lite.mmdb/dbip-asn-lite.mmdb, gitignored,
                re-download monthly for freshness per docs/geoip.md) —
                this is not a placeholder, it's the actual configured
                provider. Found and fixed one real gap the switch
                surfaced: DB-IP's free tier leaves subdivision IsoCode
                empty (GeoLite2 always has it) — regionOf now falls back
                to the English name instead of silently dropping region
                data that exists, covered by new unit tests.
                apps/internal/classifier/maxmind/testdata/ carries
                MaxMind's own public Apache-2.0 test fixtures (unrelated
                to DB-IP, kept for the unit tests below) — committed,
                small, no license key needed.
                MaxMind GeoLite2 remains available as an alternative
                (docs/geoip.md has both paths) if the user later creates
                an account themselves — not attempted further this
                session per explicit instruction to leave DB-IP as-is.
                Verified: go build/vet/test -count=1 ./... clean,
                including fixture-backed unit tests (known IP ->
                country/region/city and IP -> ASN, the "not in database"
                no-error case, and regionOf's ISO-code/name-fallback
                logic). Real end-to-end pass against the actual dev
                tracker using the real, currently-live DB-IP databases:
                hit a real tracking link via curl with X-Forwarded-For:
                8.8.8.8 through the real redirect path, confirmed the
                resulting click_events row in ClickHouse carried
                country=US/region=California/city=Mountain View/
                asn=AS15169, and confirmed the same row renders correctly
                in the Click Log page in a real browser session. Also
                separately verified graceful degradation with MaxMind's
                test fixtures: removed them, restarted, confirmed a clean
                warning log and normal startup, lookups falling back to
                empty.
LAST COMMIT   : feat(classifier): switch default Geo/ASN source to DB-IP
                Lite, no registration required — fc39384
UNCOMMITTED   : none of this phase's own work. Same pre-existing
                unrelated uncommitted work noted in the superseded block
                below is still untouched in the tree.
NEXT          : confirm scope before starting. Real candidates: (a) the
                pre-existing list from the superseded block below, still
                open; (b) if the user ever gets a MaxMind account, switch
                infra/geoip/'s two files and docker-compose's two env
                vars over — no code change needed, just a data swap.

```

</details>

<details>
<summary>Superseded — Click Log (kept for history, see above for current state)</summary>

```
CURRENT PHASE : CLICK LOG (user-scoped, post-Phase-34)
STATUS        : done — user asked whether FLOX captures per-click IP/geo;
                shown that click_events always has (IP/UserAgent/geo/
                device on apps/internal/event.Event), they asked to see
                it in the UI. No such screen existed — analytics/the
                report builder only ever expose aggregates, never
                row-level click_events reads. Added a read-only Click Log
                page: one row per SOURCE_CLICK/SOURCE_FILTER visit,
                org-wide, newest first, paginated — mirrors
                apps/internal/postbacklogs' read side (same 90-day range
                cap, same 50/200 default/max page size). New backend:
                chstore.EventStore.ListClickLog/CountClickLog
                (chstore/click_log.go), new apps/internal/clicklogs
                package (model/service/handler), mounted at GET
                /click-log. No writes/replay — a click row isn't
                retriable like a postback attempt. New frontend:
                features/clicklogs/* (click-log-view.tsx, click-log-
                columns.tsx), /click-log page under "Traffic" in the nav
                (after Campaigns). Every click_events column is
                reachable via the table's Columns menu; only a practical
                default subset (time/type/campaign/click id/country/
                city/device/OS/browser/IP) is visible on first load —
                DataTable gained a generic initialColumnVisibility prop
                for this (apps/web/src/components/ui/data-table.tsx).
                Campaign ids resolve to names via the existing
                useCampaigns() list; stream-set/flow ids stay raw (no
                org-wide stream-set listing exists, and resolving those
                would mean an N+1 fetch per campaign).
                Deliberately unchanged, called out on the page and in
                docs/click-log.md: geo/ASN are still NoopGeoProvider/
                NoopASNProvider (render "—" for real traffic — a real
                provider is separately-scoped work); click_events still
                has no retention policy, IP/UA stored raw and
                indefinitely (a real compliance decision for the user to
                make explicitly, not invented here).
                Verified: go build/vet/test -count=1 ./... clean,
                including a real-ClickHouse integration test
                (chstore/click_log_test.go — list/count, pagination,
                newest-first, IP/geo/device/subs round-trip, cross-org
                isolation per CLAUDE.md #5). tsc --noEmit/eslint clean.
                Real browser pass against the actual reporting user's
                own account: real rows render (real internal IPs,
                resolved campaign names), Columns menu correctly toggles
                every hidden field, no console errors.
LAST COMMIT   : feat(analytics): read-only Click Log page (IP/geo/device
                per visit) — 36003de
UNCOMMITTED   : none of this phase's own work. NOTE: the working tree
                also carries a large amount of pre-existing, unrelated
                uncommitted work this phase deliberately left untouched
                and did not commit — a Sheet-to-full-page conversion
                across several features (domains/landings/networks/
                offers/pixels/postlanding/pwa/traffic-sources: *-form-
                sheet.tsx deleted, new *-form.tsx/*-detail-view.tsx/
                new-*-view.tsx added) plus stream-set filter-picker
                changes and a new relative-time.tsx component. Not this
                session's work to explain or commit — flag to the user
                before touching any of those files.
NEXT          : confirm scope before starting. Real candidates: (a) a
                real Geo/ASN provider (e.g. MaxMind GeoLite2) behind the
                existing classifier.GeoProvider/ASNProvider interfaces —
                no go.mod dependency, config plumbing, or docker-compose
                .mmdb volume exists yet for this; (b) the pre-existing
                list from the superseded block below, still open.

```

</details>

<details>
<summary>Superseded — Campaign edit page layout (kept for history, see above for current state)</summary>

```
CURRENT PHASE : CAMPAIGN EDIT PAGE LAYOUT (user-scoped, post-Phase-34)
STATUS        : done — user shared a reference tracker's campaign edit
                screenshot (compact settings + tracking-link block left,
                wide Stream Sets panel right) and asked for the same
                block layout. campaign-detail-view.tsx's Overview tab
                restructured into a lg:grid-cols-[360px_1fr] two-column
                layout: CampaignForm (moved out of Settings) + TrackingLinks
                on the left, StreamSetList on the right; KPI stats/chart
                stay full-width below. Settings tab now holds only the
                danger zone. Added a real name-search box to StreamSetList
                (drag-to-reorder disabled while filtered, to avoid
                reordering a partial view by index).
                Follow-up in the same session: user clarified they
                specifically wanted Keitaro-style inline row editing, not
                a Sheet. StreamSetFormSheet deleted outright; its exact
                form (same fields/validation/sub-components, nothing about
                the editing logic changed) now lives in stream-set-
                editor.tsx, rendered inline under a row when expanded
                (StreamSetRow owns its own expand/collapse state, chevron
                + name click toggle it). "New Stream Set" now creates
                immediately via createStreamSet — no dialog — defaulting
                the new set's flow to the org's first offer/network
                (the backend rejects an empty offerId; the old Sheet flow
                never hit this since client-side Zod validation always
                blocked that submission first). Button disabled when the
                org has no offers yet, alongside the pre-existing
                no-networks case. Also wired the pre-existing but unused
                useDeleteStreamSet hook into the row menu with a confirm
                dialog, matching the reference's trash icon.
                Not done, real gap if ever wanted: per-row Уники/
                Депозиты/Доход columns — no ClickHouse column ties an
                event to a stream_set_id, only campaign_id/flow_id, so
                those numbers would have to be fabricated.
                Fixed a stale streamSets.json string claiming the
                fallback URL lived "in Settings" (en+ru).
LAST COMMIT   : feat(web): campaign edit page as a two-column layout,
                inline stream-set editing — a92c9b5
UNCOMMITTED   : none.
NEXT          : confirm scope before starting — same real candidates as
                before (see superseded block below), plus, if ever
                wanted: a stream_set_id column on click/tracking/
                conversion events so per-set stats could be real.

```

</details>

<details>
<summary>Superseded — Domains + Tracking Links (kept for history, see above for current state)</summary>

```
CURRENT PHASE : DOMAINS + TRACKING LINKS (user-scoped, post-Phase-34)
STATUS        : done — apps/internal/domain + apps/internal/trackinglink
                (new packages) are the real CRUD the domains/tracking_links
                tables (migration 00007) never had — apps/tracker's hot
                path already read them for real, but nothing let an
                operator populate them, and apps/web's Domains page was
                pure mock data. User reported creating a campaign and
                finding no way to tell it was active and no link to send
                traffic to. Two root causes, both fixed:
                (1) campaign-row-actions.tsx's row menu had no Activate
                action for a draft campaign (backend already supported
                it — frontend-only gap, now fixed).
                (2) No tracking-link UI anywhere. New POST /domains +
                GET/PATCH/DELETE /domains/{id}; new POST /campaigns/
                {campaignId}/tracking-links + PATCH/DELETE .../{id},
                returning a computed url (http://{domain}/t/{slug})
                matching apps/tracker's own /t/{trackingID} route. No
                registrar/DNS/SSL automation (none exists in this repo) —
                status/ssl/registrar/dnsProvider are operator-set fields,
                same honest shape as campaign.Status; the mock's "Verify
                ownership"/"Issue SSL" quick actions were dropped, not
                carried forward as fake automation.
                apps/web: features/domains/* switched to real API calls,
                lib/mock/domains.ts + stores/domains.ts deleted outright.
                New features/campaigns/tracking-links.tsx at the top of a
                campaign's Overview tab (domain picker, slug input, live
                URL preview, copy/pause/delete on existing links) — fully
                localized (new campaigns.json trackingLinks keys, en+ru),
                unlike the Domains page itself which stays English
                (pre-existing, separately-scoped gap, not touched here).
                Full design + the two tenant-isolation checks that
                actually matter in trackinglink.Repository.Create
                (domain_id from the request body, campaign_id from the
                route — neither re-validated against org elsewhere the
                way a plain List/Get already is) written up in
                docs/domains-tracking-links.md.
                Verified: go build/vet/test -count=1 ./... clean,
                including both new packages' own repository tests against
                real Postgres (cross-org domain conflict, cross-org
                domain/campaign rejection on tracking-link create,
                duplicate-slug conflict). Real browser pass against the
                actual reporting user's own account and real campaign
                (not a synthetic test org): added a real domain, created a
                tracking link from the Overview tab, and confirmed with a
                direct curl against the real containerized tracker that it
                302-redirects to the campaign's configured offer — the
                real click path, not a mocked-looking success screen.
                tsc --noEmit/eslint clean.
LAST COMMIT   : feat(domains): real Domains + Tracking Links CRUD, fix
                draft-campaign activation — aa25993
UNCOMMITTED   : none.
NEXT          : confirm scope before starting. Real candidates, not
                ranked: (a) the six remaining frontend-integration gaps —
                Domains' own UI still isn't localized (separate from
                today's fix, which only added the backend + the campaign-
                side tracking-links UI), Settings, Tags, Custom Metrics,
                Report Presets, Referral, Content Gallery; (b) a "distinct
                values for dimension X" endpoint for the analytics report
                builder's filter-value dropdown (docs/analytics-report-
                api.md); (c) campaign_id/flow_id -> display name
                resolution in Top Campaigns/Top Flows; (d) the DataTable
                column-resize/export gap; (e) repository/handler-layer
                unit test coverage; (f) real registrar/DNS/SSL automation
                for Domains, if ever wanted — today's fix deliberately
                left status/ssl as operator-declared, not automated.

```

</details>

<details>
<summary>Superseded — Phase 35 (kept for history, see above for current state)</summary>

```
CURRENT PHASE : PHASE 35 — REAL MULTI-DIMENSION /ANALYTICS BACKEND
STATUS        : done — apps/internal/analytics/report.go +
                apps/internal/chstore/report.go (Report/Timeseries/
                Breakdown/Funnel/Dashboard) replace the mock data the
                report builder (features/analytics/*) and /overview
                dashboard ran on since their Phase 14/14.5 frontend-first
                build. 11 of the report builder's 16 named dimensions are
                backed (5 — landing/pwa/postlanding/offer/network — have
                no column anywhere in the event pipeline yet, confirmed
                as later work, not faked). Cost joins only at campaign
                granularity or coarser (cost_entries is campaign+day
                grained in Postgres); every other dimension combination
                correctly returns null cost/profit/roi/roas/cpc/cpa,
                never a fabricated 0 (CLAUDE.md #6). New HTTP surface:
                POST /analytics/{report,timeseries,breakdown,funnel,
                dashboard}, JSON body. Frontend: hooks/use-analytics-
                report.ts + lib/api/analytics.ts additions replace
                aggregate.ts's client-side aggregateReport/
                aggregateTimeSeries/aggregateByDimension (deleted) and
                lib/mock/{analytics,dashboard}.ts's generators (deleted,
                lib/mock/dashboard.ts removed outright) — ReportTable's
                existing ReportRow/custom-metric-formula code needed no
                changes since API responses map onto that exact shape.
                No Top Offers card on /overview (offer isn't a backed
                dimension — dropped, not faked). Full design, dimension
                whitelist, and known-gaps writeup in
                docs/analytics-report-api.md; CHANGELOG's own Phase 35
                entry has the fix/verification detail.
                This phase also committed three other pieces of already-
                finished work found sitting uncommitted in the tree (see
                CHANGELOG for each): containerizing the dev stack (api/
                tracker/worker/web now run as containers with air hot-
                reload, dropping the last host-network-shaped thing in
                the repo), gating login behind email verification (real
                SMTP mailer, new /auth/verify-email + /auth/resend-
                verification), and closing the recurring ECharts/zrender
                animation crash for good (animation: false as the master
                kill switch) plus russifying /overview and the shared
                date-range picker, plus a small unrelated i18n pass on
                the analytics report builder screens.
                Found and fixed two real bugs: (1) apps/api/main.go
                didn't compile — analytics.NewService's second
                (CostSource) parameter was added in an interrupted
                earlier session but the call site was never updated to
                pass costSvc. (2) The already-written dashboard mapping
                this phase inherited had KpiGrid showing a fabricated $0
                for Spend/Profit instead of "—" whenever an org has zero
                cost_entries rows — PeriodMetrics.spend/.profit are now
                nullable end to end, rendering "—" like the ROI/CPA tiles
                already did.
                Verified: gofmt/go vet/go build/go test -count=1 ./...
                clean repo-wide, including chstore's report tests run
                against a REAL ClickHouse container (not mocked) —
                dimension grouping/filtering/by-day/cross-tenant
                isolation all pass. Full containerized dev stack brought
                up; signed up a real test org; curl'd all five new
                endpoints (empty-state, then non-empty after inserting
                real ClickHouse rows) — correct aggregation and null-cost
                cascade. Real browser pass (Claude-in-Chrome): /overview
                and all four /analytics report-builder tabs render real
                data with no console errors for both an empty org and the
                seeded one; a published custom metric correctly evaluates
                to "—" against a null-cost row. tsc --noEmit/eslint clean
                on apps/web.
LAST COMMIT   : feat(analytics): real multi-dimension /analytics backend
                (Phase 35) — cfe3390, preceded by 6aa98df feat(auth): gate
                login behind email verification and b3306ff feat(infra):
                containerize the dev stack, drop host networking
UNCOMMITTED   : none — this phase's commit(s) include everything that was
                previously sitting uncommitted (containerization, email
                verification, the zrender fix, dashboard/analytics i18n)
                alongside Phase 35's own work.
NEXT          : confirm scope before starting. Real candidates, not
                ranked: (a) the six remaining frontend-integration gaps —
                Domains, Settings, Tags, Custom Metrics, Report Presets,
                Referral, Content Gallery (Analytics/Overview's own gap
                is now closed by this phase); (b) a "distinct values for
                dimension X" endpoint for the report builder's filter-
                value dropdown (currently still a fixed mock pool, see
                docs/analytics-report-api.md); (c) campaign_id/flow_id ->
                display name resolution in Top Campaigns/Top Flows; (d)
                the DataTable column-resize/export gap; (e) repository/
                handler-layer unit test coverage.

```

</details>

<details>
<summary>Superseded — Phase 34 audit (kept for history, see above for current state)</summary>

```
CURRENT PHASE : PHASE 34 — FINAL AUDIT
STATUS        : done — §62's full checklist audited (UX/UI/accessibility/
                security/tenant isolation/performance/database/API/
                routing/attribution/analytics/LTV/testing/observability/
                documentation), plus the repo-wide TODO/FIXME/mock/dummy/
                temporary/hardcoded/console.log/debugger sweep. Zero
                TODO/FIXME/console.log/debugger found in source.
                Headline finding (not fixed this phase — real product-
                surface work, not an audit-phase action): seven whole
                frontend feature domains were built frontend-first
                (Phases 14/14.5-14.9) against apps/web/src/lib/mock/*.ts
                and never taken through a backend-integration phase —
                Domains, Settings (organization/API-keys/integrations
                panels), Tags, Custom Metrics, Report Presets, Referral,
                Content Gallery. Confirmed by absence: no matching
                apps/internal/* Go package, no apps/web/src/lib/api/*.ts
                client, no route in apps/api/main.go for any of the
                seven; their zustand stores import straight from lib/
                mock/. This is wider than the "missing tracking-URL
                generation API" gap Phase 32 flagged (that was only the
                Domains slice of it) — CURRENT STATE previously
                undersold the true scope. Concretely, CLAUDE.md
                non-negotiable #12 (custom-metric formula safety) is
                unenforceable today: no formula-evaluation engine exists
                anywhere, frontend or backend. Same root cause also
                explains the standalone Analytics report builder
                (`features/analytics/*`, dimension/funnel report UI) and
                the Overview dashboard (`(app)/overview`) still reading
                `lib/mock/analytics.ts`/`lib/mock/dashboard.ts`: the real
                `/analytics` backend only ever shipped two fixed
                campaign-scoped endpoints (daily clicks, daily revenue —
                genuinely wired via `hooks/use-campaign-analytics.ts`),
                never the flexible multi-dimension/funnel query capability
                that page's UI needs. Legitimately deferred, not a wiring
                bug — noted here so it doesn't get mis-filed as one later.
                Three real bugs were found and fixed (not just documented):
                (1) `campaign-cost-entries.tsx`'s delete action had zero
                confirmation step, violating CLAUDE.md's destructive-
                action rule — every sibling row-actions file (network,
                traffic source, postlanding) correctly gates delete behind
                a Dialog, this one didn't; now it does, with matching
                cost.json i18n keys (en+ru). (2) A stale doc comment in
                `apps/internal/routing/weighted.go` claimed the frontend
                had "a temporary simulator mock" mirroring the Go pick
                logic — no such file has existed for a while; the routing
                simulator has genuinely been a thin client over
                `/routing/simulate` all along (invariant #1 upheld in
                practice, the comment was just wrong). (3)
                `use-current-member.ts`'s comment claimed "no auth session
                yet (that's Phase 28)" — false, real `/auth/me` has
                existed since Phase 28; the actual reason this hook still
                reads the mock team roster (Custom Metrics/Referral/
                Content Gallery have no backend of their own yet, and
                switching identity source ahead of each feature's own
                integration phase would break their mock "is this mine"
                checks) is now stated accurately instead.
                Also removed as dead code (zero importers, confirmed by
                grep before deletion): `stores/{campaigns,offers,
                event-mappings}.ts` and `lib/mock/{offers,event-mappings,
                conversions}.ts` — all six pre-date the real API clients
                that replaced them and were never cleaned up.
                `lib/mock/campaigns.ts` was NOT deleted — `tag-
                assignments.ts` (the live, still-deferred Tags mock)
                still imports it for realistic fixture data.
                Everything else audited clean: routing single-source-of-
                truth (routing simulator only calls /routing/simulate, no
                TS reimplementation), deterministic weighted pick +
                sticky-cookie-is-truth (#4, §38), tenant isolation (all 19
                route groups in apps/api/main.go sit behind
                tenantMiddleware; repository-layer spot checks across 13+
                domains all filter on organization_id; the only two
                queries missing an explicit org_id filter are in
                auth/repository.go's AcceptInvite, which is correctly
                scoped upstream by a verified invite token instead),
                CSRF/secrets/postback-dedup (#3 — event_ref enforced in
                code, not just convention), no hardcoded secrets, .env
                correctly gitignored, tracker hot path (#9 — Handler.track
                never blocks on persistence or analytics; eventbuf.
                Writer.Enqueue is a non-blocking channel send), DB indexes
                on tenant-scoped hot tables, ClickHouse sort keys lead
                with organization_id, LTV/cohort currency handling (#7 —
                FX rate looked up at the event's OccurredAt, never
                time.Now()), OpenTelemetry tracing genuinely wired (real
                OTLP exporter + otelhttp instrumentation on all 3
                binaries, not just imported), no sensitive data (passwords/
                tokens/secrets) in any log call site, infra/alerts.yml's 8
                rules cross-checked against actual metric names (all
                resolve), go build/vet/test -count=1 ./... and tsc/eslint
                clean.
                Fixed: apps/web/src/app/globals.css gained the one global
                `@media (prefers-reduced-motion: reduce)` rule (§66) —
                previously absent entirely, so no component respected
                the OS setting. docs/deployment.md gained a Connection
                pool sizing section documenting that none of the four Go
                binaries set explicit Postgres/Redis pool limits
                (pgxpool.New/redis.Options both ride library defaults) —
                not a bug (Phase 31's real load test measured p95
                1.0-1.5ms against the 50ms budget with defaults as-is),
                but a previously-undocumented production tuning knob;
                documented the DATABASE_URL?pool_max_conns= path.
                Known gaps documented, not fixed this phase (real scoped
                work, not audit-phase actions): (a) the seven-domain
                frontend-integration gap above; (b) the standalone
                Analytics report builder + Overview dashboard's missing
                multi-dimension backend, above; (c) the shared DataTable
                component (`components/ui/data-table.tsx`) never got
                column resizing or CSV/export wired despite both being in
                CLAUDE.md's own UX-floor table rules (§64) — sorting,
                pagination, column visibility, search/filter are all
                real and used consistently, resizing/export are not, and
                this is repo-wide (every table), not per-table; (d)
                repository/handler-layer Go packages (network, offer,
                pixel, campaign, streamset, cost, and ~18 others — see
                `go test -cover` output) sit at 0.0% unit-test coverage —
                DB-backed logic is exercised only via the Phase 32 e2e
                scenario and manual dev-stack verification. Core
                computation packages are well covered (routing 81.7%,
                ltv 67.0%, attribution/event/macro/metrics 100%).
                Verified: gofmt -l/go vet/go build/go test -count=1
                ./... clean repo-wide; tsc --noEmit/eslint clean on
                apps/web.
LAST COMMIT   : fix(audit): close Phase 34 findings — cost-entry confirm dialog, dead code, stale comments
UNCOMMITTED   : between-phases infra work (user-scoped, see CHANGELOG's
                "[Between phases] — containerize the dev stack, drop host
                networking"): infra/docker-compose.dev.yml now runs
                api/tracker/worker/web themselves as containers (each
                Go service's Dockerfile gained a `dev` target — air
                hot-reload over a bind-mounted apps/ tree — web reuses
                its existing `deps` stage with `npm run dev`), not just
                the data stores. Removed the last host-network-shaped
                thing in the repo: prometheus's `extra_hosts:
                host-gateway` + `host.docker.internal` scrape targets,
                now `api:8080`/`tracker:8081`/`worker:8082` by compose
                service name. Root Makefile rewritten around `docker
                compose` instead of host `go run`/`npm run dev` + PID
                files. Verified end-to-end: full stack up, all 9
                containers healthy, all 4 app containers'
                `NetworkMode` is the compose bridge network (not
                `host`), hot reload confirmed live.
                Second, later batch of uncommitted work, same session
                (see CHANGELOG's "[Between phases] — gate login behind
                email verification"): registration used to auto-login
                immediately with no proof of email ownership — user-
                reported as an incomplete registration flow. Signup no
                longer creates a session; it emails a real verification
                link instead (new apps/internal/mailer, a real net/smtp
                client — the SMTP_URL/mailhog infra from the first batch
                above was staged exactly for this), and Login now 403s
                unverified accounts with a distinct email_not_verified
                code. New migration 00022 (users.email_verified_at +
                verification token columns, existing rows grandfathered
                verified). New POST /auth/verify-email (auto-logs in,
                same AcceptInvite-style token consumption) and POST
                /auth/resend-verification (silent/anti-enumeration).
                AcceptInvite also marks accounts verified (clicking an
                invite link is equally strong proof of inbox access) —
                without this, invited teammates would be permanently
                locked out by Login's new check. apps/web: new "check
                your email" post-signup panel, new /verify-email page,
                login's new resend-on-unverified action. 8 new Go tests;
                existing 21-test suite needed zero changes (signupOrg
                helper's signature/return shape unchanged, now routes
                through a spy Mailer + the real verify step internally).
                Found and fixed two real bugs along the way: a stuck-
                loading React bug in the new verify-email page (useQuery
                fixed what useMutation-fired-from-an-effect couldn't —
                full writeup in docs/auth.md) and leftover host-run
                api/tracker/worker/web processes (from before the first
                batch's containerization) blocking the new containers'
                ports. Full curl + real-browser verification, all
                documented in docs/auth.md. Not yet committed — waiting
                on the user re: how to handle the other unrelated
                uncommitted work already in the tree (analytics-report
                i18n) before commit.
NEXT          : confirm scope before starting. Real candidates, not
                ranked: (a) the seven-domain frontend-integration gap
                above (Domains, Settings, Tags, Custom Metrics, Report
                Presets, Referral, Content Gallery) — likely several
                phases' worth, probably slice-by-slice like the earlier
                domain-integration phases (Traffic Sources, Networks/
                Offers, Stream Sets) rather than one giant phase; (b) a
                real multi-dimension /analytics backend for the report
                builder + dashboard, replacing their mock data; (c) the
                DataTable column-resize/export gap; (d) repository/
                handler-layer unit test coverage. Per §9's fixed phase
                order, Phase 34 was the last listed phase — anything past
                it is v3-scope/hardening work the user should scope
                explicitly, not an assumed default.

```

</details>

> At the end of every phase: update the four lines above, add a CHANGELOG entry,
> commit, then STOP and wait. Do not roll into the next phase automatically.

---

## THE ONE RULE

**Work strictly one phase at a time.** Never build ahead. Never generate large
code before validating the current phase. Every phase follows:

```
Inspect → Plan → Implement → Run → Test → Fix → Review → Document → STOP
```

Only advance when the current phase meets its Definition of Done (§79).
Phase order is fixed — see §9. Do not reorder or skip.

---

## PHASE PROTOCOL (per §2)

Open each phase with:

```
PHASE X — NAME — GOAL
What will be built
Files created/changed
Dependencies
Acceptance criteria
```

Close each phase with:

```
COMPLETED / TESTS / KNOWN ISSUES / FILES CHANGED / NEXT PHASE
```

Run typecheck, lint, unit tests, and build where applicable before reporting
complete. Never skip validation. Do not ask permission to continue unless a real
architectural blocker exists — but never make silent destructive changes.

---

## STACK (do not swap without asking)

**Frontend:** Next.js · React · TypeScript · Tailwind · shadcn/ui · Radix ·
TanStack Query/Table · React Hook Form · Zod · Zustand · Lucide · date-fns ·
Apache ECharts.

**Backend:** Go · chi · pgx · sqlc (where sensible) · goose migrations ·
slog/zerolog · OpenTelemetry.

**Data:** PostgreSQL (control plane) · ClickHouse (high-volume events + analytics)
· Redis (cache, rate limits, sticky CACHE only, postback dedup) · S3-compatible
object storage.

**Architecture:** modular monolith. `apps/tracker` and `apps/worker` are separate
binaries inside the same Go module, sharing `internal/routing` and
`internal/classifier`. No microservices up front. No unnecessary frameworks.

---

## REPO LAYOUT

```
apps/
  web/       Next.js frontend (the only non-Go app)
  api/       Go control-plane API
  tracker/   Go hot-path click/redirect service
  worker/    Go async event/postback processor
packages/
  ui/ config/ types/
docs/        architecture, routing, event-model, ltv, metrics, etc. (§76)
infra/       docker-compose, deployment
FLOX-master-prompt-v3.md   ← full spec
CLAUDE.md                  ← this file
```

Frontend internal structure: `app/ components/ features/ hooks/ lib/ stores/
types/ schemas/`. Domain code lives in `features/`. All server calls go through
`src/lib/api` — never scatter `fetch()` in components (§32, §70).

Go structure per module: `handler → service → repository → domain`. Handlers are
thin; business logic never lives in handlers or in React components (§34, §71).

---

## NON-NEGOTIABLE INVARIANTS

Read the referenced section before touching related code.

1. **Single source of truth for routing (§6-SHARED, §26).** Routing/filter/sticky
   /metric decision logic exists ONCE (in Go). The routing simulator is a thin UI
   over the `/routing/simulate` contract — NOT a second TS implementation. Both
   sides pass one shared conformance fixture.

2. **Full event model (§43).** ~20 event types incl. CPA statuses
   HOLD/ACCEPT/REDEP/DECLINE/TRASH as an enum. Design the ClickHouse schema for
   all of them from day one — adding types later is a live-data migration. Never
   collapse conversions into one "conversion" type.

3. **Postback dedup key = (click_id, status, event_ref)** (§45) — NOT click_id
   alone, and NOT (click_id, status). `event_ref` = network txn id for CPA_REDEP
   (the only repeatable status), empty string for every other status even if a
   txn id was sent. **Status never goes back to CPA_HOLD** — nightly partner
   replays re-send it after approval and would take revenue out of a closed
   report (§45 STATUS PROGRESSION). Long Redis TTL + durable DB unique
   constraint. Store original currency + USD value at event time.
   `acceptDuplicates` override per network — it does not bypass the
   progression rule.

4. **Sticky = cookie is truth, Redis is cache only (§39-STICKY).** Cookie
   `sf_{campaignId}` = `setId:flowId[:clickId]`. Redis-only sticky is forbidden
   (silently corrupts A/B tests).

5. **Tenant isolation (§36-TENANCY).** Every tenant-scoped table has
   `organization_id`; every query filters on it; enforcement is in the repository
   layer. `organization_id` comes from the session/API key, never the request
   body. ClickHouse sort keys lead with it. Cross-tenant isolation test is part of
   DoD for API/data phases.

6. **Cost or it doesn't exist (§27-COST).** Profit/ROI/ROAS need spend. Manual
   cost entry first; FB/TikTok import later. No cost for a slice → show ROI as
   "—", never compute against zero.

7. **Currency at event time (§50-FX).** Normalize to USD using the rate on the
   event date, never the current rate. Store both original and USD.

8. **Regex = RE2 only (§5, §22).** Go stdlib `regexp`. Validate user patterns at
   save time, never on the hot path. No PCRE libs.

9. **Redirect hot path stays fast (§41, §56).** tracking p50 < 20ms / p95 < 50ms
   (excl. third-party latency). Persist events async (buffered batch → queue).
   Never run analytics queries or block on outbound partner calls in the redirect.

10. **WebView bounce ≠ moderator cloaking (§73).** Bouncing in-app WebView
    (FB/IG/TikTok/Telegram) to the external browser so the PWA install prompt can
    fire is a REQUIRED, provider-neutral capability. Vendor-specific moderator
    detection is FORBIDDEN. Keep them separate.

11. **Providers behind interfaces (§74–75).** Geo, ASN, bot, cost, conversion,
    pixel, registrar, DNS, FX — all swappable. No vendor lock-in in core logic.

12. **Custom metrics build on the registry, safely (§30.5, §50).** User formulas
    reference registry metrics by stable id; division-by-zero yields empty, never
    an error; one formula = one data source (no mixing push with regular); LTV not
    allowed in formulas; team-private.

---

## EVENT MODEL QUICK REF (authoritative list in §43)

```
SOURCE_CLICK, SOURCE_FILTER
LAND_VIEW, LAND_CLICK, POSTLANDING_VIEW, POSTLANDING_CLICK
PWA_VIEW, PWA_OPEN, PWA_INSTALL, IOS_INSTALL
NOTIFICATION_REQUEST/SUBSCRIBE/DECLINE/UNSUBSCRIBE/CLICK
TG_JOIN, TG_START
CPA_HOLD, CPA_ACCEPT, CPA_REDEP, CPA_DECLINE, CPA_TRASH
```

Canonical funnel: `SOURCE_CLICK → LAND_VIEW → LAND_CLICK → PWA_VIEW →
PWA_INSTALL → CPA_HOLD → CPA_ACCEPT → CPA_REDEP`. Chain linked by one click_id.

---

## ROUTING QUICK REF (full detail §21, §39)

```
Campaign → Stream Set (priority, first match wins, AND/OR filters, pixels)
        → Flow (weighted, pickWeighted) → destination
No set matches → campaign fallback / safe destination.
```

`pickWeighted` is **deterministic by visit key** (§38): an unseeded FNV-1a hash
of a stable property of the visit, never an RNG. Same visit → same flow on every
replica and after every restart. Independent of sticky — the cookie is still the
truth for a returning visitor (#4); the hash covers what happens before a cookie
exists. Candidates are filtered *before* the draw, so weights mean what the
operator typed.

Decisions must be explainable: why matched / why not / why this flow / why
fallback / sticky applied from where (§72).

---

## BUILD ORDER (frontend-first, §9 / §83)

Design & UI on mock contracts first (phases 2–15), then Go backend & engines
(16+), then integration (27), then LTV (26.5), auth/RBAC (28), hardening.
"Frontend-first" means UI/UX first on mock CONTRACTS — it does NOT license a
parallel routing engine in TS.

New in v3 (secondary, after core): Tags (14.5), Custom Metrics (14.6), Report
Presets + directory stats (14.7), Referral (14.8), Content Gallery (14.9).

Core workflows to prioritize (§84): Create Campaign → Stream Set → Filters →
Flow → Simulate Routing → Analytics → Track Conversion (HOLD→ACCEPT→REDEP) →
Postback → LTV.

---

## UX FLOOR (every screen, §63–68)

loading / empty / error / success states on everything. Destructive actions
confirm. Forms validate with inline errors + disabled submit + success feedback.
Tables: sort, paginate, column visibility/resize, search, filter, export;
virtualize large sets. Dark-first, dense, tabular numerals, meaningful status
colors — a trading-terminal control plane, not a generic admin dashboard.
WCAG 2.2 AA. Animation sparing (150–250ms ease-out).

---

## DEFINITION OF DONE (§79)

```
✓ implemented        ✓ no TS errors      ✓ no Go build errors
✓ lint passes        ✓ tests pass        ✓ build passes
✓ UX reviewed        ✓ responsive        ✓ loading/empty/error states
✓ tenant isolation verified (API/data phases)
✓ docs updated       ✓ CHANGELOG entry   ✓ CLAUDE.md CURRENT STATE updated
✓ logical commit (§78)
```

---

## NEVER (§80)

Build all phases at once · replace repo without inspection · duplicate business
logic (esp. a 2nd routing impl in TS) · hardcode secrets · commit `.env` · skip
tests/validation · ignore TS/Go errors · fake APIs that look real · hide errors ·
dedup on click_id alone or on (click_id, status) alone · record a postback that
moves a conversion back to CPA_HOLD · treat missing cost as zero · Redis-only sticky ·
truncate the event model · leak data across orgs · embed vendor moderator
detection · divide-by-zero into an error in custom metrics · mix push+regular
metrics in one formula · reimplement tags per entity · treat empty FB subs as
"unknown campaign".

Mocks are allowed only in frontend-first phases and must implement the same
contract as the real backend, and be explicitly replaceable.

---

## COMMITS (§78)

Logical, per-phase commits. Examples:
`feat(ui): design system` · `feat(routing): filter builder` ·
`feat(routing): sticky engine` · `feat(ltv): cohort engine` ·
`feat(api): campaign endpoints` · `feat(tracker): tracking endpoint`.
No single giant commit.

---

## SESSION START CHECKLIST

1. Read CURRENT STATE above.
2. Read the relevant §sections in `FLOX-master-prompt-v3.md` for this phase.
3. Confirm the phase plan (§2) before writing code.
4. Implement → validate → document → update CURRENT STATE → commit → STOP.

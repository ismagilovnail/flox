"use client";

import { useState } from "react";
import {
  ArrowRightIcon,
  BookOpenIcon,
  BookmarkIcon,
  GamepadIcon,
  HelpCircleIcon,
  InfoIcon,
  LayoutGridIcon,
  SearchIcon,
  Share2Icon,
  SmartphoneIcon,
  SmileIcon,
  StarIcon,
  XIcon,
} from "lucide-react";
import type { TFunction } from "i18next";
import useEmblaCarousel from "embla-carousel-react";

import { cn } from "@/lib/utils";

/**
 * Color tokens copied verbatim from the reference tracker's own
 * `:root` custom properties (pasted by the user, not fetched from their
 * cdn.adset.pro script — see pwa-form.tsx's note on why the loader
 * itself was declined). Kept as named constants rather than inlined
 * Tailwind classes so the source of each hex is traceable back to the
 * reference's `--color-*` variable it stands in for.
 */
const REF_PRIMARY = "#00A173";
const REF_VERIFIED_BADGE_BLUE = "#4285f4";
const REF_BORDER = "#dadce0";

const STAR_ROWS = [5, 4, 3, 2, 1] as const;

/** The real multicolor Google Play triangle mark from the reference markup — an inline vector, not a fetched asset. */
function GooglePlayGlyph() {
  return (
    <svg
      className="size-6"
      viewBox="0 0 40 40"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <path
        d="M19.7,19.2L4.3,35.3c0,0,0,0,0,0c0.5,1.7,2.1,3,4,3c0.8,0,1.5-0.2,2.1-0.6l0,0l17.4-9.9L19.7,19.2z"
        fill="#EA4335"
      />
      <path
        d="M35.3,16.4L35.3,16.4l-7.5-4.3l-8.4,7.4l8.5,8.3l7.5-4.2c1.3-0.7,2.2-2.1,2.2-3.6C37.5,18.5,36.6,17.1,35.3,16.4z"
        fill="#FBBC04"
      />
      <path
        d="M4.3,4.7C4.2,5,4.2,5.4,4.2,5.8v28.5c0,0.4,0,0.7,0.1,1.1l16-15.7L4.3,4.7z"
        fill="#4285F4"
      />
      <path
        d="M19.8,20l8-7.9L10.5,2.3C9.9,1.9,9.1,1.7,8.3,1.7c-1.9,0-3.6,1.3-4,3c0,0,0,0,0,0L19.8,20z"
        fill="#34A853"
      />
    </svg>
  );
}

/** The reference's verified-developer checkmark badge, same blue regardless of theme. */
function VerifiedBadge() {
  return (
    <svg
      className="size-4 shrink-0"
      viewBox="0 0 512 512"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <path
        d="M256 472.153L176.892 512l-41.725-81.129-86.275-16.654 11.596-91.422L0 256l60.488-66.795-11.596-91.422 86.275-16.654L176.892 0 256 39.847 335.108 0l41.725 81.129 86.275 16.654-11.596 91.422L512 256l-60.488 66.795 11.596 91.422-86.275 16.654L335.108 512z"
        fill={REF_VERIFIED_BADGE_BLUE}
      />
      <path
        d="M211.824 284.5L171 243.678l-36 36 40.824 40.824-.063.062 36 36 .063-.062.062.062 36-36-.062-.062L376.324 192l-36-36z"
        fill="#fff"
      />
    </svg>
  );
}

/** The reference's own download-size glyph (an arrow into a framed box), not a disk-drive icon. */
function SizeGlyph() {
  return (
    <svg
      className="size-[18px] text-slate-900"
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <path
        d="M6 18L18 18"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M12 6V14M12 14L15.5 10.5M12 14L8.5 10.5"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M3 20.4V3.6C3 3.26863 3.26863 3 3.6 3H20.4C20.7314 3 21 3.26863 21 3.6V20.4C21 20.7314 20.7314 21 20.4 21H3.6C3.26863 21 3 20.7314 3 20.4Z"
        stroke="currentColor"
        strokeWidth="1.5"
      />
    </svg>
  );
}

/** One star of the review-summary row, filled left-to-right by `fill` (0–1) the way the reference clips its own star SVG. */
function PartialStar({ fill }: { fill: number }) {
  return (
    <span className="relative inline-flex size-4">
      <StarIcon className="size-4 fill-slate-200 text-slate-200" />
      <span
        className="absolute inset-y-0 left-0 overflow-hidden"
        style={{ width: `${Math.round(fill * 100)}%` }}
      >
        <StarIcon
          className="size-4"
          style={{ fill: REF_PRIMARY, color: REF_PRIMARY }}
        />
      </span>
    </span>
  );
}

/**
 * The reference's own screenshot gallery is built on Embla Carousel (its
 * rendered markup — pasted by the user — is literally `.embla`/
 * `.embla__viewport`/`.embla__container`/`.embla__slide` with a
 * `transform: translate3d(...)` driving the slide, not a native
 * `overflow-x: scroll`). Built on the same real, open-source library
 * (`embla-carousel-react`, MIT, widely used — also what shadcn/ui's own
 * carousel component wraps) rather than reimplementing drag/momentum/
 * snap by hand — matches the reference's actual technique, not just its
 * look. `dragFree: true` lets the drag travel any distance instead of
 * hard-snapping to one slide at a time, matching how the reference's own
 * mixed-width (portrait + landscape) screenshots actually feel to drag.
 */
function ScreenshotsCarousel({ screenshots, t }: { screenshots: string[]; t: TFunction }) {
  const [emblaRef] = useEmblaCarousel({ dragFree: true, containScroll: "trimSnaps" });
  return (
    // The px-4 pb-5 inset lives on this plain outer div, not the Embla
    // viewport below — border-box sizing means padding on the *same*
    // element as an explicit height eats into it ("h-[204px] pb-5"
    // together rendered images at 184px, not 204 — caught live, not in
    // review). Keeping the viewport unpadded keeps its own h-[204px]
    // exact; the outer div needs no squish-guard itself since it has no
    // overflow of its own (default `visible`), so its automatic minimum
    // size stays content-based, which already includes the protected
    // child below.
    <div className="px-4 pb-5">
      <div
        ref={emblaRef}
        className="h-[204px] shrink-0 overflow-hidden"
        // h-[204px] shrink-0 are load-bearing, not decorative: this sits
        // inside pwa-preview-card's flex-col card (capped at max-h-[70vh],
        // overflow-y-auto), and any flex item with non-visible overflow
        // on any axis (Embla sets overflow-hidden on its own viewport
        // element) has its *automatic minimum size* collapse to 0 per the
        // CSS flexbox/overflow spec — without an explicit height, a card
        // whose total content exceeds 70vh would let the flex layout
        // crush this row toward zero instead of leaving it to the outer
        // scrollbar, exactly the "screenshots render squished" bug a real
        // user hit once already here.
      >
        <div className="flex h-full gap-3">
          {screenshots.map((src, i) => (
            <div key={`${src}-${i}`} className="h-full shrink-0">
              {/* eslint-disable-next-line @next/next/no-img-element -- external, operator-supplied screenshot URL */}
              <img
                src={src}
                alt={t("preview.screenshotAlt", { ns: "pwa", n: i + 1 })}
                loading="lazy"
                // h-full w-auto (not a fixed aspect-ratio box): matches
                // the real Google Play gallery's own technique — verified
                // live against play.google.com, whose screenshot cards
                // share one fixed height while each image's width follows
                // its own natural aspect ratio (177px-wide portraits next
                // to 494px-wide landscape shots, all exactly 296px tall).
                className="h-full w-auto rounded-xl object-cover"
                onError={(e) => (e.currentTarget.style.display = "none")}
              />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function StarBar({ n, pct }: { n: number; pct?: number }) {
  const hasValue = typeof pct === "number" && !Number.isNaN(pct);
  return (
    <div className="flex items-center gap-2 text-xs text-slate-600">
      <span className="w-2 text-right">{n}</span>
      <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-slate-100">
        <div
          className="h-full rounded-full"
          style={{
            width: `${hasValue ? Math.min(100, Math.max(0, pct)) : 0}%`,
            backgroundColor: REF_PRIMARY,
          }}
        />
      </div>
      {!hasValue && <span className="w-4 text-right">—</span>}
    </div>
  );
}

/**
 * A live Google-Play-listing mockup — deliberately light/white regardless
 * of host theme (it's simulating a screenshot of a different app, the
 * same reason a browser-preview mockup would render a light webpage even
 * inside a dark host UI). Every data field renders exactly what the
 * operator entered in PwaForm; nothing here is a fabricated number
 * (CLAUDE.md #6) — rating/reviews/downloads/size/breakdown all fall back
 * to "—" or an omitted section when unset, never a fake placeholder
 * value, and an unset icon renders no icon at all rather than an
 * invented letter avatar (the reference behaves the same way). Footer
 * links and the bottom nav bar are pure decoration (match the real
 * Google Play chrome, go nowhere) — not backed by any field.
 */
export function PwaPreviewCard({
  t,
  name,
  developer,
  iconUrl,
  description,
  rating,
  ratingCount,
  downloadsLabel,
  sizeMb,
  screenshots,
  whatsNew,
  ratingStars5,
  ratingStars4,
  ratingStars3,
  ratingStars2,
  ratingStars1,
  version,
  ageRating,
  category,
  interactiveElements,
  containsAds,
  inAppPurchases,
  updatedDate,
  dir = "ltr",
  className,
}: {
  t: TFunction;
  name?: string;
  developer?: string;
  iconUrl?: string;
  description?: string;
  rating?: number;
  ratingCount?: number;
  downloadsLabel?: string;
  sizeMb?: number;
  screenshots?: string[];
  whatsNew?: string;
  ratingStars5?: number;
  ratingStars4?: number;
  ratingStars3?: number;
  ratingStars2?: number;
  ratingStars1?: number;
  /** Detail-sheet fields, shown in the "About this app" sheet's
   * "detailed information" table. A row whose field is empty is left
   * out entirely rather than printed with a placeholder value. */
  version?: string;
  ageRating?: string;
  category?: string;
  interactiveElements?: string;
  /** Ads/purchases replace what was a hardcoded "no ads or paid
   * content" line — an assertion the preview used to make about every
   * PWA with nothing behind it. */
  containsAds?: boolean;
  inAppPurchases?: boolean;
  /** The listing's own "updated on" date (YYYY-MM-DD). Empty hides the
   * row: the preview used to print today's date instead, which claimed
   * every PWA had just been updated. */
  updatedDate?: string;
  /** "rtl" mirrors layout direction only (the reference's own "Arabic
   * text" toggle) — no machine translation, matching the user's explicit
   * "our own code, not their Google Translate widget" instruction. */
  dir?: "ltr" | "rtl";
  className?: string;
}) {
  const hasRating = typeof rating === "number" && !Number.isNaN(rating);
  const ratingText = hasRating
    ? new Intl.NumberFormat(undefined, {
        minimumFractionDigits: 1,
        maximumFractionDigits: 1,
      }).format(rating!)
    : "—";
  const displayName =
    name?.trim() || t("preview.placeholderName", { ns: "pwa" });
  const displayDeveloper =
    developer?.trim() || t("preview.placeholderDeveloper", { ns: "pwa" });
  const reviewCountText = ratingCount
    ? t("preview.reviewsCount", { ns: "pwa", count: ratingCount })
    : t("preview.noData", { ns: "pwa" });
  const starPct: Record<(typeof STAR_ROWS)[number], number | undefined> = {
    5: ratingStars5,
    4: ratingStars4,
    3: ratingStars3,
    2: ratingStars2,
    1: ratingStars1,
  };
  // Formatted from the operator's own date. Never falls back to today:
  // an unset date leaves the row (and the card's whole "updated on"
  // block) out instead of claiming a fresh update, which is what this
  // used to do for every PWA.
  const parsedUpdatedDate = updatedDate?.trim() ? new Date(`${updatedDate}T00:00:00`) : undefined;
  const updatedDateText =
    parsedUpdatedDate && !Number.isNaN(parsedUpdatedDate.getTime())
      ? new Intl.DateTimeFormat(undefined, { dateStyle: "long" }).format(parsedUpdatedDate)
      : "";
  // The reference prints these under the developer name, and again in
  // the developer cell of the detail sheet.
  const monetizationNotes = [
    containsAds ? t("preview.containsAds", { ns: "pwa" }) : "",
    inAppPurchases ? t("preview.inAppPurchases", { ns: "pwa" }) : "",
  ].filter(Boolean);
  const [aboutOpen, setAboutOpen] = useState(false);

  // The sheet's "detailed information" table, in the reference's own
  // order. A row with no value is dropped, not padded with "—": unlike
  // the rating/size stats (where the dash carries meaning next to the
  // numbers beside it), a whole missing row here has nothing to say.
  const detailRows = [
    {
      label: t("preview.detailUpdated", { ns: "pwa" }),
      value: updatedDateText,
    },
    {
      label: t("preview.detailSize", { ns: "pwa" }),
      value:
        typeof sizeMb === "number" && !Number.isNaN(sizeMb)
          ? t("preview.sizeMb", { ns: "pwa", size: sizeMb })
          : "",
    },
    {
      label: t("preview.detailDownloads", { ns: "pwa" }),
      value: downloadsLabel?.trim() ?? "",
    },
    {
      label: t("preview.detailVersion", { ns: "pwa" }),
      value: version?.trim() ?? "",
    },
    {
      label: t("preview.detailAge", { ns: "pwa" }),
      value: ageRating?.trim() ?? "",
    },
    {
      label: t("preview.detailDeveloper", { ns: "pwa" }),
      value: developer?.trim() ?? "",
      notes: monetizationNotes,
    },
    {
      label: t("preview.detailCategory", { ns: "pwa" }),
      value: category?.trim() ?? "",
    },
    {
      label: t("preview.detailInteractive", { ns: "pwa" }),
      value: interactiveElements?.trim() ?? "",
    },
  ].filter((row) => row.value !== "");

  return (
    <div
      dir={dir}
      className={cn(
        "relative mx-auto flex w-[390px] max-w-full flex-col overflow-hidden rounded-[2.5rem] border-[12px] border-neutral-900 bg-white text-slate-900 shadow-xl dark:border-neutral-800",
        className,
      )}
      // Matches the reference listing's own font stack minus "Google
      // Sans" itself — that name isn't actually served by Google Fonts
      // (it's Google's internal-only typeface), so their real rendered
      // font is this exact system-font fallback chain already. Using it
      // directly needs no external font request at all.
      style={{
        fontFamily:
          '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
      }}
    >
      <div className="flex max-h-[70vh] flex-col overflow-y-auto">
        <div
          className="sticky top-0 z-10 flex items-center justify-between border-b bg-white px-4 py-3"
          style={{ borderColor: REF_BORDER }}
        >
          <div className="flex items-center gap-2 text-lg font-medium text-slate-900">
            <GooglePlayGlyph />
            Google Play
          </div>
          <div className="flex items-center gap-4 text-slate-900">
            <SearchIcon className="size-6" />
            <HelpCircleIcon className="size-6" />
          </div>
        </div>

        <div className="flex flex-col gap-4 px-5 py-5">
          <div className="flex items-start gap-4">
            {!!iconUrl && (
              <div className="size-[72px] shrink-0 overflow-hidden rounded-2xl">
                {/* eslint-disable-next-line @next/next/no-img-element -- external, unknown-dimension icon URL; next/image would need a remotePatterns entry per operator-supplied host */}
                <img
                  src={iconUrl}
                  alt=""
                  className="size-full object-cover"
                  onError={(e) => (e.currentTarget.style.display = "none")}
                />
              </div>
            )}
            <div className="flex min-w-0 flex-col gap-0.5">
              <p className="text-[22px] font-semibold leading-tight text-slate-900">
                {displayName}
              </p>
              <p
                className="flex items-center gap-1.5 text-base font-medium"
                style={{ color: REF_PRIMARY }}
              >
                {displayDeveloper}
                <VerifiedBadge />
              </p>
              <p className="text-xs text-slate-600">
                {monetizationNotes.length > 0
                  ? monetizationNotes.join(" · ")
                  : t("preview.adFree", { ns: "pwa" })}
              </p>
            </div>
          </div>

          <div className="flex items-center justify-around text-center">
            <div className="flex flex-col items-center gap-1">
              <span className="flex items-center gap-1 text-base font-semibold text-slate-900">
                {ratingText}
                <StarIcon
                  className="size-3.5"
                  style={{ fill: REF_PRIMARY, color: REF_PRIMARY }}
                />
              </span>
              <span className="flex items-center gap-1 text-xs text-slate-600">
                {reviewCountText}
                <InfoIcon className="size-3 shrink-0" />
              </span>
            </div>
            <div className="h-6 w-px" style={{ backgroundColor: REF_BORDER }} />
            <div className="flex flex-col items-center gap-1">
              <span className="text-base font-semibold text-slate-900">
                {downloadsLabel?.trim() || "—"}
              </span>
              <span className="text-xs text-slate-600">
                {t("preview.downloadsLabel", { ns: "pwa" })}
              </span>
            </div>
            <div className="h-6 w-px" style={{ backgroundColor: REF_BORDER }} />
            <div className="flex flex-col items-center gap-1">
              <SizeGlyph />
              <span className="text-xs text-slate-900">
                {typeof sizeMb === "number" && !Number.isNaN(sizeMb)
                  ? t("preview.sizeMb", { ns: "pwa", size: sizeMb })
                  : "—"}
              </span>
            </div>
          </div>

          <button
            type="button"
            disabled
            className="h-9 w-full rounded-lg text-sm font-bold text-white disabled:opacity-100"
            style={{ backgroundColor: REF_PRIMARY }}
          >
            {t("preview.install", { ns: "pwa" })}
          </button>

          <div
            className="flex items-start justify-between gap-4 text-[13px] font-medium"
            style={{ color: REF_PRIMARY }}
          >
            <span className="flex items-center gap-2">
              <Share2Icon className="size-5 shrink-0" />{" "}
              {t("preview.share", { ns: "pwa" })}
            </span>
            <span className="flex items-center gap-2 text-right">
              <BookmarkIcon className="size-5 shrink-0" />{" "}
              {t("preview.wishlist", { ns: "pwa" })}
            </span>
          </div>

          <div className="flex items-center gap-2 text-xs text-slate-600">
            <SmartphoneIcon className="size-[18px] shrink-0" />
            {t("preview.availableOnDevices", { ns: "pwa" })}
          </div>
        </div>

        {!!screenshots?.length && <ScreenshotsCarousel screenshots={screenshots} t={t} />}

        {!!description?.trim() && (
          <div className="flex flex-col gap-3 px-4 pb-5">
            <div className="flex items-center justify-between">
              <p className="text-lg font-bold text-slate-900">
                {t("preview.aboutTitle", { ns: "pwa" })}
              </p>
              <button
                type="button"
                onClick={() => setAboutOpen(true)}
                aria-label={t("preview.aboutOpenAria", { ns: "pwa" })}
                className="rounded p-1 text-slate-900 hover:bg-slate-100"
              >
                <ArrowRightIcon className="size-5" />
              </button>
            </div>
            <p className="line-clamp-5 whitespace-pre-line text-sm leading-relaxed text-slate-900">
              {description}
            </p>
          </div>
        )}

        {!!updatedDateText && (
          <div className="flex flex-col gap-1 px-4 pb-5">
            <p className="text-sm font-bold text-slate-700">
              {t("preview.updatedTitle", { ns: "pwa" })}
            </p>
            <p className="text-sm text-slate-900">{updatedDateText}</p>
          </div>
        )}

        <div
          className="flex flex-col gap-3 border-t px-4 py-5"
          style={{ borderColor: REF_BORDER }}
        >
          <div className="flex items-center justify-between">
            <p className="text-lg font-bold text-slate-900">
              {t("preview.dataSafetyTitle", { ns: "pwa" })}
            </p>
            <span
              aria-hidden
              className="text-xl font-light leading-none text-slate-900"
            >
              ›
            </span>
          </div>
          <p className="text-sm leading-relaxed text-slate-700">
            {t("preview.dataSafetyText", { ns: "pwa" })}
          </p>
        </div>

        <div
          className="flex flex-col gap-1 border-t px-4 py-5"
          style={{ borderColor: REF_BORDER }}
        >
          <p className="text-lg font-semibold text-slate-900">
            {t("preview.reviewsTitle", { ns: "pwa" })}
          </p>
          <p className="mb-3 text-[13px] text-slate-700">
            {t("preview.reviewsVerified", { ns: "pwa" })}
          </p>
          <div className="flex items-center gap-5">
            <div className="flex min-w-[100px] flex-col items-center gap-1">
              <span className="text-[56px] font-normal leading-none text-slate-900">
                {ratingText}
              </span>
              <span className="flex items-center">
                {STAR_ROWS.map((n) => {
                  const index = 6 - n; // 1..5, left to right
                  return (
                    <PartialStar
                      key={n}
                      fill={
                        hasRating
                          ? Math.min(1, Math.max(0, rating! - (index - 1)))
                          : 0
                      }
                    />
                  );
                })}
              </span>
              <span className="text-xs text-slate-600">{reviewCountText}</span>
            </div>
            <div className="flex flex-1 flex-col gap-1">
              {STAR_ROWS.map((n) => (
                <StarBar key={n} n={n} pct={starPct[n]} />
              ))}
            </div>
          </div>
        </div>

        {!!whatsNew?.trim() && (
          <div
            className="flex flex-col gap-3 border-t px-4 py-5"
            style={{ borderColor: REF_BORDER }}
          >
            <p className="text-lg font-bold text-slate-900">
              {t("preview.whatsNewTitle", { ns: "pwa" })}
            </p>
            <p className="whitespace-pre-line text-sm leading-relaxed text-slate-900">
              {whatsNew}
            </p>
          </div>
        )}

        <div
          className="flex flex-col gap-3 border-t px-5 py-5 text-sm text-slate-900"
          style={{ borderColor: REF_BORDER }}
        >
          <span>{t("preview.footer.store", { ns: "pwa" })}</span>
          <span>{t("preview.footer.giftCards", { ns: "pwa" })}</span>
          <span>{t("preview.footer.redeemCode", { ns: "pwa" })}</span>
          <span>{t("preview.footer.familyLibrary", { ns: "pwa" })}</span>
          <p className="pt-2 text-sm font-semibold">
            {t("preview.footer.kidsFamilyTitle", { ns: "pwa" })}
          </p>
          <span>{t("preview.footer.parentGuide", { ns: "pwa" })}</span>
          <span>{t("preview.footer.familyLibrary", { ns: "pwa" })}</span>
        </div>
        <div
          className="flex flex-wrap gap-4 border-t px-5 py-3 text-[10px] text-slate-900"
          style={{ borderColor: REF_BORDER }}
        >
          <span>{t("preview.footer.terms", { ns: "pwa" })}</span>
          <span>{t("preview.footer.privacy", { ns: "pwa" })}</span>
          <span>{t("preview.footer.developers", { ns: "pwa" })}</span>
        </div>

        <div
          className="sticky bottom-0 z-30 mt-auto flex items-center justify-around border-t bg-white px-2 py-2"
          style={{ borderColor: REF_BORDER }}
        >
          {[
            {
              Icon: GamepadIcon,
              label: t("preview.nav.games", { ns: "pwa" }),
              active: true,
            },
            {
              Icon: LayoutGridIcon,
              label: t("preview.nav.apps", { ns: "pwa" }),
              active: false,
            },
            {
              Icon: BookOpenIcon,
              label: t("preview.nav.books", { ns: "pwa" }),
              active: false,
            },
            {
              Icon: SmileIcon,
              label: t("preview.nav.kids", { ns: "pwa" }),
              active: false,
            },
          ].map(({ Icon, label, active }) => (
            <div
              key={label}
              className={cn(
                "flex min-w-[70px] flex-col items-center gap-1 rounded-lg px-2 py-1 text-slate-600",
                active && "bg-[#00A173]/10",
              )}
              style={active ? { color: REF_PRIMARY } : undefined}
            >
              <Icon className="size-6" />
              <span className="text-[10px]">{label}</span>
            </div>
          ))}
        </div>
      </div>

      {aboutOpen && (
        <div className="absolute inset-0 z-20 flex items-center justify-center bg-black/30 px-4">
          <div
            role="dialog"
            aria-modal="false"
            aria-label={t("preview.aboutTitle", { ns: "pwa" })}
            className="flex max-h-[80%] w-full flex-col overflow-hidden rounded-2xl bg-white shadow-xl"
          >
            <div
              className="flex items-center justify-between gap-3 border-b px-4 py-3"
              style={{ borderColor: REF_BORDER }}
            >
              <div className="flex min-w-0 items-center gap-3">
                {!!iconUrl && (
                  <div className="size-10 shrink-0 overflow-hidden rounded-full">
                    {/* eslint-disable-next-line @next/next/no-img-element -- operator-supplied icon URL, see the card header */}
                    <img
                      src={iconUrl}
                      alt=""
                      className="size-full object-cover"
                      onError={(e) => (e.currentTarget.style.display = "none")}
                    />
                  </div>
                )}
                <div className="flex min-w-0 flex-col">
                  <p className="truncate text-base font-bold text-slate-900">
                    {displayName}
                  </p>
                  <p className="text-xs text-slate-600">
                    {t("preview.aboutTitle", { ns: "pwa" })}
                  </p>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setAboutOpen(false)}
                aria-label={t("preview.closeAria", { ns: "pwa" })}
                className="rounded-full p-1.5 text-slate-600 hover:bg-slate-100"
              >
                <XIcon className="size-4" />
              </button>
            </div>

            <div className="flex flex-col gap-5 overflow-y-auto px-4 py-4">
              <p className="whitespace-pre-line text-sm leading-relaxed text-slate-900">
                {description}
              </p>

              {detailRows.length > 0 && (
                <>
                  <div
                    className="border-t"
                    style={{ borderColor: REF_BORDER }}
                  />
                  <div className="flex flex-col gap-4">
                    <p className="text-lg font-bold text-slate-900">
                      {t("preview.detailsTitle", { ns: "pwa" })}
                    </p>
                    <div className="grid grid-cols-2 gap-x-4 gap-y-4">
                      {detailRows.map((row) => (
                        <div key={row.label} className="flex flex-col gap-1">
                          <p className="text-sm font-bold text-slate-900">
                            {row.label}
                          </p>
                          <p className="text-sm text-slate-600">{row.value}</p>
                          {row.notes?.map((note) => (
                            <p key={note} className="text-sm text-slate-600">
                              {note}
                            </p>
                          ))}
                        </div>
                      ))}
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

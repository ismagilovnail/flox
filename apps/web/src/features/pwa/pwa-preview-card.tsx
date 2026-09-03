import {
  BadgeCheckIcon,
  BookOpenIcon,
  BookmarkIcon,
  DownloadIcon,
  GamepadIcon,
  HardDriveIcon,
  HelpCircleIcon,
  LayoutGridIcon,
  PlayIcon,
  SearchIcon,
  Share2Icon,
  ShieldIcon,
  SmartphoneIcon,
  SmileIcon,
  StarIcon,
} from "lucide-react";
import type { TFunction } from "i18next";

import { cn } from "@/lib/utils";

const STAR_ROWS = [5, 4, 3, 2, 1] as const;

function StarBar({ n, pct }: { n: number; pct?: number }) {
  const hasValue = typeof pct === "number" && !Number.isNaN(pct);
  return (
    <div className="flex items-center gap-2 text-xs text-neutral-500">
      <span className="w-2 text-right">{n}</span>
      <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-neutral-200">
        <div className="h-full rounded-full bg-neutral-900" style={{ width: `${hasValue ? Math.min(100, Math.max(0, pct)) : 0}%` }} />
      </div>
      <span className="w-8 text-right">{hasValue ? `${pct}%` : "—"}</span>
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
 * value. Footer links and the bottom nav bar are pure decoration (match
 * the real Google Play chrome, go nowhere) — not backed by any field.
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
  /** "rtl" mirrors layout direction only (the reference's own "Arabic
   * text" toggle) — no machine translation, matching the user's explicit
   * "our own code, not their Google Translate widget" instruction. */
  dir?: "ltr" | "rtl";
  className?: string;
}) {
  const hasRating = typeof rating === "number" && !Number.isNaN(rating);
  const displayName = name?.trim() || t("preview.placeholderName", { ns: "pwa" });
  const displayDeveloper = developer?.trim() || t("preview.placeholderDeveloper", { ns: "pwa" });
  const starPct: Record<(typeof STAR_ROWS)[number], number | undefined> = {
    5: ratingStars5,
    4: ratingStars4,
    3: ratingStars3,
    2: ratingStars2,
    1: ratingStars1,
  };
  const updatedDate = new Intl.DateTimeFormat(undefined, { dateStyle: "long" }).format(new Date());

  return (
    <div
      dir={dir}
      className={cn(
        "mx-auto flex w-[390px] max-w-full flex-col overflow-hidden rounded-[2.5rem] border-[12px] border-neutral-900 bg-white text-neutral-900 shadow-xl dark:border-neutral-800",
        className,
      )}
      // Matches the reference listing's own font stack minus "Google
      // Sans" itself — that name isn't actually served by Google Fonts
      // (it's Google's internal-only typeface), so their real rendered
      // font is this exact system-font fallback chain already. Using it
      // directly needs no external font request at all.
      style={{ fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif' }}
    >
      <div className="flex max-h-[70vh] flex-col overflow-y-auto">
        <div className="sticky top-0 z-10 flex items-center justify-between bg-white px-4 pb-2 pt-4">
          <div className="flex items-center gap-1.5 text-sm font-semibold">
            <PlayIcon className="size-4 fill-emerald-500 text-emerald-500" />
            Google Play
          </div>
          <div className="flex items-center gap-3 text-neutral-400">
            <SearchIcon className="size-4" />
            <HelpCircleIcon className="size-4" />
          </div>
        </div>

        <div className="flex flex-col gap-3 px-4 pb-4">
          <div className="flex items-start gap-3">
            <div className="flex size-16 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-neutral-100 text-2xl font-bold text-neutral-300">
              {iconUrl ? (
                // eslint-disable-next-line @next/next/no-img-element -- external, unknown-dimension icon URL; next/image would need a remotePatterns entry per operator-supplied host
                <img src={iconUrl} alt="" className="size-full object-cover" onError={(e) => (e.currentTarget.style.display = "none")} />
              ) : (
                displayName.charAt(0).toUpperCase()
              )}
            </div>
            <div className="flex flex-col gap-0.5 pt-0.5">
              <p className="text-base font-semibold leading-tight text-neutral-900">{displayName}</p>
              <p className="flex items-center gap-1 text-sm text-blue-600">
                {displayDeveloper}
                <BadgeCheckIcon className="size-3.5 shrink-0 fill-blue-600 text-white" />
              </p>
              <p className="text-xs text-neutral-500">{t("preview.adFree", { ns: "pwa" })}</p>
            </div>
          </div>

          <div className="flex items-center justify-around text-center">
            <div className="flex flex-col items-center gap-0.5">
              <span className="flex items-center gap-0.5 text-sm font-medium text-neutral-900">
                {hasRating ? rating!.toFixed(1) : "—"}
                <StarIcon className="size-3 fill-neutral-900 text-neutral-900" />
              </span>
              <span className="text-[0.65rem] text-neutral-500">
                {ratingCount ? t("preview.reviewsCount", { ns: "pwa", count: ratingCount }) : t("preview.noData", { ns: "pwa" })}
              </span>
            </div>
            <div className="h-6 w-px bg-neutral-200" />
            <div className="flex flex-col items-center gap-0.5">
              <DownloadIcon className="size-3.5 text-neutral-700" />
              <span className="text-[0.65rem] text-neutral-500">{downloadsLabel?.trim() || "—"}</span>
            </div>
            <div className="h-6 w-px bg-neutral-200" />
            <div className="flex flex-col items-center gap-0.5">
              <HardDriveIcon className="size-3.5 text-neutral-700" />
              <span className="text-[0.65rem] text-neutral-500">
                {typeof sizeMb === "number" && !Number.isNaN(sizeMb) ? t("preview.sizeMb", { ns: "pwa", size: sizeMb }) : "—"}
              </span>
            </div>
          </div>

          <button
            type="button"
            disabled
            className="w-full rounded-full bg-emerald-600 py-2 text-sm font-medium text-white disabled:opacity-100"
          >
            {t("preview.install", { ns: "pwa" })}
          </button>

          <div className="flex items-center justify-between text-xs font-medium text-blue-600">
            <span className="flex items-center gap-1">
              <Share2Icon className="size-3.5" /> {t("preview.share", { ns: "pwa" })}
            </span>
            <span className="flex items-center gap-1">
              <BookmarkIcon className="size-3.5" /> {t("preview.wishlist", { ns: "pwa" })}
            </span>
          </div>

          <div className="flex items-center gap-1.5 text-[0.65rem] text-neutral-500">
            <SmartphoneIcon className="size-3.5 shrink-0 text-neutral-400" />
            {t("preview.availableOnDevices", { ns: "pwa" })}
          </div>
        </div>

        {!!screenshots?.length && (
          <div className="flex gap-2 overflow-x-auto px-4 pb-4">
            {screenshots.map((src, i) => (
              // eslint-disable-next-line @next/next/no-img-element -- external, operator-supplied screenshot URL
              <img
                key={`${src}-${i}`}
                src={src}
                alt={t("preview.screenshotAlt", { ns: "pwa", n: i + 1 })}
                className="h-32 w-auto shrink-0 rounded-lg border border-neutral-200 object-cover"
                onError={(e) => (e.currentTarget.style.display = "none")}
              />
            ))}
          </div>
        )}

        {!!description?.trim() && (
          <div className="flex flex-col gap-1 px-4 pb-4">
            <p className="text-sm font-semibold text-neutral-900">{t("preview.aboutTitle", { ns: "pwa" })}</p>
            <p className="line-clamp-4 whitespace-pre-line text-xs text-neutral-600">{description}</p>
          </div>
        )}

        <div className="flex flex-col gap-1 px-4 pb-4">
          <p className="text-[0.65rem] font-semibold text-neutral-500">{t("preview.updatedTitle", { ns: "pwa" })}</p>
          <p className="text-xs text-neutral-700">{updatedDate}</p>
        </div>

        <div className="flex flex-col gap-2 border-t border-neutral-100 px-4 py-4">
          <div className="flex items-center gap-1.5">
            <ShieldIcon className="size-3.5 text-neutral-500" />
            <p className="text-sm font-semibold text-neutral-900">{t("preview.dataSafetyTitle", { ns: "pwa" })}</p>
          </div>
          <p className="text-[0.65rem] leading-relaxed text-neutral-500">{t("preview.dataSafetyText", { ns: "pwa" })}</p>
        </div>

        <div className="flex flex-col gap-2 border-t border-neutral-100 px-4 py-4">
          <p className="text-sm font-semibold text-neutral-900">{t("preview.reviewsTitle", { ns: "pwa" })}</p>
          <div className="flex items-center gap-4">
            <div className="flex flex-col items-center">
              <span className="text-3xl font-normal text-neutral-900">{hasRating ? rating!.toFixed(1) : "—"}</span>
              <span className="text-[0.65rem] text-neutral-500">
                {ratingCount ? t("preview.reviewsCount", { ns: "pwa", count: ratingCount }) : t("preview.noData", { ns: "pwa" })}
              </span>
            </div>
            <div className="flex flex-1 flex-col gap-1">
              {STAR_ROWS.map((n) => (
                <StarBar key={n} n={n} pct={starPct[n]} />
              ))}
            </div>
          </div>
        </div>

        {!!whatsNew?.trim() && (
          <div className="flex flex-col gap-1 border-t border-neutral-100 px-4 py-4">
            <p className="text-sm font-semibold text-neutral-900">{t("preview.whatsNewTitle", { ns: "pwa" })}</p>
            <p className="whitespace-pre-line text-xs text-neutral-600">{whatsNew}</p>
          </div>
        )}

        <div className="flex flex-col gap-3 border-t border-neutral-100 px-4 py-4 text-xs text-neutral-700">
          <span>{t("preview.footer.store", { ns: "pwa" })}</span>
          <span>{t("preview.footer.giftCards", { ns: "pwa" })}</span>
          <span>{t("preview.footer.redeemCode", { ns: "pwa" })}</span>
          <span>{t("preview.footer.familyLibrary", { ns: "pwa" })}</span>
        </div>
        <div className="flex flex-wrap gap-3 border-t border-neutral-100 px-4 py-3 text-[0.65rem] text-neutral-400">
          <span>{t("preview.footer.terms", { ns: "pwa" })}</span>
          <span>{t("preview.footer.privacy", { ns: "pwa" })}</span>
          <span>{t("preview.footer.developers", { ns: "pwa" })}</span>
        </div>

        <div className="sticky bottom-0 mt-auto flex items-center justify-around border-t border-neutral-100 bg-white px-2 py-2">
          {[
            { Icon: GamepadIcon, label: t("preview.nav.games", { ns: "pwa" }) },
            { Icon: LayoutGridIcon, label: t("preview.nav.apps", { ns: "pwa" }) },
            { Icon: BookOpenIcon, label: t("preview.nav.books", { ns: "pwa" }) },
            { Icon: SmileIcon, label: t("preview.nav.kids", { ns: "pwa" }) },
          ].map(({ Icon, label }) => (
            <div key={label} className="flex flex-col items-center gap-0.5 text-neutral-400">
              <Icon className="size-4" />
              <span className="text-[0.6rem]">{label}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

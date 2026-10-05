"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";
import { Controller, useFieldArray, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { ImagesIcon, XIcon } from "lucide-react";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Mono } from "@/components/ui/typography";
import type { PwaStatus } from "@/lib/api/pwa";
import { PwaCommentsEditor } from "@/features/pwa/pwa-comments-editor";
import { commentStats, setKey } from "@/features/pwa/pwa-comments";
import { PwaPreviewCard } from "@/features/pwa/pwa-preview-card";
import { GalleryFilePickerDialog } from "@/features/content-gallery/gallery-file-picker-dialog";
import { GalleryUploadCell } from "@/features/content-gallery/gallery-upload-cell";
import { cn } from "@/lib/utils";

/** register(name, { setValueAs: numberOrUndefined }) turns an empty
 * input into `undefined` (not NaN) before zod ever sees it — the schema
 * can then stay a plain z.number().optional(), no preprocess/refine
 * needed, and useForm's generic matches the resolver's output type
 * exactly (a z.preprocess schema's input/output types diverge, which
 * @hookform/resolvers' Resolver<Input, Context, Output> then rejects
 * against a single-type useForm<PwaFormValues>). */
function numberOrUndefined(v: string): number | undefined {
  return v === "" ? undefined : Number(v);
}

/** Two independent dropdowns, matching a reference tracker's own
 * landing-page "Цветовая схема" (accent color) + "Тема" (light/dark)
 * pair exactly, rather than one combined picker — replacing the raw
 * theme_color/background_color pickers outright (the user explicitly
 * asked to drop the raw palette UI once the dropdowns covered the same
 * ground, the same "pick-only, no manual escape hatch" call already
 * made for the icon field above). A PWA whose stored color doesn't
 * match either preset list (legacy data, or set through some other
 * client) simply shows no dropdown selection until the operator picks
 * one — its real value is untouched until then. */
const PWA_ACCENT_COLORS = [
  { value: "green", color: "#16a34a" },
  { value: "blue", color: "#2563eb" },
  { value: "blueModern", color: "#0ea5e9" },
  { value: "purple", color: "#7c3aed" },
] as const;

const PWA_BACKGROUND_THEMES = [
  { value: "light", color: "#ffffff" },
  { value: "dark", color: "#0a0a0a" },
] as const;

function matchPreset<T extends { value: string; color: string }>(presets: readonly T[], hex?: string): string {
  return presets.find((p) => p.color.toLowerCase() === (hex ?? "").toLowerCase())?.value ?? "";
}

/** Factory, not a module-level const — see source-form.tsx's
 * buildSourceFormSchema for why (Zod messages are user-facing text and
 * need the live translator). */
export function buildPwaFormSchema(t: TFunction) {
  const hexColor = z
    .string()
    .regex(
      /^#[0-9a-fA-F]{6}$/,
      t("form.validation.colorInvalid", { ns: "pwa" }),
    );
  const ratingStarPct = z
    .number()
    .min(0, t("form.validation.ratingStarsRange", { ns: "pwa" }))
    .max(100, t("form.validation.ratingStarsRange", { ns: "pwa" }))
    .optional();
  const detailLabel = z
    .string()
    .max(60, t("form.validation.detailLabelMax", { ns: "pwa" }))
    .optional();
  return z.object({
    name: z
      .string()
      .min(2, t("form.validation.nameMin", { ns: "pwa" }))
      .max(80),
    shortName: z
      .string()
      .min(1, t("form.validation.shortNameRequired", { ns: "pwa" }))
      .max(20),
    themeColor: hexColor,
    backgroundColor: hexColor,
    themeAuto: z.boolean(),
    showHeader: z.boolean(),
    showBottomNav: z.boolean(),
    headerTitle: z
      .string()
      .max(40, t("form.validation.headerTitleMax", { ns: "pwa" })),
    headerLogoUrl: z.union([
      z.literal(""),
      z.url(t("form.validation.headerLogoUrlInvalid", { ns: "pwa" })),
    ]),
    iconUrl: z.url(t("form.validation.iconUrlInvalid", { ns: "pwa" })),
    startUrl: z
      .string()
      .min(1, t("form.validation.startUrlRequired", { ns: "pwa" })),
    bounceInAppWebview: z.boolean(),
    status: z.enum(["active", "paused", "archived"] as [
      PwaStatus,
      ...PwaStatus[],
    ]),
    developer: z
      .string()
      .max(100, t("form.validation.developerMax", { ns: "pwa" }))
      .optional(),
    description: z
      .string()
      .max(4000, t("form.validation.descriptionMax", { ns: "pwa" }))
      .optional(),
    rating: z
      .number()
      .min(0, t("form.validation.ratingRange", { ns: "pwa" }))
      .max(5, t("form.validation.ratingRange", { ns: "pwa" }))
      .optional(),
    ratingCount: z
      .number()
      .min(0, t("form.validation.ratingCountRange", { ns: "pwa" }))
      .optional(),
    downloadsLabel: z
      .string()
      .max(20, t("form.validation.downloadsLabelMax", { ns: "pwa" }))
      .optional(),
    sizeMb: z
      .number()
      .min(0, t("form.validation.sizeMbRange", { ns: "pwa" }))
      .optional(),
    screenshots: z.array(
      z.object({
        url: z.url(t("form.validation.screenshotUrlInvalid", { ns: "pwa" })),
      }),
    ),
    whatsNew: z
      .string()
      .max(2000, t("form.validation.whatsNewMax", { ns: "pwa" }))
      .optional(),
    ratingStars5: ratingStarPct,
    ratingStars4: ratingStarPct,
    ratingStars3: ratingStarPct,
    ratingStars2: ratingStarPct,
    ratingStars1: ratingStarPct,
    version: detailLabel,
    ageRating: detailLabel,
    category: detailLabel,
    interactiveElements: detailLabel,
    containsAds: z.boolean(),
    inAppPurchases: z.boolean(),
    comments: z.array(
      z.object({
        geo: z.string(),
        lang: z.string(),
        author: z.string(),
        avatarUrl: z.string(),
        rating: z.number(),
        likes: z.number(),
        date: z.string(),
        text: z.string(),
        reply: z.string(),
      }),
    ),
    updatedDate: z
      .string()
      .regex(
        /^\d{4}-\d{2}-\d{2}$/,
        t("form.validation.updatedDateInvalid", { ns: "pwa" }),
      )
      .or(z.literal(""))
      .optional(),
  });
}

export type PwaFormValues = z.infer<ReturnType<typeof buildPwaFormSchema>>;

const STAR_FIELDS = [
  { n: 5, name: "ratingStars5" as const },
  { n: 4, name: "ratingStars4" as const },
  { n: 3, name: "ratingStars3" as const },
  { n: 2, name: "ratingStars2" as const },
  { n: 1, name: "ratingStars1" as const },
];

/** The PWA create/edit form — its own page (features/pwa/{new-pwa-view,
 * pwa-detail-view}.tsx) instead of a slide-out Sheet, matching the
 * traffic-source and offer create/edit pages' layout. Two-column layout:
 * settings on the left, a live store-listing preview (pwa-preview-card.tsx)
 * on the right, matching a reference tracker's PWA builder the user shared —
 * including its screenshot gallery, "what's new," and per-star rating
 * breakdown, all real operator-entered fields (CLAUDE.md #6: never a
 * fabricated number). */
export function PwaForm({
  title,
  defaultValues,
  submitLabel,
  onSubmit,
  headerActions,
  disabled = false,
}: {
  title: string;
  defaultValues: Partial<PwaFormValues>;
  submitLabel: string;
  onSubmit: (values: PwaFormValues) => void;
  /** Rendered in the settings card's own header (top-right, via
   * CardAction) — the edit page's Pause/Resume/Duplicate/Archive
   * buttons (PwaRowActions variant="inline"), status changes moved out
   * of the form itself since they're not part of "save changes" (same
   * status-field-removed-in-favor-of-header-buttons call already made
   * for campaigns). Omitted on the new-PWA page — there's no existing
   * record yet to act on. */
  headerActions?: ReactNode;
  disabled?: boolean;
}) {
  const { t } = useTranslation(["pwa", "common"]);
  const form = useForm<PwaFormValues>({
    resolver: zodResolver(buildPwaFormSchema(t)),
    defaultValues: {
      name: "",
      shortName: "",
      themeColor: "#16a34a",
      backgroundColor: "#ffffff",
      themeAuto: false,
      showHeader: true,
      showBottomNav: true,
      headerTitle: "",
      headerLogoUrl: "",
      iconUrl: "",
      startUrl: "/install",
      bounceInAppWebview: true,
      status: "active",
      developer: "",
      description: "",
      downloadsLabel: "",
      screenshots: [],
      comments: [],
      whatsNew: "",
      version: "",
      containsAds: false,
      inAppPurchases: false,
      updatedDate: "",
      ageRating: "",
      category: "",
      interactiveElements: "",
      ...defaultValues,
    },
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  const screenshotArray = useFieldArray({ control, name: "screenshots" });
  const manifest = useWatch({ control });
  const allComments = useWatch({ control, name: "comments" }) ?? [];
  const [activeSet, setActiveSet] = useState(() => (defaultValues.comments?.[0] ? setKey(defaultValues.comments[0]) : ""));
  // The preview shows the comment set being edited; its rating, count and
  // star bars come from those comments when there are any.
  const setComments = allComments.filter((c) => setKey(c) === activeSet);
  const stats = commentStats(setComments);
  const [previewDir, setPreviewDir] = useState<"ltr" | "rtl">("ltr");
  const [pickingIcon, setPickingIcon] = useState(false);
  const [pickingHeaderLogo, setPickingHeaderLogo] = useState(false);
  const [pickingScreenshot, setPickingScreenshot] = useState(false);
  const accentScheme = matchPreset(PWA_ACCENT_COLORS, manifest.themeColor);
  // "auto" is not a color: it is its own flag, shown as a third dropdown option.
  const backgroundTheme = manifest.themeAuto
    ? "auto"
    : matchPreset(PWA_BACKGROUND_THEMES, manifest.backgroundColor);

  function handleAccentChange(value: string) {
    const preset = PWA_ACCENT_COLORS.find((p) => p.value === value);
    if (preset) form.setValue("themeColor", preset.color, { shouldValidate: true, shouldDirty: true });
  }

  function handleBackgroundThemeChange(value: string) {
    if (value === "auto") {
      form.setValue("themeAuto", true, { shouldValidate: true, shouldDirty: true });
      return;
    }
    form.setValue("themeAuto", false, { shouldValidate: true, shouldDirty: true });
    const preset = PWA_BACKGROUND_THEMES.find((p) => p.value === value);
    if (preset) form.setValue("backgroundColor", preset.color, { shouldValidate: true, shouldDirty: true });
  }

  return (
    <form
      onSubmit={handleSubmit(onSubmit)}
      className="flex flex-col gap-4 lg:grid lg:grid-cols-[1fr_440px] lg:items-start lg:gap-6"
    >
      <fieldset disabled={disabled} className="contents">
      <div className="flex flex-col gap-4">
        <Card>
          <CardHeader>
            <CardTitle>{title}</CardTitle>
            <CardDescription>
              {t("form.description", { ns: "pwa" })}
            </CardDescription>
            <CardAction className="flex flex-wrap items-center justify-end gap-2">
              {headerActions}
              <Button type="button" variant="outline" asChild>
                <Link href="/pwa">{t("actions.cancel", { ns: "common" })}</Link>
              </Button>
              <Button type="submit" disabled={isSubmitting}>
                {submitLabel}
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="pwa-name">
                  {t("form.nameLabel", { ns: "pwa" })}
                </Label>
                <Input
                  id="pwa-name"
                  placeholder={t("form.namePlaceholder", { ns: "pwa" })}
                  {...register("name")}
                  aria-invalid={!!errors.name}
                />
                {errors.name && (
                  <p className="text-xs text-danger">{errors.name.message}</p>
                )}
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="pwa-short-name">
                  {t("form.shortNameLabel", { ns: "pwa" })}
                </Label>
                <Input
                  id="pwa-short-name"
                  placeholder={t("form.shortNamePlaceholder", { ns: "pwa" })}
                  {...register("shortName")}
                  aria-invalid={!!errors.shortName}
                />
                {errors.shortName && (
                  <p className="text-xs text-danger">
                    {errors.shortName.message}
                  </p>
                )}
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="pwa-accent">
                  {t("form.colorSchemeLabel", { ns: "pwa" })}
                </Label>
                <Select value={accentScheme} onValueChange={handleAccentChange}>
                  <SelectTrigger id="pwa-accent" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PWA_ACCENT_COLORS.map((p) => (
                      <SelectItem key={p.value} value={p.value}>
                        <span className="size-3 rounded-full border border-black/10" style={{ background: p.color }} />
                        {t(`form.colorScheme.${p.value}`, { ns: "pwa" })}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="pwa-theme-mode">
                  {t("form.backgroundThemeLabel", { ns: "pwa" })}
                </Label>
                <Select value={backgroundTheme} onValueChange={handleBackgroundThemeChange}>
                  <SelectTrigger id="pwa-theme-mode" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PWA_BACKGROUND_THEMES.map((p) => (
                      <SelectItem key={p.value} value={p.value}>
                        <span className="size-3 rounded-full border border-black/10" style={{ background: p.color }} />
                        {t(`form.backgroundTheme.${p.value}`, { ns: "pwa" })}
                      </SelectItem>
                    ))}
                    <SelectItem value="auto">
                      <span
                        className="size-3 rounded-full border border-black/10"
                        style={{ background: "linear-gradient(90deg, #ffffff 50%, #0a0a0a 50%)" }}
                      />
                      {t("form.backgroundTheme.auto", { ns: "pwa" })}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid gap-1.5">
              <Label id="pwa-icon-label">
                {t("form.iconUrlLabel", { ns: "pwa" })}
              </Label>
              <div className="relative w-fit">
                <button
                  type="button"
                  aria-labelledby="pwa-icon-label"
                  onClick={() => setPickingIcon(true)}
                  className={cn(
                    "group relative flex size-24 items-center justify-center overflow-hidden rounded-lg border-2 border-dashed text-muted-foreground transition-colors",
                    errors.iconUrl ? "border-danger" : "border-border hover:border-muted-foreground/50 hover:text-foreground",
                  )}
                >
                  {manifest.iconUrl ? (
                    <>
                      {/* eslint-disable-next-line @next/next/no-img-element -- a Content Gallery public object-storage URL (or any externally-hosted one), not a Next-optimizable static asset */}
                      <img src={manifest.iconUrl} alt="" className="size-full object-cover" />
                      <div className="absolute inset-0 flex items-center justify-center bg-black/50 opacity-0 transition-opacity group-hover:opacity-100">
                        <span className="text-xs font-medium text-white">{t("form.iconUrlChange", { ns: "pwa" })}</span>
                      </div>
                    </>
                  ) : (
                    <span className="flex flex-col items-center gap-1 px-2 text-center">
                      <ImagesIcon className="size-5" />
                      <span className="text-[11px] font-medium">{t("form.iconUrlChoose", { ns: "pwa" })}</span>
                    </span>
                  )}
                </button>
                {manifest.iconUrl && (
                  <IconButton
                    type="button"
                    size="icon-xs"
                    variant="secondary"
                    className="absolute -top-1.5 -right-1.5 rounded-full shadow"
                    aria-label={t("form.iconUrlRemoveAria", { ns: "pwa" })}
                    onClick={(e) => {
                      e.stopPropagation();
                      form.setValue("iconUrl", "", { shouldValidate: true, shouldDirty: true });
                    }}
                  >
                    <XIcon className="size-3" />
                  </IconButton>
                )}
              </div>
              {errors.iconUrl && (
                <p className="text-xs text-danger">{errors.iconUrl.message}</p>
              )}
              <GalleryFilePickerDialog
                open={pickingIcon}
                onOpenChange={setPickingIcon}
                requiredAspect={1}
                onPick={(file) =>
                  form.setValue("iconUrl", file.url, { shouldValidate: true, shouldDirty: true })
                }
              />
            </div>

            {/* start_url isn't operator-editable: this PWA is attached to a
                campaign and into flows, which is what actually resolves
                where an install/open lands — a manually-typed manifest
                path here would just be a second, disconnected source of
                truth. Still a real, required Web App Manifest field
                (kept in the schema/preview/backend), so it stays
                registered — a fixed default for new PWAs, an existing
                record's own saved value on edit — just with no input for
                the operator to change it through. */}
            <input type="hidden" {...register("startUrl")} />

            <div className="flex items-center justify-between rounded-md border border-border p-2.5">
              <div>
                <p className="text-sm font-medium">
                  {t("form.bounceLabel", { ns: "pwa" })}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t("form.bounceHint", { ns: "pwa" })}
                </p>
              </div>
              <Controller
                control={control}
                name="bounceInAppWebview"
                render={({ field }) => (
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    aria-label={t("form.bounceAria", { ns: "pwa" })}
                  />
                )}
              />
            </div>

            <div className="flex items-center justify-between rounded-md border border-border p-2.5">
              <div>
                <p className="text-sm font-medium">
                  {t("form.showHeaderLabel", { ns: "pwa" })}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t("form.showHeaderHint", { ns: "pwa" })}
                </p>
              </div>
              <Controller
                control={control}
                name="showHeader"
                render={({ field }) => (
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    aria-label={t("form.showHeaderAria", { ns: "pwa" })}
                  />
                )}
              />
            </div>

            {manifest.showHeader && (
              <div className="grid gap-4 rounded-md border border-border p-2.5 sm:grid-cols-[auto_1fr]">
                <div className="grid gap-1.5">
                  <Label id="pwa-header-logo-label">
                    {t("form.headerLogoLabel", { ns: "pwa" })}
                  </Label>
                  <div className="relative w-fit">
                    <button
                      type="button"
                      aria-labelledby="pwa-header-logo-label"
                      onClick={() => setPickingHeaderLogo(true)}
                      className={cn(
                        "group relative flex size-16 items-center justify-center overflow-hidden rounded-lg border-2 border-dashed text-muted-foreground transition-colors",
                        errors.headerLogoUrl ? "border-danger" : "border-border hover:border-muted-foreground/50 hover:text-foreground",
                      )}
                    >
                      {manifest.headerLogoUrl ? (
                        <>
                          {/* eslint-disable-next-line @next/next/no-img-element -- a Content Gallery public object-storage URL, same as the icon above */}
                          <img src={manifest.headerLogoUrl} alt="" className="size-full object-contain p-1" />
                          <div className="absolute inset-0 flex items-center justify-center bg-black/50 opacity-0 transition-opacity group-hover:opacity-100">
                            <span className="text-xs font-medium text-white">{t("form.iconUrlChange", { ns: "pwa" })}</span>
                          </div>
                        </>
                      ) : (
                        <span className="flex flex-col items-center gap-1 px-1 text-center">
                          <ImagesIcon className="size-4" />
                          <span className="text-[10px] font-medium">{t("form.iconUrlChoose", { ns: "pwa" })}</span>
                        </span>
                      )}
                    </button>
                    {manifest.headerLogoUrl && (
                      <IconButton
                        type="button"
                        size="icon-xs"
                        variant="secondary"
                        className="absolute -top-1.5 -right-1.5 rounded-full shadow"
                        aria-label={t("form.headerLogoRemoveAria", { ns: "pwa" })}
                        onClick={(e) => {
                          e.stopPropagation();
                          form.setValue("headerLogoUrl", "", { shouldValidate: true, shouldDirty: true });
                        }}
                      >
                        <XIcon className="size-3" />
                      </IconButton>
                    )}
                  </div>
                  {errors.headerLogoUrl && (
                    <p className="text-xs text-danger">{errors.headerLogoUrl.message}</p>
                  )}
                  <GalleryFilePickerDialog
                    open={pickingHeaderLogo}
                    onOpenChange={setPickingHeaderLogo}
                    onPick={(file) =>
                      form.setValue("headerLogoUrl", file.url, { shouldValidate: true, shouldDirty: true })
                    }
                  />
                </div>
                <div className="grid content-start gap-1.5">
                  <Label htmlFor="pwa-header-title">
                    {t("form.headerTitleLabel", { ns: "pwa" })}
                  </Label>
                  <Input
                    id="pwa-header-title"
                    maxLength={40}
                    placeholder={t("form.headerTitlePlaceholder", { ns: "pwa" })}
                    {...register("headerTitle")}
                    aria-invalid={!!errors.headerTitle}
                  />
                  {errors.headerTitle && (
                    <p className="text-xs text-danger">{errors.headerTitle.message}</p>
                  )}
                  <p className="text-xs text-muted-foreground">
                    {t("form.headerBrandHint", { ns: "pwa" })}
                  </p>
                </div>
              </div>
            )}

            <div className="flex items-center justify-between rounded-md border border-border p-2.5">
              <div>
                <p className="text-sm font-medium">
                  {t("form.showBottomNavLabel", { ns: "pwa" })}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t("form.showBottomNavHint", { ns: "pwa" })}
                </p>
              </div>
              <Controller
                control={control}
                name="showBottomNav"
                render={({ field }) => (
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    aria-label={t("form.showBottomNavAria", { ns: "pwa" })}
                  />
                )}
              />
            </div>

            <Separator />

            <div className="flex flex-col gap-1">
              <p className="text-sm font-medium">
                {t("form.listingSectionLabel", { ns: "pwa" })}
              </p>
              <p className="text-xs text-muted-foreground">
                {t("form.listingSectionHint", { ns: "pwa" })}
              </p>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="pwa-developer">
                  {t("form.developerLabel", { ns: "pwa" })}
                </Label>
                <Input
                  id="pwa-developer"
                  placeholder={t("form.developerPlaceholder", { ns: "pwa" })}
                  {...register("developer")}
                  aria-invalid={!!errors.developer}
                />
                {errors.developer && (
                  <p className="text-xs text-danger">
                    {errors.developer.message}
                  </p>
                )}
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="pwa-downloads">
                  {t("form.downloadsLabelLabel", { ns: "pwa" })}
                </Label>
                <Input
                  id="pwa-downloads"
                  placeholder={t("form.downloadsLabelPlaceholder", {
                    ns: "pwa",
                  })}
                  {...register("downloadsLabel")}
                  aria-invalid={!!errors.downloadsLabel}
                />
                {errors.downloadsLabel && (
                  <p className="text-xs text-danger">
                    {errors.downloadsLabel.message}
                  </p>
                )}
              </div>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="pwa-description">
                {t("form.descriptionLabel", { ns: "pwa" })}
              </Label>
              <Textarea
                id="pwa-description"
                rows={3}
                placeholder={t("form.descriptionPlaceholder", { ns: "pwa" })}
                {...register("description")}
                aria-invalid={!!errors.description}
              />
              {errors.description && (
                <p className="text-xs text-danger">
                  {errors.description.message}
                </p>
              )}
            </div>

            <div className="grid gap-4 sm:grid-cols-3">
              <div className="grid gap-1.5">
                <Label htmlFor="pwa-rating">
                  {t("form.ratingLabel", { ns: "pwa" })}
                </Label>
                <Input
                  id="pwa-rating"
                  type="number"
                  min={0}
                  max={5}
                  step={0.1}
                  placeholder={t("form.ratingPlaceholder", { ns: "pwa" })}
                  {...register("rating", { setValueAs: numberOrUndefined })}
                  aria-invalid={!!errors.rating}
                />
                {errors.rating && (
                  <p className="text-xs text-danger">{errors.rating.message}</p>
                )}
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="pwa-rating-count">
                  {t("form.ratingCountLabel", { ns: "pwa" })}
                </Label>
                <Input
                  id="pwa-rating-count"
                  type="number"
                  min={0}
                  step={1}
                  placeholder={t("form.ratingCountPlaceholder", { ns: "pwa" })}
                  {...register("ratingCount", {
                    setValueAs: numberOrUndefined,
                  })}
                  aria-invalid={!!errors.ratingCount}
                />
                {errors.ratingCount && (
                  <p className="text-xs text-danger">
                    {errors.ratingCount.message}
                  </p>
                )}
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="pwa-size">
                  {t("form.sizeMbLabel", { ns: "pwa" })}
                </Label>
                <Input
                  id="pwa-size"
                  type="number"
                  min={0}
                  step={0.1}
                  placeholder={t("form.sizeMbPlaceholder", { ns: "pwa" })}
                  {...register("sizeMb", { setValueAs: numberOrUndefined })}
                  aria-invalid={!!errors.sizeMb}
                />
                {errors.sizeMb && (
                  <p className="text-xs text-danger">{errors.sizeMb.message}</p>
                )}
              </div>
            </div>

            <div className="grid gap-1.5">
              <Label>{t("form.ratingBreakdownLabel", { ns: "pwa" })}</Label>
              <p className="text-xs text-muted-foreground">
                {t("form.ratingBreakdownHint", { ns: "pwa" })}
              </p>
              <div className="grid grid-cols-5 gap-2">
                {STAR_FIELDS.map(({ n, name }) => (
                  <div key={name} className="grid gap-1">
                    <Label
                      htmlFor={`pwa-${name}`}
                      className="text-xs text-muted-foreground"
                    >
                      {t("form.ratingStarsShort", { ns: "pwa", n })}
                    </Label>
                    <Input
                      id={`pwa-${name}`}
                      type="number"
                      min={0}
                      max={100}
                      step={1}
                      placeholder="—"
                      {...register(name, { setValueAs: numberOrUndefined })}
                      aria-invalid={!!errors[name]}
                    />
                  </div>
                ))}
              </div>
              {STAR_FIELDS.map(
                ({ name }) =>
                  errors[name] && (
                    <p key={name} className="text-xs text-danger">
                      {errors[name]?.message}
                    </p>
                  ),
              )}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="pwa-whats-new">
                {t("form.whatsNewLabel", { ns: "pwa" })}
              </Label>
              <Textarea
                id="pwa-whats-new"
                rows={2}
                placeholder={t("form.whatsNewPlaceholder", { ns: "pwa" })}
                {...register("whatsNew")}
                aria-invalid={!!errors.whatsNew}
              />
              {errors.whatsNew && (
                <p className="text-xs text-danger">{errors.whatsNew.message}</p>
              )}
            </div>

            <div className="flex flex-col gap-3">
              <div>
                <Label>{t("form.detailsSectionLabel", { ns: "pwa" })}</Label>
                <p className="text-xs text-muted-foreground">
                  {t("form.detailsSectionHint", { ns: "pwa" })}
                </p>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label htmlFor="pwa-version">
                    {t("form.versionLabel", { ns: "pwa" })}
                  </Label>
                  <Input
                    id="pwa-version"
                    placeholder={t("form.versionPlaceholder", { ns: "pwa" })}
                    {...register("version")}
                    aria-invalid={!!errors.version}
                  />
                  {errors.version && (
                    <p className="text-xs text-danger">
                      {errors.version.message}
                    </p>
                  )}
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="pwa-age-rating">
                    {t("form.ageRatingLabel", { ns: "pwa" })}
                  </Label>
                  <Input
                    id="pwa-age-rating"
                    placeholder={t("form.ageRatingPlaceholder", { ns: "pwa" })}
                    {...register("ageRating")}
                    aria-invalid={!!errors.ageRating}
                  />
                  {errors.ageRating && (
                    <p className="text-xs text-danger">
                      {errors.ageRating.message}
                    </p>
                  )}
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="pwa-category">
                    {t("form.categoryLabel", { ns: "pwa" })}
                  </Label>
                  <Input
                    id="pwa-category"
                    placeholder={t("form.categoryPlaceholder", { ns: "pwa" })}
                    {...register("category")}
                    aria-invalid={!!errors.category}
                  />
                  {errors.category && (
                    <p className="text-xs text-danger">
                      {errors.category.message}
                    </p>
                  )}
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="pwa-updated-date">
                    {t("form.updatedDateLabel", { ns: "pwa" })}
                  </Label>
                  <Input
                    id="pwa-updated-date"
                    type="date"
                    {...register("updatedDate")}
                    aria-invalid={!!errors.updatedDate}
                  />
                  {errors.updatedDate && (
                    <p className="text-xs text-danger">
                      {errors.updatedDate.message}
                    </p>
                  )}
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="pwa-interactive">
                    {t("form.interactiveElementsLabel", { ns: "pwa" })}
                  </Label>
                  <Input
                    id="pwa-interactive"
                    placeholder={t("form.interactiveElementsPlaceholder", {
                      ns: "pwa",
                    })}
                    {...register("interactiveElements")}
                    aria-invalid={!!errors.interactiveElements}
                  />
                  {errors.interactiveElements && (
                    <p className="text-xs text-danger">
                      {errors.interactiveElements.message}
                    </p>
                  )}
                </div>
              </div>
              <div className="flex flex-col gap-3 sm:flex-row sm:gap-8">
                <div className="flex items-center gap-3">
                  <Controller
                    control={control}
                    name="containsAds"
                    render={({ field }) => (
                      <Switch
                        id="pwa-contains-ads"
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        aria-label={t("form.containsAdsLabel", { ns: "pwa" })}
                      />
                    )}
                  />
                  <Label htmlFor="pwa-contains-ads">
                    {t("form.containsAdsLabel", { ns: "pwa" })}
                  </Label>
                </div>
                <div className="flex items-center gap-3">
                  <Controller
                    control={control}
                    name="inAppPurchases"
                    render={({ field }) => (
                      <Switch
                        id="pwa-in-app-purchases"
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        aria-label={t("form.inAppPurchasesLabel", {
                          ns: "pwa",
                        })}
                      />
                    )}
                  />
                  <Label htmlFor="pwa-in-app-purchases">
                    {t("form.inAppPurchasesLabel", { ns: "pwa" })}
                  </Label>
                </div>
              </div>
            </div>

            <div className="flex flex-col gap-3">
              <div>
                <Label>{t("form.screenshotsLabel", { ns: "pwa" })}</Label>
                <p className="text-xs text-muted-foreground">
                  {t("form.screenshotsUploadHint", { ns: "pwa" })}
                </p>
                <p className="text-[11px] text-muted-foreground">
                  {t("form.screenshotsFormatsHint", { ns: "pwa" })}
                </p>
              </div>

              <div className="grid grid-cols-3 gap-2 sm:grid-cols-4 md:grid-cols-6">
                {screenshotArray.fields.map((field, index) => (
                  <div key={field.id} className="group relative aspect-[9/16] overflow-hidden rounded-lg border border-border bg-muted/30">
                    {manifest.screenshots?.[index]?.url ? (
                      /* eslint-disable-next-line @next/next/no-img-element -- a Content Gallery public object-storage URL (or any externally-hosted one), not a Next-optimizable static asset */
                      <img src={manifest.screenshots[index]?.url} alt="" className="size-full object-cover" />
                    ) : (
                      <div className="flex size-full items-center justify-center text-muted-foreground">
                        <ImagesIcon className="size-5" />
                      </div>
                    )}
                    <IconButton
                      aria-label={t("form.removeScreenshotAria", { ns: "pwa" })}
                      size="icon-xs"
                      variant="secondary"
                      className="absolute top-1 right-1 rounded-full opacity-0 shadow transition-opacity group-hover:opacity-100"
                      onClick={() => screenshotArray.remove(index)}
                    >
                      <XIcon className="size-3" />
                    </IconButton>
                  </div>
                ))}
                <GalleryUploadCell className="aspect-[9/16]" onUploaded={(file) => screenshotArray.append({ url: file.url })} />
              </div>

              {errors.screenshots?.message && (
                <p className="text-xs text-danger">
                  {errors.screenshots.message}
                </p>
              )}

              <Button type="button" variant="outline" size="sm" className="self-start" onClick={() => setPickingScreenshot(true)}>
                <ImagesIcon className="size-3.5" /> {t("form.addScreenshotFromGalleryButton", { ns: "pwa" })}
              </Button>

              <GalleryFilePickerDialog
                open={pickingScreenshot}
                onOpenChange={setPickingScreenshot}
                onPick={(file) => screenshotArray.append({ url: file.url })}
              />
            </div>

            <Separator />

            <div className="grid gap-1.5">
              <Label>{t("form.manifestPreviewLabel", { ns: "pwa" })}</Label>
              <pre className="overflow-x-auto rounded-md bg-muted p-2.5 text-xs">
                <Mono>
                  {JSON.stringify(
                    {
                      name: manifest.name || "",
                      short_name: manifest.shortName || "",
                      theme_color: manifest.themeColor || "",
                      background_color: manifest.backgroundColor || "",
                      start_url: manifest.startUrl || "",
                      display: "standalone",
                      icons: [
                        {
                          src: manifest.iconUrl || "",
                          sizes: "512x512",
                          type: "image/png",
                        },
                      ],
                    },
                    null,
                    2,
                  )}
                </Mono>
              </pre>
            </div>
          </CardContent>
        </Card>
        <Controller
          control={control}
          name="comments"
          render={({ field }) => (
            <PwaCommentsEditor
              value={field.value}
              onChange={field.onChange}
              activeSet={activeSet}
              onActiveSetChange={setActiveSet}
            />
          )}
        />
      </div>

      <div className="lg:sticky lg:top-4">
        <Card>
          <CardHeader className="flex flex-row items-start justify-between gap-2">
            <div>
              <CardTitle>{t("preview.title", { ns: "pwa" })}</CardTitle>
              <CardDescription>
                {t("preview.description", { ns: "pwa" })}
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Label
                htmlFor="pwa-preview-rtl"
                className="text-xs text-muted-foreground"
              >
                {t("preview.rtlToggleLabel", { ns: "pwa" })}
              </Label>
              <Switch
                id="pwa-preview-rtl"
                checked={previewDir === "rtl"}
                onCheckedChange={(checked) =>
                  setPreviewDir(checked ? "rtl" : "ltr")
                }
                aria-label={t("preview.rtlToggleLabel", { ns: "pwa" })}
              />
            </div>
          </CardHeader>
          <CardContent>
            <PwaPreviewCard
              t={t}
              name={manifest.name}
              developer={manifest.developer}
              iconUrl={manifest.iconUrl}
              accentColor={manifest.themeColor}
              backgroundColor={manifest.backgroundColor}
              themeAuto={manifest.themeAuto}
              showHeader={manifest.showHeader}
              showBottomNav={manifest.showBottomNav}
              headerTitle={manifest.headerTitle}
              headerLogoUrl={manifest.headerLogoUrl}
              description={manifest.description}
              rating={stats?.rating ?? manifest.rating}
              ratingCount={stats?.count ?? manifest.ratingCount}
              comments={setComments}
              downloadsLabel={manifest.downloadsLabel}
              sizeMb={manifest.sizeMb}
              screenshots={(manifest.screenshots ?? [])
                .map((s) => s?.url)
                .filter((u): u is string => !!u)}
              whatsNew={manifest.whatsNew}
              ratingStars5={stats?.stars[5] ?? manifest.ratingStars5}
              ratingStars4={stats?.stars[4] ?? manifest.ratingStars4}
              ratingStars3={stats?.stars[3] ?? manifest.ratingStars3}
              ratingStars2={stats?.stars[2] ?? manifest.ratingStars2}
              ratingStars1={stats?.stars[1] ?? manifest.ratingStars1}
              version={manifest.version}
              ageRating={manifest.ageRating}
              category={manifest.category}
              interactiveElements={manifest.interactiveElements}
              containsAds={manifest.containsAds}
              inAppPurchases={manifest.inAppPurchases}
              updatedDate={manifest.updatedDate}
              dir={previewDir}
            />
          </CardContent>
        </Card>
      </div>
      </fieldset>
    </form>
  );
}

"use client";

import Link from "next/link";
import { useState } from "react";
import { Controller, useFieldArray, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { ImagesIcon, PlusIcon, XIcon } from "lucide-react";
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
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Mono } from "@/components/ui/typography";
import type { PwaStatus } from "@/lib/api/pwa";
import { PwaPreviewCard } from "@/features/pwa/pwa-preview-card";
import { GalleryFilePickerDialog } from "@/features/content-gallery/gallery-file-picker-dialog";

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
    screenshots: z
      .array(
        z.object({
          url: z.url(t("form.validation.screenshotUrlInvalid", { ns: "pwa" })),
        }),
      )
      .max(12, t("form.validation.screenshotsMax", { ns: "pwa" })),
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

const STATUS_OPTIONS: PwaStatus[] = ["active", "paused", "archived"];
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
}: {
  title: string;
  defaultValues: Partial<PwaFormValues>;
  submitLabel: string;
  onSubmit: (values: PwaFormValues) => void;
}) {
  const { t } = useTranslation(["pwa", "common"]);
  const form = useForm<PwaFormValues>({
    resolver: zodResolver(buildPwaFormSchema(t)),
    defaultValues: {
      name: "",
      shortName: "",
      themeColor: "#16a34a",
      backgroundColor: "#0a0a0a",
      iconUrl: "",
      startUrl: "/install",
      bounceInAppWebview: true,
      status: "active",
      developer: "",
      description: "",
      downloadsLabel: "",
      screenshots: [],
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
  const [previewDir, setPreviewDir] = useState<"ltr" | "rtl">("ltr");
  const [pickingIcon, setPickingIcon] = useState(false);

  return (
    <form
      onSubmit={handleSubmit(onSubmit)}
      className="flex flex-col gap-4 lg:grid lg:grid-cols-[1fr_440px] lg:items-start lg:gap-6"
    >
      <div className="flex flex-col gap-4">
        <Card>
          <CardHeader>
            <CardTitle>{title}</CardTitle>
            <CardDescription>
              {t("form.description", { ns: "pwa" })}
            </CardDescription>
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
                <Label htmlFor="pwa-theme">
                  {t("form.themeColorLabel", { ns: "pwa" })}
                </Label>
                <div className="flex items-center gap-2">
                  <input
                    type="color"
                    value={
                      /^#[0-9a-fA-F]{6}$/.test(manifest.themeColor ?? "")
                        ? manifest.themeColor
                        : "#16a34a"
                    }
                    onChange={(e) =>
                      form.setValue("themeColor", e.target.value)
                    }
                    className="size-8 shrink-0 rounded-md border border-input bg-transparent"
                    aria-label={t("form.themeColorPickerAria", { ns: "pwa" })}
                  />
                  <Input
                    id="pwa-theme"
                    {...register("themeColor")}
                    className="font-mono text-xs"
                    aria-invalid={!!errors.themeColor}
                  />
                </div>
                {errors.themeColor && (
                  <p className="text-xs text-danger">
                    {errors.themeColor.message}
                  </p>
                )}
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="pwa-bg">
                  {t("form.backgroundColorLabel", { ns: "pwa" })}
                </Label>
                <div className="flex items-center gap-2">
                  <input
                    type="color"
                    value={
                      /^#[0-9a-fA-F]{6}$/.test(manifest.backgroundColor ?? "")
                        ? manifest.backgroundColor
                        : "#0a0a0a"
                    }
                    onChange={(e) =>
                      form.setValue("backgroundColor", e.target.value)
                    }
                    className="size-8 shrink-0 rounded-md border border-input bg-transparent"
                    aria-label={t("form.backgroundColorPickerAria", {
                      ns: "pwa",
                    })}
                  />
                  <Input
                    id="pwa-bg"
                    {...register("backgroundColor")}
                    className="font-mono text-xs"
                    aria-invalid={!!errors.backgroundColor}
                  />
                </div>
                {errors.backgroundColor && (
                  <p className="text-xs text-danger">
                    {errors.backgroundColor.message}
                  </p>
                )}
              </div>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="pwa-icon">
                {t("form.iconUrlLabel", { ns: "pwa" })}
              </Label>
              <div className="flex gap-1.5">
                <Input
                  id="pwa-icon"
                  placeholder={t("form.iconUrlPlaceholder", { ns: "pwa" })}
                  className="font-mono text-xs"
                  {...register("iconUrl")}
                  aria-invalid={!!errors.iconUrl}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  onClick={() => setPickingIcon(true)}
                  aria-label={t("form.iconUrlBrowseAria", { ns: "pwa" })}
                >
                  <ImagesIcon className="size-4" />
                </Button>
              </div>
              {errors.iconUrl && (
                <p className="text-xs text-danger">{errors.iconUrl.message}</p>
              )}
              <GalleryFilePickerDialog
                open={pickingIcon}
                onOpenChange={setPickingIcon}
                onPick={(file) =>
                  form.setValue("iconUrl", file.url, { shouldValidate: true, shouldDirty: true })
                }
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="pwa-start">
                {t("form.startUrlLabel", { ns: "pwa" })}
              </Label>
              <Input
                id="pwa-start"
                placeholder={t("form.startUrlPlaceholder", { ns: "pwa" })}
                className="font-mono text-xs"
                {...register("startUrl")}
                aria-invalid={!!errors.startUrl}
              />
              {errors.startUrl && (
                <p className="text-xs text-danger">{errors.startUrl.message}</p>
              )}
            </div>

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

            <div className="grid gap-1.5 sm:w-1/3">
              <Label htmlFor="pwa-status">
                {t("form.statusLabel", { ns: "pwa" })}
              </Label>
              <Controller
                control={control}
                name="status"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="pwa-status" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {STATUS_OPTIONS.map((s) => (
                        <SelectItem key={s} value={s}>
                          {t(`status.${s}`, { ns: "common" })}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
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
                  {t("form.screenshotsHint", { ns: "pwa" })}
                </p>
              </div>
              <div className="flex flex-col gap-2">
                {screenshotArray.fields.map((field, index) => (
                  <div key={field.id} className="flex items-center gap-2">
                    <Input
                      {...register(`screenshots.${index}.url`)}
                      placeholder={t("form.screenshotUrlPlaceholder", {
                        ns: "pwa",
                      })}
                      className="font-mono text-xs"
                      aria-invalid={!!errors.screenshots?.[index]?.url}
                    />
                    <IconButton
                      aria-label={t("form.removeScreenshotAria", { ns: "pwa" })}
                      size="icon-sm"
                      onClick={() => screenshotArray.remove(index)}
                    >
                      <XIcon className="size-3.5" />
                    </IconButton>
                  </div>
                ))}
              </div>
              {errors.screenshots?.message && (
                <p className="text-xs text-danger">
                  {errors.screenshots.message}
                </p>
              )}
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="self-start"
                disabled={screenshotArray.fields.length >= 12}
                onClick={() => screenshotArray.append({ url: "" })}
              >
                <PlusIcon className="size-3.5" />{" "}
                {t("form.addScreenshotButton", { ns: "pwa" })}
              </Button>
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

        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" asChild>
            <Link href="/pwa">{t("actions.cancel", { ns: "common" })}</Link>
          </Button>
          <Button type="submit" disabled={isSubmitting}>
            {submitLabel}
          </Button>
        </div>
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
              description={manifest.description}
              rating={manifest.rating}
              ratingCount={manifest.ratingCount}
              downloadsLabel={manifest.downloadsLabel}
              sizeMb={manifest.sizeMb}
              screenshots={(manifest.screenshots ?? [])
                .map((s) => s?.url)
                .filter((u): u is string => !!u)}
              whatsNew={manifest.whatsNew}
              ratingStars5={manifest.ratingStars5}
              ratingStars4={manifest.ratingStars4}
              ratingStars3={manifest.ratingStars3}
              ratingStars2={manifest.ratingStars2}
              ratingStars1={manifest.ratingStars1}
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
    </form>
  );
}

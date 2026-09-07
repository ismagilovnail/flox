"use client";

import Link from "next/link";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MultiSelect } from "@/components/ui/multi-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  PIXEL_EVENT_TYPES,
  PIXEL_PROVIDERS,
  PIXEL_PROVIDER_I18N_KEY,
  type PixelEventType,
  type PixelProvider,
  type PixelStatus,
} from "@/lib/api/pixels";

/** Factory, not a module-level const — Zod's validation messages are
 * user-facing text and need the live translator, which only exists inside
 * a component. */
export function buildPixelFormSchema(t: TFunction) {
  return z.object({
    name: z.string().min(2, t("form.validation.nameMin", { ns: "pixels" })).max(100),
    provider: z.enum(PIXEL_PROVIDERS as [PixelProvider, ...PixelProvider[]]),
    pixelId: z.string().max(80, t("form.validation.pixelIdMax", { ns: "pixels" })),
    events: z.array(z.enum(PIXEL_EVENT_TYPES)).min(1, t("form.validation.eventsRequired", { ns: "pixels" })),
    status: z.enum(["active", "paused", "archived"] as [PixelStatus, ...PixelStatus[]]),
  });
}

export type PixelFormValues = z.infer<ReturnType<typeof buildPixelFormSchema>>;

const STATUS_OPTIONS: PixelStatus[] = ["active", "paused", "archived"];
// Event codes are canonical §43 event-model identifiers, not UI text —
// deliberately left untranslated, same treatment as the Event Mappings
// panel and Postlanding's form.
const EVENT_OPTIONS = PIXEL_EVENT_TYPES.map((e) => ({ value: e, label: e }));

/** The pixel create/edit form — its own page (features/pixels/{new-pixel-
 * view,pixel-detail-view}.tsx) instead of a slide-out Sheet, matching the
 * offers/traffic-source/campaign create/edit pages' layout. Same fields/
 * validation/mutation shape the Sheet version had; only the container
 * changed. */
export function PixelForm({
  title,
  defaultValues,
  submitLabel,
  onSubmit,
  disabled = false,
}: {
  title: string;
  defaultValues: Partial<PixelFormValues>;
  submitLabel: string;
  onSubmit: (values: PixelFormValues) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation(["pixels", "common"]);
  const form = useForm<PixelFormValues>({
    resolver: zodResolver(buildPixelFormSchema(t)),
    defaultValues: { name: "", provider: "facebook", pixelId: "", events: [], status: "active", ...defaultValues },
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
      <fieldset disabled={disabled} className="contents">
      <Card>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
          <CardDescription>{t("form.description", { ns: "pixels" })}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="pxl-name">{t("form.nameLabel", { ns: "pixels" })}</Label>
              <Input
                id="pxl-name"
                placeholder={t("form.namePlaceholder", { ns: "pixels" })}
                {...register("name")}
                aria-invalid={!!errors.name}
              />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="pxl-provider">{t("form.providerLabel", { ns: "pixels" })}</Label>
              <Controller
                control={control}
                name="provider"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="pxl-provider" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {PIXEL_PROVIDERS.map((p) => (
                        <SelectItem key={p} value={p}>
                          {t(PIXEL_PROVIDER_I18N_KEY[p], { ns: "pixels" })}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="pxl-id">{t("form.pixelIdLabel", { ns: "pixels" })}</Label>
            <Input
              id="pxl-id"
              placeholder={t("form.pixelIdPlaceholder", { ns: "pixels" })}
              className="font-mono text-xs"
              {...register("pixelId")}
              aria-invalid={!!errors.pixelId}
            />
            {errors.pixelId && <p className="text-xs text-danger">{errors.pixelId.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label>{t("form.eventsLabel", { ns: "pixels" })}</Label>
            <Controller
              control={control}
              name="events"
              render={({ field }) => (
                <MultiSelect
                  label={t("form.eventsLabel", { ns: "pixels" })}
                  options={EVENT_OPTIONS}
                  selected={field.value}
                  onChange={(values) => field.onChange(values as PixelEventType[])}
                  className="w-full"
                />
              )}
            />
            {errors.events && <p className="text-xs text-danger">{errors.events.message}</p>}
          </div>

          <div className="grid gap-1.5 sm:w-1/3">
            <Label htmlFor="pxl-status">{t("form.statusLabel", { ns: "pixels" })}</Label>
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="pxl-status" className="w-full">
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
        </CardContent>
      </Card>
      </fieldset>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" asChild>
          <Link href="/pixels">{t("actions.cancel", { ns: "common" })}</Link>
        </Button>
        <Button type="submit" disabled={disabled || isSubmitting}>
          {submitLabel}
        </Button>
      </div>
    </form>
  );
}

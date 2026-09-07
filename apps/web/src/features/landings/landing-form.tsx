"use client";

import Link from "next/link";
import { Controller, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Mono } from "@/components/ui/typography";
import { slugify } from "@/lib/utils";
import type { LandingStatus, LandingType } from "@/lib/api/landings";

/** Factory, not a module-level const — Zod's validation messages are
 * user-facing text and need the live translator, which only exists inside
 * a component. */
export function buildLandingFormSchema(t: TFunction) {
  return z
    .object({
      name: z.string().min(2, t("form.validation.nameMin", { ns: "landings" })).max(100),
      type: z.enum(["internal", "external"] as [LandingType, ...LandingType[]]),
      url: z.string().optional(),
      content: z.string().optional(),
      status: z.enum(["active", "paused", "archived"] as [LandingStatus, ...LandingStatus[]]),
    })
    .superRefine((data, ctx) => {
      if (data.type === "external") {
        if (!data.url || !z.url().safeParse(data.url).success) {
          ctx.addIssue({ code: "custom", path: ["url"], message: t("form.validation.urlInvalid", { ns: "landings" }) });
        }
      } else if (!data.content || data.content.trim().length === 0) {
        ctx.addIssue({ code: "custom", path: ["content"], message: t("form.validation.contentRequired", { ns: "landings" }) });
      }
    });
}

export type LandingFormValues = z.infer<ReturnType<typeof buildLandingFormSchema>>;

const STATUS_OPTIONS: LandingStatus[] = ["active", "paused", "archived"];

/** The landing create/edit form — its own page (features/landings/{new-
 * landing-view,landing-detail-view}.tsx) instead of a slide-out Sheet,
 * matching the traffic-source/offer/campaign create/edit pages' layout.
 * Same fields/validation/mutation shape the Sheet version had; only the
 * container changed. */
export function LandingForm({
  title,
  defaultValues,
  submitLabel,
  onSubmit,
  disabled = false,
}: {
  title: string;
  defaultValues: Partial<LandingFormValues>;
  submitLabel: string;
  onSubmit: (values: LandingFormValues) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation(["landings", "common"]);
  const form = useForm<LandingFormValues>({
    resolver: zodResolver(buildLandingFormSchema(t)),
    defaultValues: { name: "", type: "internal", url: "", content: "", status: "active", ...defaultValues },
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  const type = useWatch({ control, name: "type" });
  const name = useWatch({ control, name: "name" });

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
          <CardDescription>{t("form.description", { ns: "landings" })}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="lnd-name">{t("form.nameLabel", { ns: "landings" })}</Label>
              <Input
                id="lnd-name"
                placeholder={t("form.namePlaceholder", { ns: "landings" })}
                {...register("name")}
                aria-invalid={!!errors.name}
                disabled={disabled}
              />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="lnd-type">{t("form.typeLabel", { ns: "landings" })}</Label>
              <Controller
                control={control}
                name="type"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange} disabled={disabled}>
                    <SelectTrigger id="lnd-type" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="internal">{t("type.internal", { ns: "landings" })}</SelectItem>
                      <SelectItem value="external">{t("type.external", { ns: "landings" })}</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          {type === "external" ? (
            <div className="grid gap-1.5">
              <Label htmlFor="lnd-url">{t("form.urlLabel", { ns: "landings" })}</Label>
              <Input
                id="lnd-url"
                placeholder={t("form.urlPlaceholder", { ns: "landings" })}
                className="font-mono text-xs"
                {...register("url")}
                aria-invalid={!!errors.url}
                disabled={disabled}
              />
              {errors.url && <p className="text-xs text-danger">{errors.url.message}</p>}
            </div>
          ) : (
            <>
              <div className="grid gap-1.5">
                <Label htmlFor="lnd-content">{t("form.contentLabel", { ns: "landings" })}</Label>
                <Textarea
                  id="lnd-content"
                  placeholder={t("form.contentPlaceholder", { ns: "landings" })}
                  className="min-h-28 font-mono text-xs"
                  {...register("content")}
                  aria-invalid={!!errors.content}
                  disabled={disabled}
                />
                {errors.content && <p className="text-xs text-danger">{errors.content.message}</p>}
              </div>
              <div className="grid gap-1">
                <Label>{t("form.hostedUrlLabel", { ns: "landings" })}</Label>
                <Mono className="text-xs text-muted-foreground">
                  https://cdn.floxlink.io/lnd/{slugify(name || "untitled")}
                </Mono>
              </div>
            </>
          )}

          <div className="grid gap-1.5 sm:w-1/3">
            <Label htmlFor="lnd-status">{t("form.statusLabel", { ns: "landings" })}</Label>
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange} disabled={disabled}>
                  <SelectTrigger id="lnd-status" className="w-full">
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

      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" asChild>
          <Link href="/landings">{t("actions.cancel", { ns: "common" })}</Link>
        </Button>
        <Button type="submit" disabled={disabled || isSubmitting}>
          {submitLabel}
        </Button>
      </div>
    </form>
  );
}

"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { type CampaignStatus } from "@/lib/api/campaigns";
import { useTrafficSources } from "@/hooks/use-traffic-sources";

/** A factory, not a module-level const — see the identical pattern (and
 * rationale) in features/traffic-sources/source-form-sheet.tsx. */
export function buildCampaignFormSchema(t: TFunction) {
  return z.object({
    name: z.string().min(2, t("form.validation.nameMin", { ns: "campaigns" })).max(80),
    trafficSourceId: z.string().min(1, t("form.validation.sourceRequired", { ns: "campaigns" })),
    fallbackUrl: z.url(t("form.validation.urlInvalid", { ns: "campaigns" })),
    externalCampaignId: z.string().max(200, t("form.validation.externalCampaignIdMax", { ns: "campaigns" })).optional(),
    notes: z.string().max(500).optional(),
    status: z.enum(["active", "paused", "draft", "archived"] as [CampaignStatus, ...CampaignStatus[]]).optional(),
  });
}

export type CampaignFormValues = z.infer<ReturnType<typeof buildCampaignFormSchema>>;

const STATUS_OPTIONS: CampaignStatus[] = ["draft", "active", "paused", "archived"];

export function CampaignForm({
  defaultValues,
  showStatus = false,
  submitLabel,
  onSubmit,
}: {
  defaultValues: Partial<CampaignFormValues>;
  showStatus?: boolean;
  submitLabel?: string;
  onSubmit: (values: CampaignFormValues) => void;
}) {
  const { t } = useTranslation(["campaigns", "common"]);
  const sourcesQuery = useTrafficSources();
  const sources = sourcesQuery.data?.trafficSources ?? [];

  const form = useForm<CampaignFormValues>({
    resolver: zodResolver(buildCampaignFormSchema(t)),
    values: {
      name: "",
      trafficSourceId: sources[0]?.id ?? "",
      fallbackUrl: "",
      externalCampaignId: "",
      notes: "",
      status: "draft",
      ...defaultValues,
    },
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{t("form.detailsTitle", { ns: "campaigns" })}</CardTitle>
          <CardDescription>{t("form.detailsDescription", { ns: "campaigns" })}</CardDescription>
        </CardHeader>
        <CardContent className="grid max-w-lg gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="name">{t("form.nameLabel", { ns: "campaigns" })}</Label>
            <Input
              id="name"
              placeholder={t("form.namePlaceholder", { ns: "campaigns" })}
              {...register("name")}
              aria-invalid={!!errors.name}
            />
            {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="trafficSourceId">{t("form.sourceLabel", { ns: "campaigns" })}</Label>
            {sourcesQuery.isPending ? (
              <p className="text-xs text-muted-foreground">{t("form.sourceLoading", { ns: "campaigns" })}</p>
            ) : sourcesQuery.isError ? (
              <p className="text-xs text-danger">
                {t("form.sourceLoadError", { ns: "campaigns", message: sourcesQuery.error.message })}
              </p>
            ) : sources.length === 0 ? (
              <p className="text-xs text-muted-foreground">{t("form.sourceEmpty", { ns: "campaigns" })}</p>
            ) : (
              <Controller
                control={control}
                name="trafficSourceId"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="trafficSourceId" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {sources.map((s) => (
                        <SelectItem key={s.id} value={s.id}>
                          {s.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            )}
            {errors.trafficSourceId && <p className="text-xs text-danger">{errors.trafficSourceId.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="fallbackUrl">{t("form.fallbackUrlLabel", { ns: "campaigns" })}</Label>
            <Input
              id="fallbackUrl"
              placeholder={t("form.fallbackUrlPlaceholder", { ns: "campaigns" })}
              {...register("fallbackUrl")}
              aria-invalid={!!errors.fallbackUrl}
            />
            {errors.fallbackUrl && <p className="text-xs text-danger">{errors.fallbackUrl.message}</p>}
            <p className="text-xs text-muted-foreground">{t("form.fallbackUrlHint", { ns: "campaigns" })}</p>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="externalCampaignId">{t("form.externalCampaignIdLabel", { ns: "campaigns" })}</Label>
            <Input
              id="externalCampaignId"
              placeholder={t("form.externalCampaignIdPlaceholder", { ns: "campaigns" })}
              {...register("externalCampaignId")}
              aria-invalid={!!errors.externalCampaignId}
            />
            {errors.externalCampaignId && (
              <p className="text-xs text-danger">{errors.externalCampaignId.message}</p>
            )}
            <p className="text-xs text-muted-foreground">{t("form.externalCampaignIdHint", { ns: "campaigns" })}</p>
          </div>

          {showStatus && (
            <div className="grid gap-1.5">
              <Label htmlFor="status">{t("form.statusLabel", { ns: "campaigns" })}</Label>
              <Controller
                control={control}
                name="status"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="status" className="w-full">
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
          )}

          <div className="grid gap-1.5">
            <Label htmlFor="notes">{t("form.notesLabel", { ns: "campaigns" })}</Label>
            <Textarea id="notes" placeholder={t("form.notesPlaceholder", { ns: "campaigns" })} {...register("notes")} />
          </div>
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button type="submit" disabled={isSubmitting || sources.length === 0}>
          {submitLabel ?? t("form.submitDefault", { ns: "campaigns" })}
        </Button>
      </div>
    </form>
  );
}

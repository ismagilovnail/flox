"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { MacroPicker } from "@/components/shared/macro-picker";
import {
  COST_INTEGRATION_I18N_KEY,
  SOURCE_TYPE_I18N_KEY,
  SOURCE_TYPES,
  type CostIntegration,
  type SourceStatus,
  type SourceType,
} from "@/lib/api/traffic-sources";

const COST_INTEGRATIONS: CostIntegration[] = ["none", "manual", "facebook_ads", "tiktok_ads"];

/** A factory, not a module-level const: Zod's validation messages are
 * user-facing text and need the live translator, which only exists inside
 * a component. The runtime *shape* (and therefore SourceFormValues, below)
 * never changes across locales — only the message strings do. */
export function buildSourceFormSchema(t: TFunction) {
  return z.object({
    name: z.string().min(2, t("form.validation.nameMin", { ns: "trafficSources" })).max(80),
    type: z.enum(SOURCE_TYPES as [SourceType, ...SourceType[]]),
    trackingTemplate: z.url(t("form.validation.urlInvalid", { ns: "trafficSources" })),
    costIntegration: z.enum(COST_INTEGRATIONS as [CostIntegration, ...CostIntegration[]]),
    status: z.enum(["active", "paused", "archived"] as [SourceStatus, ...SourceStatus[]]).optional(),
  });
}

export type SourceFormValues = z.infer<ReturnType<typeof buildSourceFormSchema>>;

const STATUS_OPTIONS: SourceStatus[] = ["active", "paused", "archived"];

export function SourceFormSheet({
  open,
  onOpenChange,
  defaultValues,
  title,
  submitLabel,
  showStatus = false,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<SourceFormValues>;
  title: string;
  submitLabel: string;
  showStatus?: boolean;
  onSubmit: (values: SourceFormValues) => void;
}) {
  const { t } = useTranslation(["trafficSources", "common"]);
  const form = useForm<SourceFormValues>({
    resolver: zodResolver(buildSourceFormSchema(t)),
    values: {
      name: "",
      type: SOURCE_TYPES[0],
      trackingTemplate: "",
      costIntegration: "none",
      status: "active",
      ...defaultValues,
    },
  });

  const {
    register,
    handleSubmit,
    control,
    setValue,
    getValues,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-lg" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{t("form.description", { ns: "trafficSources" })}</SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="src-name">{t("form.nameLabel", { ns: "trafficSources" })}</Label>
              <Input
                id="src-name"
                placeholder={t("form.namePlaceholder", { ns: "trafficSources" })}
                {...register("name")}
                aria-invalid={!!errors.name}
              />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="src-type">{t("form.typeLabel", { ns: "trafficSources" })}</Label>
              <Controller
                control={control}
                name="type"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="src-type" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {SOURCE_TYPES.map((type) => (
                        <SelectItem key={type} value={type}>
                          {t(SOURCE_TYPE_I18N_KEY[type], { ns: "trafficSources" })}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          <div className="grid gap-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="src-template">{t("form.trackingTemplateLabel", { ns: "trafficSources" })}</Label>
              <MacroPicker
                onInsert={(token) => setValue("trackingTemplate", `${getValues("trackingTemplate")}${token}`)}
              />
            </div>
            <Input
              id="src-template"
              placeholder={t("form.trackingTemplatePlaceholder", { ns: "trafficSources" })}
              className="font-mono text-xs"
              {...register("trackingTemplate")}
              aria-invalid={!!errors.trackingTemplate}
            />
            {errors.trackingTemplate && <p className="text-xs text-danger">{errors.trackingTemplate.message}</p>}
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="src-cost">{t("form.costIntegrationLabel", { ns: "trafficSources" })}</Label>
              <Controller
                control={control}
                name="costIntegration"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="src-cost" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {COST_INTEGRATIONS.map((c) => (
                        <SelectItem key={c} value={c}>
                          {t(COST_INTEGRATION_I18N_KEY[c], { ns: "trafficSources" })}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              <p className="text-xs text-muted-foreground">{t("form.costIntegrationHint", { ns: "trafficSources" })}</p>
            </div>

            {showStatus && (
              <div className="grid gap-1.5">
                <Label htmlFor="src-status">{t("form.statusLabel", { ns: "trafficSources" })}</Label>
                <Controller
                  control={control}
                  name="status"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="src-status" className="w-full">
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
          </div>

          <SheetFooter className="mt-0 flex-row justify-end gap-2 p-0">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("actions.cancel", { ns: "common" })}
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {submitLabel}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}

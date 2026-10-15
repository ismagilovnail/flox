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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { MacroPicker } from "@/components/shared/macro-picker";
import type { NetworkStatus } from "@/lib/api/networks";
import { nameField } from "@/schemas/fields";
import { CURRENCIES } from "@/lib/countries";

/** Factory, not a module-level const — see source-form.tsx's
 * buildSourceFormSchema for why (Zod messages are user-facing text and
 * need the live translator). */
export function buildNetworkFormSchema(t: TFunction) {
  return z.object({
    name: nameField(t("form.validation.nameMin", { ns: "networks" }), { max: 80 }),
    // Optional (audit 2026-10-15, UX-2): empty means no outgoing postback.
    postbackUrl: z.union([z.literal(""), z.url(t("form.validation.urlInvalid", { ns: "networks" }))]),
    acceptDuplicates: z.boolean(),
    currency: z.string().regex(/^[A-Z]{3}$/, t("form.validation.currencyInvalid", { ns: "networks" })),
    status: z.enum(["active", "paused", "archived"] as [NetworkStatus, ...NetworkStatus[]]).optional(),
  });
}

export type NetworkFormValues = z.infer<ReturnType<typeof buildNetworkFormSchema>>;

const STATUS_OPTIONS: NetworkStatus[] = ["active", "paused", "archived"];

/** The network create/edit form — its own page (features/networks/{new-
 * network-view,network-detail-view}.tsx) instead of a slide-out Sheet,
 * matching the traffic-source/offer/campaign create/edit pages' layout.
 * Same fields/validation/mutation shape the Sheet version had; only the
 * container changed. The post-create postback-secret reveal step lives in
 * new-network-view.tsx, not here — this component only ever collects
 * field values and hands them to onSubmit. */
export function NetworkForm({
  title,
  defaultValues,
  submitLabel,
  showStatus = false,
  onSubmit,
  disabled = false,
}: {
  title: string;
  defaultValues: Partial<NetworkFormValues>;
  submitLabel: string;
  showStatus?: boolean;
  onSubmit: (values: NetworkFormValues) => void;
  /** Locks every field read-only (offer.write) via a native <fieldset
   * disabled>, same technique offer-form.tsx/stream-set-editor.tsx use. */
  disabled?: boolean;
}) {
  const { t } = useTranslation(["networks", "common"]);
  const form = useForm<NetworkFormValues>({
    resolver: zodResolver(buildNetworkFormSchema(t)),
    defaultValues: { name: "", postbackUrl: "", acceptDuplicates: false, currency: "USD", status: "active", ...defaultValues },
  });
  // A network saved with a currency outside the short list keeps it as an
  // option, same as the organization's currency (organization-panel.tsx).
  const current = defaultValues.currency;
  const currencyOptions = current && !CURRENCIES.includes(current) ? [current, ...CURRENCIES] : CURRENCIES;

  const {
    register,
    handleSubmit,
    control,
    setValue,
    getValues,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
          <CardDescription>{t("form.description", { ns: "networks" })}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
        <fieldset disabled={disabled} className="contents">
          {disabled && (
            <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
              {t("permissionDenied.title", { ns: "common" })}
            </p>
          )}
          <div className="grid gap-1.5">
            <Label htmlFor="net-name">{t("form.nameLabel", { ns: "networks" })}</Label>
            <Input
              id="net-name"
              placeholder={t("form.namePlaceholder", { ns: "networks" })}
              {...register("name")}
              aria-invalid={!!errors.name}
            />
            {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="net-postback">{t("form.postbackUrlLabel", { ns: "networks" })}</Label>
              <MacroPicker onInsert={(token) => setValue("postbackUrl", `${getValues("postbackUrl")}${token}`)} />
            </div>
            <Input
              id="net-postback"
              placeholder={t("form.postbackUrlPlaceholder", { ns: "networks" })}
              className="font-mono text-xs"
              {...register("postbackUrl")}
              aria-invalid={!!errors.postbackUrl}
            />
            {errors.postbackUrl && <p className="text-xs text-danger">{errors.postbackUrl.message}</p>}
            <p className="text-xs text-muted-foreground">{t("form.postbackUrlHint", { ns: "networks" })}</p>
          </div>

          <div className="grid gap-1.5 sm:w-1/3">
            <Label htmlFor="net-currency">{t("form.currencyLabel", { ns: "networks" })}</Label>
            <Controller
              control={control}
              name="currency"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange} disabled={disabled}>
                  <SelectTrigger id="net-currency" className="w-full font-mono" aria-invalid={!!errors.currency}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {currencyOptions.map((c) => (
                      <SelectItem key={c} value={c} className="font-mono">
                        {c}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
            {errors.currency && <p className="text-xs text-danger">{errors.currency.message}</p>}
          </div>
          <p className="-mt-2 text-xs text-muted-foreground">{t("form.currencyHint", { ns: "networks" })}</p>

          <div className="flex items-center justify-between rounded-md border border-border p-2.5">
            <div>
              <p className="text-sm font-medium">{t("form.acceptDuplicatesLabel", { ns: "networks" })}</p>
              <p className="text-xs text-muted-foreground">{t("form.acceptDuplicatesHint", { ns: "networks" })}</p>
            </div>
            <Controller
              control={control}
              name="acceptDuplicates"
              render={({ field }) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  aria-label={t("form.acceptDuplicatesAria", { ns: "networks" })}
                />
              )}
            />
          </div>

          {showStatus && (
            <div className="grid gap-1.5 sm:w-1/3">
              <Label htmlFor="net-status">{t("form.statusLabel", { ns: "networks" })}</Label>
              <Controller
                control={control}
                name="status"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="net-status" className="w-full">
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
        </fieldset>
        </CardContent>
      </Card>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" asChild>
          <Link href="/networks">{t("actions.cancel", { ns: "common" })}</Link>
        </Button>
        <Button type="submit" disabled={disabled || isSubmitting}>
          {submitLabel}
        </Button>
      </div>
    </form>
  );
}

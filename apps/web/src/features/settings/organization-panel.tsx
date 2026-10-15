"use client";

import * as React from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useOrganization, useUpdateOrganization } from "@/hooks/use-organization";
import { TimezoneSelect } from "@/components/ui/timezone-select";
import { CURRENCIES } from "@/lib/countries";
import { nameField } from "@/schemas/fields";

// Plain non-empty strings, not a z.enum against the curated
// TIMEZONES/CURRENCIES dropdown lists — the real backend
// (auth.Service.UpdateOrganizationSettings) validates far more broadly
// (any IANA timezone via time.LoadLocation, any 3-letter currency code),
// so an org already set to a value outside these two short lists must
// still load and save correctly, not get rejected by a stricter
// frontend-only check the backend itself doesn't share.
function organizationSchema(t: TFunction<"settings">) {
  return z.object({
    name: nameField(t("organization.errors.name"), { max: 80 }),
    timezone: z.string().min(1, t("organization.errors.timezone")),
    currency: z.string().min(1, t("organization.errors.currency")),
  });
}

type OrganizationValues = z.infer<ReturnType<typeof organizationSchema>>;

function OrganizationForm({ org }: { org: OrganizationValues }) {
  const { t } = useTranslation("settings");
  const updateOrganization = useUpdateOrganization();

  const form = useForm<OrganizationValues>({
    resolver: zodResolver(organizationSchema(t)),
    defaultValues: org,
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  function onSubmit(values: OrganizationValues) {
    updateOrganization.mutate(values, {
      onSuccess: () => toast(t("organization.saved")),
      onError: (err) => toast.error(t("organization.saveError"), { description: err.message }),
    });
  }

  // The current value might not be one of the curated shortlist entries
  // (a real org set via some other path, or just a currency/timezone
  // this UI hasn't added to its own dropdown yet) — always keep it
  // selectable rather than silently falling back to nothing.
  const currencyOptions = CURRENCIES.includes(org.currency) ? CURRENCIES : [org.currency, ...CURRENCIES];

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="grid max-w-md gap-4">
      <div className="grid gap-1.5">
        <Label htmlFor="org-name">{t("organization.name")}</Label>
        <Input id="org-name" {...register("name")} aria-invalid={!!errors.name} />
        {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="org-timezone">{t("organization.timezone")}</Label>
        <Controller
          control={control}
          name="timezone"
          render={({ field }) => (
            <TimezoneSelect id="org-timezone" value={field.value} onValueChange={field.onChange} className="w-full" />
          )}
        />
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="org-currency">{t("organization.currency")}</Label>
        <Controller
          control={control}
          name="currency"
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger id="org-currency" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {currencyOptions.map((c) => (
                  <SelectItem key={c} value={c}>
                    {c}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
      </div>

      <Button type="submit" disabled={isSubmitting} className="w-fit">
        {t("organization.save")}
      </Button>
    </form>
  );
}

export function OrganizationPanel() {
  const { t } = useTranslation("settings");
  const orgQuery = useOrganization();

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("organization.title")}</CardTitle>
        <CardDescription>{t("organization.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        {orgQuery.isPending && <LoadingState label={t("organization.loading")} />}
        {orgQuery.isError && (
          <ErrorState
            title={t("organization.loadError")}
            description={orgQuery.error.message}
            onRetry={() => orgQuery.refetch()}
          />
        )}
        {orgQuery.data && <OrganizationForm key={orgQuery.data.id} org={orgQuery.data} />}
      </CardContent>
    </Card>
  );
}

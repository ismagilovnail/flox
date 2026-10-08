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
  DNS_PROVIDER_LABELS,
  REGISTRAR_LABELS,
  type DnsProvider,
  type DomainPurpose,
  type DomainStatus,
  type Registrar,
} from "@/lib/api/domains";

const DNS_PROVIDERS: DnsProvider[] = ["cloudflare", "route53", "unmanaged"];
const REGISTRARS: Registrar[] = ["namecheap", "godaddy", "cloudflare_registrar", "unmanaged"];
const STATUS_OPTIONS: DomainStatus[] = ["active", "pending", "error", "expired"];
const PURPOSES: DomainPurpose[] = ["tracking", "pwa", "fallback"];

export function domainFormSchema(t: TFunction<"domains">) {
  return z.object({
    domain: z.string().regex(/^([a-z0-9-]+\.)+[a-z]{2,}$/i, t("form.errors.domain")),
    purpose: z.array(z.enum(PURPOSES as [DomainPurpose, ...DomainPurpose[]])).min(1, t("form.errors.purpose")),
    registrar: z.enum(REGISTRARS as [Registrar, ...Registrar[]]),
    dnsProvider: z.enum(DNS_PROVIDERS as [DnsProvider, ...DnsProvider[]]),
    status: z.enum(STATUS_OPTIONS as [DomainStatus, ...DomainStatus[]]),
  });
}

export type DomainFormValues = z.infer<ReturnType<typeof domainFormSchema>>;

/** The domain create/edit form — its own page (features/domains/{new-
 * domain-view,domain-detail-view}.tsx) instead of a slide-out Sheet,
 * matching the Offers/Traffic-Sources/Campaigns create/edit pages'
 * layout. Same fields/validation/mutation shape the Sheet version had;
 * only the container changed. */
export function DomainForm({
  title,
  defaultValues,
  submitLabel,
  onSubmit,
  disabled = false,
}: {
  title: string;
  defaultValues: Partial<DomainFormValues>;
  submitLabel: string;
  onSubmit: (values: DomainFormValues) => void;
  /** Locks the submit button and every field, including MultiSelect
   * (Purpose), when the caller lacks settings.write. */
  disabled?: boolean;
}) {
  const { t } = useTranslation(["domains", "common"]);
  const purposeOptions = PURPOSES.map((value) => ({ value, label: t(`purposes.${value}`) }));
  const form = useForm<DomainFormValues>({
    resolver: zodResolver(domainFormSchema(t)),
    defaultValues: {
      domain: "",
      purpose: ["tracking"],
      registrar: "unmanaged",
      dnsProvider: "unmanaged",
      status: "pending",
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
          <CardTitle>{title}</CardTitle>
          <CardDescription>{t("form.description")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="dom-domain">{t("columns.domain")}</Label>
            <Input
              id="dom-domain"
              placeholder="track.example.com"
              className="font-mono text-sm"
              {...register("domain")}
              aria-invalid={!!errors.domain}
              disabled={disabled}
            />
            {errors.domain && <p className="text-xs text-danger">{errors.domain.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label>{t("columns.purpose")}</Label>
            <Controller
              control={control}
              name="purpose"
              render={({ field }) => (
                <MultiSelect
                  label={t("columns.purpose")}
                  options={purposeOptions}
                  selected={field.value}
                  onChange={(values) => field.onChange(values as DomainPurpose[])}
                  className="w-full"
                  disabled={disabled}
                />
              )}
            />
            {errors.purpose && <p className="text-xs text-danger">{errors.purpose.message}</p>}
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="dom-registrar">{t("form.registrar")}</Label>
              <Controller
                control={control}
                name="registrar"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange} disabled={disabled}>
                    <SelectTrigger id="dom-registrar" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {REGISTRARS.map((r) => (
                        <SelectItem key={r} value={r}>
                          {r === "unmanaged" ? t("form.unmanaged") : REGISTRAR_LABELS[r]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="dom-dns">{t("form.dnsProvider")}</Label>
              <Controller
                control={control}
                name="dnsProvider"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange} disabled={disabled}>
                    <SelectTrigger id="dom-dns" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {DNS_PROVIDERS.map((d) => (
                        <SelectItem key={d} value={d}>
                          {d === "unmanaged" ? t("form.unmanaged") : DNS_PROVIDER_LABELS[d]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          <div className="grid gap-1.5 sm:w-1/2">
            <Label htmlFor="dom-status">{t("columns.status")}</Label>
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange} disabled={disabled}>
                  <SelectTrigger id="dom-status" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {STATUS_OPTIONS.map((s) => (
                      <SelectItem key={s} value={s}>
                        {t(`statuses.${s}`)}
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
          <Link href="/domains">{t("common:actions.cancel")}</Link>
        </Button>
        <Button type="submit" disabled={disabled || isSubmitting}>
          {submitLabel}
        </Button>
      </div>
    </form>
  );
}

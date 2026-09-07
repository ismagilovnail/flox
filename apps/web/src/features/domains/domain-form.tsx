"use client";

import Link from "next/link";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

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
const PURPOSE_OPTIONS: { value: DomainPurpose; label: string }[] = [
  { value: "tracking", label: "Tracking links" },
  { value: "pwa", label: "PWA install" },
  { value: "fallback", label: "Fallback/safe" },
];

export const domainFormSchema = z.object({
  domain: z.string().regex(/^([a-z0-9-]+\.)+[a-z]{2,}$/i, "Enter a valid domain, e.g. track.example.com"),
  purpose: z.array(z.enum(["tracking", "pwa", "fallback"] as [DomainPurpose, ...DomainPurpose[]])).min(1, "Select at least one purpose"),
  registrar: z.enum(REGISTRARS as [Registrar, ...Registrar[]]),
  dnsProvider: z.enum(DNS_PROVIDERS as [DnsProvider, ...DnsProvider[]]),
  status: z.enum(STATUS_OPTIONS as [DomainStatus, ...DomainStatus[]]),
});

export type DomainFormValues = z.infer<typeof domainFormSchema>;

/** The domain create/edit form — its own page (features/domains/{new-
 * domain-view,domain-detail-view}.tsx) instead of a slide-out Sheet,
 * matching the Offers/Traffic-Sources/Campaigns create/edit pages'
 * layout. Same fields/validation/mutation shape the Sheet version had;
 * only the container changed. Domains' own UI stays plain English (no
 * domains.json exists — a known, separately-scoped gap, see CLAUDE.md),
 * unlike every other converted feature in this batch. */
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
  /** Locks the submit button and every field that supports a disabled
   * state when the caller lacks settings.write. MultiSelect (Purpose)
   * has no disabled prop to plumb this into — the submit button staying
   * disabled is what actually blocks the write either way. */
  disabled?: boolean;
}) {
  const form = useForm<DomainFormValues>({
    resolver: zodResolver(domainFormSchema),
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
          <CardDescription>
            Own-domain parking (§30). SSL issuance and ownership verification are separate actions once the
            domain exists.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="dom-domain">Domain</Label>
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
            <Label>Purpose</Label>
            <Controller
              control={control}
              name="purpose"
              render={({ field }) => (
                <MultiSelect
                  label="Purpose"
                  options={PURPOSE_OPTIONS}
                  selected={field.value}
                  onChange={(values) => field.onChange(values as DomainPurpose[])}
                  className="w-full"
                />
              )}
            />
            {errors.purpose && <p className="text-xs text-danger">{errors.purpose.message}</p>}
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="dom-registrar">Registrar</Label>
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
                          {REGISTRAR_LABELS[r]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="dom-dns">DNS provider</Label>
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
                          {DNS_PROVIDER_LABELS[d]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          <div className="grid gap-1.5 sm:w-1/2">
            <Label htmlFor="dom-status">Status</Label>
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
                        {s}
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
          <Link href="/domains">Cancel</Link>
        </Button>
        <Button type="submit" disabled={disabled || isSubmitting}>
          {submitLabel}
        </Button>
      </div>
    </form>
  );
}

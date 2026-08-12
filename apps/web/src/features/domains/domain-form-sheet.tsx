"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MultiSelect } from "@/components/ui/multi-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  DNS_PROVIDER_LABELS,
  REGISTRAR_LABELS,
  type DnsProvider,
  type DomainPurpose,
  type DomainStatus,
  type Registrar,
} from "@/lib/mock/domains";

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

export function DomainFormSheet({
  open,
  onOpenChange,
  defaultValues,
  title,
  submitLabel,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<DomainFormValues>;
  title: string;
  submitLabel: string;
  onSubmit: (values: DomainFormValues) => void;
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
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-lg" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>
            Own-domain parking (§30). SSL issuance and ownership verification are separate actions once the
            domain exists.
          </SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-1.5">
            <Label htmlFor="dom-domain">Domain</Label>
            <Input
              id="dom-domain"
              placeholder="track.example.com"
              className="font-mono text-sm"
              {...register("domain")}
              aria-invalid={!!errors.domain}
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
                  <Select value={field.value} onValueChange={field.onChange}>
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
                  <Select value={field.value} onValueChange={field.onChange}>
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

          <div className="grid gap-1.5">
            <Label htmlFor="dom-status">Status</Label>
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
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

          <SheetFooter className="mt-0 flex-row justify-end gap-2 p-0">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
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

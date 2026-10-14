"use client";

import * as React from "react";
import { Controller, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { BellIcon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MultiSelect } from "@/components/ui/multi-select";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Separator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useCreatePushBroadcast, usePushAudience } from "@/hooks/use-push";
import { useOrganization } from "@/hooks/use-organization";
import { usePwas } from "@/hooks/use-pwas";
import { ApiError } from "@/lib/api/client";
import type { PushFilters } from "@/lib/api/push";
import { countryOptions } from "@/lib/countries";
import { formatInZone, utcOffsetLabel, utcToZonedLocal, zonedLocalToUtc } from "@/lib/timezones";

// Mirrors push.MinScheduleAhead / MaxScheduleAhead (internal/push/model.go).
const MIN_AHEAD_MS = 60_000;
const MAX_AHEAD_MS = 30 * 24 * 60 * 60_000;

const httpsURL = (msg: string) => z.union([z.literal(""), z.url({ protocol: /^https$/, error: msg })]);

/** Mirrors push.Service's validation (internal/push/service.go). A
 * scheduled time is a wall-clock time in the organization's timezone. */
function buildSchema(t: TFunction, timeZone: string) {
  return z
    .object({
      name: z.string().trim().min(2, t("form.validation.name")).max(100, t("form.validation.name")),
      title: z.string().trim().min(1, t("form.validation.title")).max(80, t("form.validation.title")),
      body: z.string().trim().max(240, t("form.validation.body")),
      iconUrl: httpsURL(t("form.validation.https")),
      imageUrl: httpsURL(t("form.validation.https")),
      target: z.enum(["destination", "url"]),
      url: z.string().trim(),
      pwaIds: z.array(z.string()),
      campaignIds: z.array(z.string()),
      countries: z.array(z.string()),
      when: z.enum(["now", "later"]),
      sendAtLocal: z.string(),
    })
    .superRefine((v, ctx) => {
      if (v.target === "url" && !/^https?:\/\/[^\s/]+/i.test(v.url)) {
        ctx.addIssue({ code: "custom", message: t("form.validation.url"), path: ["url"] });
      }
      if (v.when === "later") {
        const at = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(v.sendAtLocal) ? zonedLocalToUtc(v.sendAtLocal, timeZone).getTime() : NaN;
        const ahead = at - Date.now();
        if (!(ahead >= MIN_AHEAD_MS && ahead <= MAX_AHEAD_MS)) {
          ctx.addIssue({ code: "custom", message: t("form.validation.sendAt"), path: ["sendAtLocal"] });
        }
      }
    });
}

type FormValues = z.infer<ReturnType<typeof buildSchema>>;

const EMPTY: FormValues = {
  name: "", title: "", body: "", iconUrl: "", imageUrl: "",
  target: "destination", url: "", pwaIds: [], campaignIds: [], countries: [],
  when: "now", sendAtLocal: "",
};

export function BroadcastFormSheet({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t, i18n } = useTranslation("push");
  const timeZone = useOrganization().data?.timezone || "UTC";
  const form = useForm<FormValues>({ resolver: zodResolver(buildSchema(t, timeZone)), defaultValues: EMPTY });
  const { register, handleSubmit, control, formState, reset, setError } = form;
  const values = useWatch({ control });
  const create = useCreatePushBroadcast();
  const pwas = usePwas().data?.pwas ?? [];
  const campaigns = useCampaigns().data?.campaigns ?? [];
  const [confirming, setConfirming] = React.useState<FormValues | null>(null);

  const filters: PushFilters = React.useMemo(
    () => ({ pwaIds: values.pwaIds ?? [], campaignIds: values.campaignIds ?? [], countries: values.countries ?? [] }),
    [values.pwaIds, values.campaignIds, values.countries],
  );
  const audience = usePushAudience(filters, open);
  const count = audience.data?.count;
  const countries = React.useMemo(() => countryOptions(i18n.language), [i18n.language]);
  const previewIcon = values.iconUrl || pwas.find((p) => (values.pwaIds ?? []).includes(p.id))?.iconUrl || "";

  React.useEffect(() => {
    if (!open) reset(EMPTY);
  }, [open, reset]);

  function send(v: FormValues) {
    create.mutate(
      {
        name: v.name, title: v.title, body: v.body, iconUrl: v.iconUrl, imageUrl: v.imageUrl,
        target: v.target, url: v.target === "url" ? v.url : "",
        filters: { pwaIds: v.pwaIds, campaignIds: v.campaignIds, countries: v.countries },
        sendAt: v.when === "later" ? zonedLocalToUtc(v.sendAtLocal, timeZone).toISOString() : undefined,
      },
      {
        onSuccess: (b) => {
          setConfirming(null);
          onOpenChange(false);
          toast(b.status === "scheduled" ? t("form.scheduled") : t("form.sent"), { description: b.name });
        },
        onError: (err) => {
          setConfirming(null);
          if (err instanceof ApiError && err.fields) {
            for (const [field, message] of Object.entries(err.fields)) {
              if (field in EMPTY) setError(field as keyof FormValues, { message });
              if (field === "sendAt") setError("sendAtLocal", { message });
            }
            if (err.fields.filters) toast.error(t("form.validation.noAudience"));
            return;
          }
          toast.error(t("form.sendError"), { description: err.message });
        },
      },
    );
  }

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent className="gap-0 overflow-y-auto data-[side=right]:w-full data-[side=right]:sm:max-w-2xl" side="right">
          <SheetHeader>
            <SheetTitle>{t("form.title")}</SheetTitle>
            <SheetDescription>{t("form.description")}</SheetDescription>
          </SheetHeader>

          <form id="push-broadcast" onSubmit={handleSubmit((v) => setConfirming(v))} className="flex flex-col gap-4 px-4 pb-4">
            <Field id="pb-name" label={t("form.name")} error={formState.errors.name?.message}>
              <Input id="pb-name" placeholder={t("form.namePlaceholder")} {...register("name")} aria-invalid={!!formState.errors.name} />
            </Field>
            <Field id="pb-title" label={t("form.notificationTitle")} error={formState.errors.title?.message}>
              <Input id="pb-title" placeholder={t("form.notificationTitlePlaceholder")} maxLength={80} {...register("title")} aria-invalid={!!formState.errors.title} />
            </Field>
            <Field id="pb-body" label={t("form.body")} error={formState.errors.body?.message}>
              <Textarea id="pb-body" rows={3} placeholder={t("form.bodyPlaceholder")} maxLength={240} {...register("body")} aria-invalid={!!formState.errors.body} />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="pb-icon" label={t("form.iconUrl")} hint={t("form.iconHint")} error={formState.errors.iconUrl?.message}>
                <Input id="pb-icon" className="font-mono text-xs" placeholder="https://" {...register("iconUrl")} aria-invalid={!!formState.errors.iconUrl} />
              </Field>
              <Field id="pb-image" label={t("form.imageUrl")} hint={t("form.imageHint")} error={formState.errors.imageUrl?.message}>
                <Input id="pb-image" className="font-mono text-xs" placeholder="https://" {...register("imageUrl")} aria-invalid={!!formState.errors.imageUrl} />
              </Field>
            </div>

            <div className="grid gap-2">
              <Label>{t("form.target")}</Label>
              <Controller
                control={control}
                name="target"
                render={({ field }) => (
                  <RadioGroup value={field.value} onValueChange={field.onChange} className="grid gap-2">
                    <label className="flex items-start gap-2 text-sm">
                      <RadioGroupItem value="destination" className="mt-0.5" />
                      <span>
                        {t("form.targetDestination")}
                        <span className="block text-xs text-muted-foreground">{t("form.targetDestinationHint")}</span>
                      </span>
                    </label>
                    <label className="flex items-center gap-2 text-sm">
                      <RadioGroupItem value="url" />
                      {t("form.targetUrl")}
                    </label>
                  </RadioGroup>
                )}
              />
              {values.target === "url" && (
                <Field id="pb-url" label={t("form.url")} hint={t("form.urlHint")} error={formState.errors.url?.message}>
                  <Input id="pb-url" className="font-mono text-xs" placeholder="https://example.com/?cid={click_id}" {...register("url")} aria-invalid={!!formState.errors.url} />
                </Field>
              )}
            </div>

            <Separator />

            <div className="grid gap-3">
              <div className="flex items-baseline justify-between gap-2">
                <Label>{t("form.audience")}</Label>
                <span className="text-sm font-medium tabular-nums" aria-live="polite">
                  {count === undefined ? t("form.audienceCounting") : t("form.audienceCount", { count })}
                </span>
              </div>
              <div className="grid gap-3 sm:grid-cols-3">
                <Controller
                  control={control}
                  name="pwaIds"
                  render={({ field }) => (
                    <MultiSelect label={t("form.pwas")} options={pwas.map((p) => ({ value: p.id, label: p.name }))} selected={field.value} onChange={field.onChange} />
                  )}
                />
                <Controller
                  control={control}
                  name="campaignIds"
                  render={({ field }) => (
                    <MultiSelect label={t("form.campaigns")} options={campaigns.map((c) => ({ value: c.id, label: c.name }))} selected={field.value} onChange={field.onChange} />
                  )}
                />
                <Controller
                  control={control}
                  name="countries"
                  render={({ field }) => <MultiSelect label={t("form.countries")} options={countries} selected={field.value} onChange={field.onChange} />}
                />
              </div>
              <p className="text-xs text-muted-foreground">{t("form.audienceAll")}</p>
            </div>

            <Separator />

            <div className="grid gap-2">
              <Label>{t("form.when")}</Label>
              <Controller
                control={control}
                name="when"
                render={({ field }) => (
                  <RadioGroup
                    value={field.value}
                    onValueChange={(v) => {
                      field.onChange(v);
                      if (v === "later" && !form.getValues("sendAtLocal")) {
                        form.setValue("sendAtLocal", utcToZonedLocal(new Date(Date.now() + 60 * 60_000), timeZone));
                      }
                    }}
                    className="flex flex-wrap gap-4"
                  >
                    <label className="flex items-center gap-2 text-sm">
                      <RadioGroupItem value="now" />
                      {t("form.whenNow")}
                    </label>
                    <label className="flex items-center gap-2 text-sm">
                      <RadioGroupItem value="later" />
                      {t("form.whenLater")}
                    </label>
                  </RadioGroup>
                )}
              />
              {values.when === "later" && (
                <Field
                  id="pb-send-at"
                  label={t("form.sendAt", { zone: [timeZone, utcOffsetLabel(timeZone)].filter(Boolean).join(" ") })}
                  hint={t("form.sendAtHint")}
                  error={formState.errors.sendAtLocal?.message}
                >
                  <Input id="pb-send-at" type="datetime-local" className="w-56 font-mono text-xs" {...register("sendAtLocal")} aria-invalid={!!formState.errors.sendAtLocal} />
                </Field>
              )}
            </div>

            <Separator />

            <div className="grid gap-2">
              <Label>{t("form.preview")}</Label>
              <div className="flex max-w-sm items-start gap-3 rounded-lg border bg-card p-3 shadow-sm">
                {previewIcon ? (
                  // eslint-disable-next-line @next/next/no-img-element -- an operator-supplied external URL, shown as is
                  <img src={previewIcon} alt="" className="size-10 shrink-0 rounded-md object-cover" />
                ) : (
                  <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted">
                    <BellIcon className="size-5 text-muted-foreground" />
                  </div>
                )}
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{values.title || t("form.notificationTitlePlaceholder")}</p>
                  <p className="line-clamp-2 text-xs text-muted-foreground">{values.body || t("form.bodyPlaceholder")}</p>
                </div>
              </div>
            </div>
          </form>

          <SheetFooter className="flex-row justify-end">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t("form.cancel")}
            </Button>
            <Button type="submit" form="push-broadcast" disabled={(values.when !== "later" && count === 0) || formState.isSubmitting}>
              {values.when === "later" ? t("form.submitSchedule") : t("form.submit")}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      <Dialog open={confirming !== null} onOpenChange={(o) => !o && !create.isPending && setConfirming(null)}>
        <DialogContent>
          <DialogHeader>
            {confirming?.when === "later" ? (
              <>
                <DialogTitle>{t("form.confirmScheduleTitle", { name: confirming.name })}</DialogTitle>
                <DialogDescription>
                  {t("form.confirmScheduleDescription", {
                    when: formatInZone(zonedLocalToUtc(confirming.sendAtLocal, timeZone), timeZone, i18n.language),
                    count: count ?? 0,
                  })}
                </DialogDescription>
              </>
            ) : (
              <>
                <DialogTitle>{t("form.confirmTitle", { name: confirming?.name ?? "" })}</DialogTitle>
                <DialogDescription>{t("form.confirmDescription", { count: count ?? 0 })}</DialogDescription>
              </>
            )}
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirming(null)} disabled={create.isPending}>
              {t("form.cancel")}
            </Button>
            <Button onClick={() => confirming && send(confirming)} disabled={create.isPending}>
              {confirming?.when === "later" ? t("form.confirmSchedule") : t("form.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function Field({ id, label, hint, error, children }: { id: string; label: string; hint?: string; error?: string; children: React.ReactNode }) {
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {error ? <p className="text-xs text-danger">{error}</p> : hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

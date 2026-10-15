"use client";

import Link from "next/link";
import { Controller, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { TriangleAlertIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { MultiSelect } from "@/components/ui/multi-select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { MacroPicker } from "@/components/shared/macro-picker";
import { usePixelEventNameDefaults } from "@/hooks/use-pixels";
import {
  PIXEL_EVENT_TYPES,
  PIXEL_PROVIDERS,
  PIXEL_PROVIDER_I18N_KEY,
  SENDING_PIXEL_PROVIDERS,
  providerHasTestCode,
  providerUsesToken,
  type CreatePixelInput,
  type Pixel,
  type PixelEventType,
  type PixelProvider,
  type PixelStatus,
  type UpdatePixelInput,
} from "@/lib/api/pixels";

/** Factory, not a module-level const — Zod's validation messages are
 * user-facing text and need the live translator, which only exists inside
 * a component. The API validates the same rules (pixel.Service); this is
 * the inline feedback. */
export function buildPixelFormSchema(t: TFunction) {
  return z
    .object({
      name: z.string().trim().min(2, t("form.validation.nameMin", { ns: "pixels" })).max(100),
      provider: z.enum(PIXEL_PROVIDERS as [PixelProvider, ...PixelProvider[]]),
      pixelId: z.string().trim().max(80, t("form.validation.pixelIdMax", { ns: "pixels" })),
      events: z.array(z.enum(PIXEL_EVENT_TYPES)).min(1, t("form.validation.eventsRequired", { ns: "pixels" })),
      accessToken: z.string().trim().max(1000),
      clearAccessToken: z.boolean(),
      testEventCode: z.string().trim().max(100),
      url: z.string().trim().max(2000),
      eventNames: z.record(z.string(), z.string().trim().max(100)),
      status: z.enum(["active", "paused", "archived"] as [PixelStatus, ...PixelStatus[]]),
    })
    .superRefine((v, ctx) => {
      if (v.provider === "generic" && !/^https?:\/\/\S+$/i.test(v.url)) {
        ctx.addIssue({ code: "custom", path: ["url"], message: t("form.validation.urlRequired", { ns: "pixels" }) });
      }
    });
}

export type PixelFormValues = z.infer<ReturnType<typeof buildPixelFormSchema>>;

/** What the form sends: only the fields the chosen provider uses, event
 * names for the selected events, and the access token only when one was
 * typed ("" when the operator chose to remove it). */
export function toPixelInput(values: PixelFormValues): CreatePixelInput & UpdatePixelInput {
  const usesToken = providerUsesToken(values.provider);
  const eventNames: Partial<Record<PixelEventType, string>> = {};
  for (const e of values.events) {
    const name = values.eventNames[e]?.trim();
    if (name) eventNames[e] = name;
  }
  const input: CreatePixelInput & UpdatePixelInput = {
    name: values.name,
    provider: values.provider,
    pixelId: usesToken ? values.pixelId : "",
    events: values.events,
    testEventCode: providerHasTestCode(values.provider) ? values.testEventCode : "",
    url: values.provider === "generic" ? values.url : "",
    eventNames,
    status: values.status,
  };
  if (usesToken && values.clearAccessToken) input.accessToken = "";
  else if (usesToken && values.accessToken) input.accessToken = values.accessToken;
  return input;
}

const STATUS_OPTIONS: PixelStatus[] = ["active", "paused", "archived"];
// Event codes are canonical §43 event-model identifiers, not UI text —
// deliberately left untranslated, same treatment as the Event Mappings
// panel and Postlanding's form.
const EVENT_OPTIONS = PIXEL_EVENT_TYPES.map((e) => ({ value: e, label: e }));

/** The pixel create/edit form — its own page (features/pixels/{new-pixel-
 * view,pixel-detail-view}.tsx). Fields follow the provider: platform
 * pixels take a pixel id, an access token and (Facebook/TikTok) a test
 * event code; generic takes a URL template. `pixel` is the saved pixel
 * when editing — its token is never shown, only whether one is set. */
export function PixelForm({
  title,
  defaultValues,
  pixel,
  submitLabel,
  onSubmit,
  disabled = false,
}: {
  title: string;
  defaultValues: Partial<PixelFormValues>;
  pixel?: Pixel;
  submitLabel: string;
  onSubmit: (values: PixelFormValues) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation(["pixels", "common"]);
  const defaults = usePixelEventNameDefaults();
  const form = useForm<PixelFormValues>({
    resolver: zodResolver(buildPixelFormSchema(t)),
    defaultValues: {
      name: "",
      provider: "facebook",
      pixelId: "",
      events: [],
      accessToken: "",
      clearAccessToken: false,
      testEventCode: "",
      url: "",
      eventNames: {},
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

  const provider = useWatch({ control, name: "provider" });
  const events = useWatch({ control, name: "events" });
  const clearToken = useWatch({ control, name: "clearAccessToken" });
  const usesToken = providerUsesToken(provider);
  // Twitter cannot be chosen, but an existing twitter pixel keeps showing
  // its provider until it is moved to one that sends.
  const providerOptions = pixel?.provider === "twitter" ? [...SENDING_PIXEL_PROVIDERS, "twitter" as const] : SENDING_PIXEL_PROVIDERS;
  const tokenSaved = !!pixel?.hasAccessToken && pixel.provider === provider;

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
      <fieldset disabled={disabled} className="contents">
        <Card>
          <CardHeader>
            <CardTitle>{title}</CardTitle>
            <CardDescription>{t("form.description", { ns: "pixels" })}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {provider === "twitter" && (
              <Alert>
                <TriangleAlertIcon />
                <AlertTitle>{t("form.twitterTitle", { ns: "pixels" })}</AlertTitle>
                <AlertDescription>{t("form.twitterDescription", { ns: "pixels" })}</AlertDescription>
              </Alert>
            )}

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
                        {providerOptions.map((p) => (
                          <SelectItem key={p} value={p} disabled={p === "twitter"}>
                            {t(PIXEL_PROVIDER_I18N_KEY[p], { ns: "pixels" })}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>
            </div>

            {usesToken ? (
              <>
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="grid gap-1.5">
                    <Label htmlFor="pxl-id">{t(`form.pixelIdLabel_${provider}`, { ns: "pixels", defaultValue: t("form.pixelIdLabel", { ns: "pixels" }) })}</Label>
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
                    <Label htmlFor="pxl-token">{t("form.accessTokenLabel", { ns: "pixels" })}</Label>
                    <Input
                      id="pxl-token"
                      type="password"
                      autoComplete="off"
                      placeholder={
                        tokenSaved
                          ? t("form.accessTokenSaved", { ns: "pixels", last4: pixel?.accessTokenLast4 })
                          : t("form.accessTokenPlaceholder", { ns: "pixels" })
                      }
                      className="font-mono text-xs"
                      disabled={clearToken}
                      {...register("accessToken")}
                    />
                    {tokenSaved ? (
                      <label className="flex items-center gap-2 text-xs text-muted-foreground">
                        <Controller
                          control={control}
                          name="clearAccessToken"
                          render={({ field }) => (
                            <Checkbox checked={field.value} onCheckedChange={(v) => field.onChange(v === true)} />
                          )}
                        />
                        {t("form.accessTokenRemove", { ns: "pixels" })}
                      </label>
                    ) : (
                      <p className="text-xs text-muted-foreground">{t("form.accessTokenHint", { ns: "pixels" })}</p>
                    )}
                  </div>
                </div>

                {providerHasTestCode(provider) && (
                  <div className="grid gap-1.5 sm:w-1/2">
                    <Label htmlFor="pxl-test">{t("form.testEventCodeLabel", { ns: "pixels" })}</Label>
                    <Input id="pxl-test" placeholder="TEST12345" className="font-mono text-xs" {...register("testEventCode")} />
                    <p className="text-xs text-muted-foreground">{t("form.testEventCodeHint", { ns: "pixels" })}</p>
                  </div>
                )}
              </>
            ) : (
              <div className="grid gap-1.5">
                <div className="flex items-center justify-between">
                  <Label htmlFor="pxl-url">{t("form.urlLabel", { ns: "pixels" })}</Label>
                  <MacroPicker onInsert={(token) => setValue("url", `${getValues("url")}${token}`, { shouldValidate: true })} />
                </div>
                <Input
                  id="pxl-url"
                  placeholder="https://tracker.example.com/px?cid={click_id}&event={status}"
                  className="font-mono text-xs"
                  {...register("url")}
                  aria-invalid={!!errors.url}
                />
                {errors.url ? (
                  <p className="text-xs text-danger">{errors.url.message}</p>
                ) : (
                  <p className="text-xs text-muted-foreground">{t("form.urlHint", { ns: "pixels" })}</p>
                )}
              </div>
            )}

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

            {events.length > 0 && (
              <div className="grid gap-2">
                <div>
                  <Label>{t("form.eventNamesLabel", { ns: "pixels" })}</Label>
                  <p className="text-xs text-muted-foreground">{t("form.eventNamesHint", { ns: "pixels" })}</p>
                </div>
                <div className="grid gap-2 sm:grid-cols-2">
                  {events.map((e) => (
                    <div key={e} className="grid grid-cols-[11rem_1fr] items-center gap-2">
                      <span className="font-mono text-xs text-muted-foreground">{e}</span>
                      <Input
                        aria-label={t("form.eventNameAria", { ns: "pixels", event: e })}
                        placeholder={defaults.data?.defaults[provider]?.[e] ?? e}
                        className="h-8 text-xs"
                        {...register(`eventNames.${e}`)}
                      />
                    </div>
                  ))}
                </div>
              </div>
            )}

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

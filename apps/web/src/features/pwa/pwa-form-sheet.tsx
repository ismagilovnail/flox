"use client";

import { Controller, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Mono } from "@/components/ui/typography";
import type { PwaStatus } from "@/lib/api/pwa";

/** Factory, not a module-level const — see source-form-sheet.tsx's
 * buildSourceFormSchema for why (Zod messages are user-facing text and
 * need the live translator). */
export function buildPwaFormSchema(t: TFunction) {
  const hexColor = z.string().regex(/^#[0-9a-fA-F]{6}$/, t("form.validation.colorInvalid", { ns: "pwa" }));
  return z.object({
    name: z.string().min(2, t("form.validation.nameMin", { ns: "pwa" })).max(80),
    shortName: z.string().min(1, t("form.validation.shortNameRequired", { ns: "pwa" })).max(20),
    themeColor: hexColor,
    backgroundColor: hexColor,
    iconUrl: z.url(t("form.validation.iconUrlInvalid", { ns: "pwa" })),
    startUrl: z.string().min(1, t("form.validation.startUrlRequired", { ns: "pwa" })),
    bounceInAppWebview: z.boolean(),
    status: z.enum(["active", "paused", "archived"] as [PwaStatus, ...PwaStatus[]]),
  });
}

export type PwaFormValues = z.infer<ReturnType<typeof buildPwaFormSchema>>;

const STATUS_OPTIONS: PwaStatus[] = ["active", "paused", "archived"];

export function PwaFormSheet({
  open,
  onOpenChange,
  defaultValues,
  title,
  submitLabel,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<PwaFormValues>;
  title: string;
  submitLabel: string;
  onSubmit: (values: PwaFormValues) => void;
}) {
  const { t } = useTranslation(["pwa", "common"]);
  const form = useForm<PwaFormValues>({
    resolver: zodResolver(buildPwaFormSchema(t)),
    defaultValues: {
      name: "",
      shortName: "",
      themeColor: "#16a34a",
      backgroundColor: "#0a0a0a",
      iconUrl: "",
      startUrl: "/install",
      bounceInAppWebview: true,
      status: "active",
      ...defaultValues,
    },
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  const manifest = useWatch({ control });

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-lg" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{t("form.description", { ns: "pwa" })}</SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="pwa-name">{t("form.nameLabel", { ns: "pwa" })}</Label>
              <Input
                id="pwa-name"
                placeholder={t("form.namePlaceholder", { ns: "pwa" })}
                {...register("name")}
                aria-invalid={!!errors.name}
              />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="pwa-short-name">{t("form.shortNameLabel", { ns: "pwa" })}</Label>
              <Input
                id="pwa-short-name"
                placeholder={t("form.shortNamePlaceholder", { ns: "pwa" })}
                {...register("shortName")}
                aria-invalid={!!errors.shortName}
              />
              {errors.shortName && <p className="text-xs text-danger">{errors.shortName.message}</p>}
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="pwa-theme">{t("form.themeColorLabel", { ns: "pwa" })}</Label>
              <div className="flex items-center gap-2">
                <input
                  type="color"
                  value={/^#[0-9a-fA-F]{6}$/.test(manifest.themeColor ?? "") ? manifest.themeColor : "#16a34a"}
                  onChange={(e) => form.setValue("themeColor", e.target.value)}
                  className="size-8 shrink-0 rounded-md border border-input bg-transparent"
                  aria-label={t("form.themeColorPickerAria", { ns: "pwa" })}
                />
                <Input id="pwa-theme" {...register("themeColor")} className="font-mono text-xs" aria-invalid={!!errors.themeColor} />
              </div>
              {errors.themeColor && <p className="text-xs text-danger">{errors.themeColor.message}</p>}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="pwa-bg">{t("form.backgroundColorLabel", { ns: "pwa" })}</Label>
              <div className="flex items-center gap-2">
                <input
                  type="color"
                  value={/^#[0-9a-fA-F]{6}$/.test(manifest.backgroundColor ?? "") ? manifest.backgroundColor : "#0a0a0a"}
                  onChange={(e) => form.setValue("backgroundColor", e.target.value)}
                  className="size-8 shrink-0 rounded-md border border-input bg-transparent"
                  aria-label={t("form.backgroundColorPickerAria", { ns: "pwa" })}
                />
                <Input id="pwa-bg" {...register("backgroundColor")} className="font-mono text-xs" aria-invalid={!!errors.backgroundColor} />
              </div>
              {errors.backgroundColor && <p className="text-xs text-danger">{errors.backgroundColor.message}</p>}
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="pwa-icon">{t("form.iconUrlLabel", { ns: "pwa" })}</Label>
            <Input
              id="pwa-icon"
              placeholder={t("form.iconUrlPlaceholder", { ns: "pwa" })}
              className="font-mono text-xs"
              {...register("iconUrl")}
              aria-invalid={!!errors.iconUrl}
            />
            {errors.iconUrl && <p className="text-xs text-danger">{errors.iconUrl.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="pwa-start">{t("form.startUrlLabel", { ns: "pwa" })}</Label>
            <Input
              id="pwa-start"
              placeholder={t("form.startUrlPlaceholder", { ns: "pwa" })}
              className="font-mono text-xs"
              {...register("startUrl")}
              aria-invalid={!!errors.startUrl}
            />
            {errors.startUrl && <p className="text-xs text-danger">{errors.startUrl.message}</p>}
          </div>

          <div className="flex items-center justify-between rounded-md border border-border p-2.5">
            <div>
              <p className="text-sm font-medium">{t("form.bounceLabel", { ns: "pwa" })}</p>
              <p className="text-xs text-muted-foreground">{t("form.bounceHint", { ns: "pwa" })}</p>
            </div>
            <Controller
              control={control}
              name="bounceInAppWebview"
              render={({ field }) => (
                <Switch checked={field.value} onCheckedChange={field.onChange} aria-label={t("form.bounceAria", { ns: "pwa" })} />
              )}
            />
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="pwa-status">{t("form.statusLabel", { ns: "pwa" })}</Label>
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="pwa-status" className="w-full">
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

          <Separator />

          <div className="grid gap-1.5">
            <Label>{t("form.manifestPreviewLabel", { ns: "pwa" })}</Label>
            <pre className="overflow-x-auto rounded-md bg-muted p-2.5 text-xs">
              <Mono>
                {JSON.stringify(
                  {
                    name: manifest.name || "",
                    short_name: manifest.shortName || "",
                    theme_color: manifest.themeColor || "",
                    background_color: manifest.backgroundColor || "",
                    start_url: manifest.startUrl || "",
                    display: "standalone",
                    icons: [{ src: manifest.iconUrl || "", sizes: "512x512", type: "image/png" }],
                  },
                  null,
                  2,
                )}
              </Mono>
            </pre>
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

"use client";

import { Controller, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

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
import type { PwaStatus } from "@/lib/mock/pwas";

const hexColor = z.string().regex(/^#[0-9a-fA-F]{6}$/, "Enter a hex color like #16a34a");

export const pwaFormSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(80),
  shortName: z.string().min(1, "Required for the home-screen icon label").max(20),
  themeColor: hexColor,
  backgroundColor: hexColor,
  iconUrl: z.url("Enter a valid icon URL"),
  startUrl: z.string().min(1, "Required"),
  bounceInAppWebview: z.boolean(),
  status: z.enum(["active", "paused", "archived"] as [PwaStatus, ...PwaStatus[]]),
});

export type PwaFormValues = z.infer<typeof pwaFormSchema>;

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
  const form = useForm<PwaFormValues>({
    resolver: zodResolver(pwaFormSchema),
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
          <SheetDescription>
            These fields are the real Web App Manifest (§28) — the preview below is what installs on the device.
          </SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="pwa-name">Name</Label>
              <Input id="pwa-name" placeholder="Sweeps PWA" {...register("name")} aria-invalid={!!errors.name} />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="pwa-short-name">Short name</Label>
              <Input
                id="pwa-short-name"
                placeholder="Sweeps"
                {...register("shortName")}
                aria-invalid={!!errors.shortName}
              />
              {errors.shortName && <p className="text-xs text-danger">{errors.shortName.message}</p>}
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="pwa-theme">Theme color</Label>
              <div className="flex items-center gap-2">
                <input
                  type="color"
                  value={/^#[0-9a-fA-F]{6}$/.test(manifest.themeColor ?? "") ? manifest.themeColor : "#16a34a"}
                  onChange={(e) => form.setValue("themeColor", e.target.value)}
                  className="size-8 shrink-0 rounded-md border border-input bg-transparent"
                  aria-label="Pick theme color"
                />
                <Input id="pwa-theme" {...register("themeColor")} className="font-mono text-xs" aria-invalid={!!errors.themeColor} />
              </div>
              {errors.themeColor && <p className="text-xs text-danger">{errors.themeColor.message}</p>}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="pwa-bg">Background color</Label>
              <div className="flex items-center gap-2">
                <input
                  type="color"
                  value={/^#[0-9a-fA-F]{6}$/.test(manifest.backgroundColor ?? "") ? manifest.backgroundColor : "#0a0a0a"}
                  onChange={(e) => form.setValue("backgroundColor", e.target.value)}
                  className="size-8 shrink-0 rounded-md border border-input bg-transparent"
                  aria-label="Pick background color"
                />
                <Input id="pwa-bg" {...register("backgroundColor")} className="font-mono text-xs" aria-invalid={!!errors.backgroundColor} />
              </div>
              {errors.backgroundColor && <p className="text-xs text-danger">{errors.backgroundColor.message}</p>}
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="pwa-icon">Icon URL</Label>
            <Input
              id="pwa-icon"
              placeholder="https://cdn.floxlink.io/pwa/sweeps/icon-512.png"
              className="font-mono text-xs"
              {...register("iconUrl")}
              aria-invalid={!!errors.iconUrl}
            />
            {errors.iconUrl && <p className="text-xs text-danger">{errors.iconUrl.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="pwa-start">Start URL</Label>
            <Input
              id="pwa-start"
              placeholder="/install/sweeps"
              className="font-mono text-xs"
              {...register("startUrl")}
              aria-invalid={!!errors.startUrl}
            />
            {errors.startUrl && <p className="text-xs text-danger">{errors.startUrl.message}</p>}
          </div>

          <div className="flex items-center justify-between rounded-md border border-border p-2.5">
            <div>
              <p className="text-sm font-medium">Bounce in-app WebView traffic</p>
              <p className="text-xs text-muted-foreground">
                Sends FB/IG/TikTok/Telegram in-app browser traffic to the device&apos;s external browser (Android
                intent / iOS Safari scheme) so the install prompt can fire. Provider-neutral technical requirement,
                not moderator detection (§73).
              </p>
            </div>
            <Controller
              control={control}
              name="bounceInAppWebview"
              render={({ field }) => (
                <Switch checked={field.value} onCheckedChange={field.onChange} aria-label="Bounce in-app WebView traffic" />
              )}
            />
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="pwa-status">Status</Label>
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
                        {s}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </div>

          <Separator />

          <div className="grid gap-1.5">
            <Label>Manifest preview</Label>
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

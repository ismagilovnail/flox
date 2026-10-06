"use client";

import * as React from "react";
import { toast } from "sonner";
import { CopyIcon, SlidersHorizontalIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Mono } from "@/components/ui/typography";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  buildTrackingUrl,
  PLATFORM_PRESETS,
  PARAM_KEYS,
  type LinkParams,
  type LinkPlatform,
} from "@/lib/tracking-link-params";

const PLATFORMS: LinkPlatform[] = ["facebook", "tiktok"];

function copy(text: string, message: string, errorMessage: string) {
  navigator.clipboard.writeText(text).then(
    () => toast(message),
    () => toast.error(errorMessage, { description: text }),
  );
}

/** "Copy with parameters" menu + the parameter constructor for one tracking
 * link. The source type's platform is listed first so the common case is one click. */
export function TrackingLinkBuilder({
  url,
  defaultPlatform,
  disabled = false,
}: {
  url: string;
  defaultPlatform: LinkPlatform | null;
  disabled?: boolean;
}) {
  const { t } = useTranslation("campaigns");
  const [open, setOpen] = React.useState(false);
  const [params, setParams] = React.useState<LinkParams>({});

  const platforms = defaultPlatform ? [defaultPlatform, ...PLATFORMS.filter((p) => p !== defaultPlatform)] : PLATFORMS;
  const platformName = (p: LinkPlatform) => t(`trackingLinks.builder.platform.${p}`);
  const preview = buildTrackingUrl(url, params);

  function openBuilder() {
    setParams(defaultPlatform ? { ...PLATFORM_PRESETS[defaultPlatform] } : {});
    setOpen(true);
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm" disabled={disabled} aria-label={t("trackingLinks.builder.copyMenuAria")}>
            <CopyIcon className="size-3.5" />
            {t("trackingLinks.builder.copyMenu")}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {platforms.map((p) => (
            <DropdownMenuItem
              key={p}
              onSelect={() => copy(buildTrackingUrl(url, PLATFORM_PRESETS[p]), t("trackingLinks.builder.copiedFor", { platform: platformName(p) }), t("trackingLinks.builder.copyError"))}
            >
              {t("trackingLinks.builder.copyFor", { platform: platformName(p) })}
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={openBuilder}>
            <SlidersHorizontalIcon className="size-3.5" />
            {t("trackingLinks.builder.configure")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("trackingLinks.builder.title")}</DialogTitle>
            <DialogDescription>{t("trackingLinks.builder.description", { macros: "{{campaign.id}}, __AID__…", interpolation: { escapeValue: false } })}</DialogDescription>
          </DialogHeader>

          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">{t("trackingLinks.builder.fillFrom")}</span>
            {PLATFORMS.map((p) => (
              <Button key={p} size="sm" variant="outline" onClick={() => setParams({ ...PLATFORM_PRESETS[p] })}>
                {platformName(p)}
              </Button>
            ))}
            <Button size="sm" variant="ghost" onClick={() => setParams({})}>
              {t("trackingLinks.builder.clear")}
            </Button>
          </div>

          <div className="grid max-h-[50vh] grid-cols-1 gap-x-3 gap-y-2 overflow-y-auto sm:grid-cols-2">
            {PARAM_KEYS.map((key) => (
              <div key={key} className="grid gap-1">
                <Label htmlFor={`tlb-${key}`} className="font-mono text-xs">
                  {key}
                </Label>
                <Input
                  id={`tlb-${key}`}
                  value={params[key] ?? ""}
                  onChange={(e) => setParams((prev) => ({ ...prev, [key]: e.target.value }))}
                  className="font-mono text-sm"
                  autoComplete="off"
                />
              </div>
            ))}
          </div>

          <div className="grid gap-1">
            <span className="text-xs text-muted-foreground">{t("trackingLinks.builder.preview")}</span>
            <Mono className="break-all rounded-md bg-muted px-3 py-2 text-xs">{preview}</Mono>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t("trackingLinks.builder.close")}
            </Button>
            <Button onClick={() => copy(preview, t("trackingLinks.copiedToast"), t("trackingLinks.builder.copyError"))}>
              <CopyIcon className="size-3.5" />
              {t("trackingLinks.builder.copyLink")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

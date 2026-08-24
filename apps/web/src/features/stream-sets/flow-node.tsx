"use client";

import * as React from "react";
import { CopyIcon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { IconButton } from "@/components/ui/icon-button";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

/** One stage in the §25 funnel (Landing/PWA/Postlanding/Offer/Redirect/
 * Fallback). Optional stages get an enable switch that reveals `children`
 * (their configuration); the terminal Offer/Redirect node is always on and
 * has no switch. `previewUrl` and `analytics` back §25's "preview" and
 * "analytics summary" node capabilities. */
export function FlowNode({
  icon: Icon,
  label,
  toggleable = true,
  enabled,
  onToggleEnabled,
  configured,
  previewUrl,
  analytics,
  ghost = false,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  toggleable?: boolean;
  enabled: boolean;
  onToggleEnabled?: (enabled: boolean) => void;
  configured: boolean;
  previewUrl?: string;
  analytics?: string;
  ghost?: boolean;
  children?: React.ReactNode;
}) {
  const { t } = useTranslation("streamSets");
  const statusVariant = !enabled ? "secondary" : configured ? "success" : "warning";
  const statusLabel = !enabled
    ? t("flowNode.skipped")
    : configured
      ? t("flowNode.configured")
      : t("flowNode.needsSetup");

  function copyPreview() {
    if (!previewUrl) return;
    navigator.clipboard.writeText(previewUrl);
    toast(t("flowNode.urlCopiedToast", { label }), { description: previewUrl });
  }

  return (
    <div
      className={cn(
        "flex flex-col gap-2 rounded-lg border px-3 py-2",
        ghost ? "border-dashed border-border/70 bg-transparent" : "border-border bg-card",
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Icon className={cn("size-4", ghost ? "text-muted-foreground" : "text-foreground")} />
          <span className={cn("text-sm font-medium", ghost && "text-muted-foreground")}>{label}</span>
          {!ghost && <Badge variant={statusVariant}>{statusLabel}</Badge>}
        </div>
        <div className="flex items-center gap-1">
          {previewUrl && (
            <IconButton aria-label={t("flowNode.copyUrlAria", { label })} size="icon-sm" onClick={copyPreview}>
              <CopyIcon className="size-3.5" />
            </IconButton>
          )}
          {toggleable && onToggleEnabled && (
            <Switch
              size="sm"
              checked={enabled}
              onCheckedChange={onToggleEnabled}
              aria-label={t("flowNode.enableAria", { label })}
            />
          )}
        </div>
      </div>
      {enabled && children && <div className="flex flex-wrap items-center gap-2 pl-6">{children}</div>}
      {enabled && analytics && <p className="pl-6 text-xs text-muted-foreground">{analytics}</p>}
    </div>
  );
}

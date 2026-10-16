"use client";

import * as React from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { DownloadIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePermission } from "@/hooks/use-auth";
import { useOrganizationExports, useRequestOrganizationExport } from "@/hooks/use-organization";
import { organizationExportDownloadUrl, type OrganizationExport } from "@/lib/api/organization";
import { formatDeletionDate } from "@/lib/format";

const STATUS_VARIANT: Record<OrganizationExport["status"], "secondary" | "warning" | "success" | "danger"> = {
  pending: "warning",
  running: "warning",
  done: "success",
  failed: "danger",
  expired: "secondary",
};

function isoDay(d: Date): string {
  return d.toISOString().slice(0, 10);
}

function formatSize(bytes: number): string {
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

/** The organization's data export (audit 2026-10-15, GDPR-1): every record,
 * the team and the events of a period, as a ZIP built in the background
 * and kept for 7 days. Owners and admins (settings.write). */
export function OrganizationExportCard() {
  const { t, i18n } = useTranslation("settings");
  const canManage = usePermission("settings.write");
  const exportsQuery = useOrganizationExports(canManage);
  const request = useRequestOrganizationExport();
  const today = new Date();
  const [from, setFrom] = React.useState(isoDay(new Date(today.getTime() - 89 * 86_400_000)));
  const [to, setTo] = React.useState(isoDay(today));

  if (!canManage) return null;
  const exports = exportsQuery.data?.exports ?? [];
  const busy = exports.some((e) => e.status === "pending" || e.status === "running");

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("export.title")}</CardTitle>
        <CardDescription>{t("export.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            request.mutate(
              { from, to },
              {
                onSuccess: () => toast(t("export.requestedToast")),
                onError: (err) => toast.error(t("export.requestError"), { description: err.message }),
              },
            );
          }}
        >
          <div className="grid gap-1.5">
            <Label htmlFor="export-from">{t("export.from")}</Label>
            <Input id="export-from" type="date" value={from} max={to} onChange={(e) => setFrom(e.target.value)} className="w-40" />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="export-to">{t("export.to")}</Label>
            <Input id="export-to" type="date" value={to} min={from} max={isoDay(today)} onChange={(e) => setTo(e.target.value)} className="w-40" />
          </div>
          <Button type="submit" disabled={busy || request.isPending || !from || !to}>
            {t("export.request")}
          </Button>
        </form>
        <p className="text-xs text-muted-foreground">{t("export.hint")}</p>

        {exports.length > 0 && (
          <ul className="flex flex-col gap-2">
            {exports.map((e) => (
              <li key={e.id} className="flex flex-wrap items-center gap-2 text-sm">
                <Badge variant={STATUS_VARIANT[e.status]}>{t(`export.status.${e.status}`)}</Badge>
                <span className="tabular-nums">
                  {e.from} — {e.to}
                </span>
                {e.status === "done" && (
                  <>
                    <span className="text-xs text-muted-foreground tabular-nums">{formatSize(e.sizeBytes)}</span>
                    {e.expiresAt && (
                      <span className="text-xs text-muted-foreground">
                        {t("export.until", { date: formatDeletionDate(e.expiresAt, i18n.language) })}
                      </span>
                    )}
                    <Button asChild size="sm" variant="outline" className="h-7">
                      <a href={organizationExportDownloadUrl(e.id)}>
                        <DownloadIcon className="size-3.5" /> {t("export.download")}
                      </a>
                    </Button>
                  </>
                )}
                {e.status === "failed" && <span className="text-xs text-danger">{e.error}</span>}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

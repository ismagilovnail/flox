"use client";

import * as React from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { DownloadIcon, SearchIcon, Trash2Icon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ErrorState } from "@/components/ui/error-state";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/ui/loading-state";
import { Mono } from "@/components/ui/typography";
import { useEraseVisitor, useVisitorReport } from "@/hooks/use-visitor-data";
import type { VisitorReport } from "@/lib/api/visitor-data";

const IPV4 = /^\d{1,3}(\.\d{1,3}){3}$/;

/** An IP address goes to `ip`, anything else is a click id. */
export function visitorQuery(raw: string): { clickId?: string; ip?: string } | null {
  const v = raw.trim();
  if (!v) return null;
  return IPV4.test(v) || v.includes(":") ? { ip: v } : { clickId: v };
}

/** Nothing held: a click id search echoes the id back with empty lists. */
function isEmpty(r: VisitorReport): boolean {
  return (
    r.clicks.length + r.events.length + r.conversions.length + r.pushSubscriptions.length + r.telegramUsers.length + r.pixelDeliveries ===
    0
  );
}

function download(report: VisitorReport, label: string) {
  const blob = new Blob([JSON.stringify(report, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `visitor-${label.replace(/[^\w.-]+/g, "_")}.json`;
  a.click();
  URL.revokeObjectURL(url);
}

/** A visitor's data request (audit 2026-10-15, GDPR-1): find what FLOX
 * holds about one visitor by click id or IP, give it to them as JSON, and
 * erase it. Conversions stay — they are the organization's records. */
export function VisitorDataPanel() {
  const { t } = useTranslation("settings");
  const [input, setInput] = React.useState("");
  const [query, setQuery] = React.useState<{ clickId?: string; ip?: string } | null>(null);
  const [confirming, setConfirming] = React.useState(false);
  const report = useVisitorReport(query);
  const erase = useEraseVisitor();
  const data = report.data;
  const label = query?.ip ?? query?.clickId ?? "";

  function onErase() {
    if (!data) return;
    erase.mutate(data.clickIds, {
      onSuccess: () => {
        setConfirming(false);
        toast(t("visitorData.erasedToast"));
        void report.refetch();
      },
      onError: (err) => toast.error(t("visitorData.eraseError"), { description: err.message }),
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("visitorData.title")}</CardTitle>
        <CardDescription>{t("visitorData.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <form
          className="flex max-w-xl items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            setQuery(visitorQuery(input));
          }}
        >
          <div className="grid flex-1 gap-1.5">
            <Label htmlFor="visitor-query">{t("visitorData.query")}</Label>
            <Input id="visitor-query" value={input} onChange={(e) => setInput(e.target.value)} placeholder={t("visitorData.placeholder")} className="font-mono text-xs" />
          </div>
          <Button type="submit" disabled={!input.trim()}>
            <SearchIcon className="size-3.5" /> {t("visitorData.find")}
          </Button>
        </form>
        <p className="text-xs text-muted-foreground">{t("visitorData.ipHint")}</p>

        {report.isFetching && <LoadingState label={t("visitorData.loading")} />}
        {report.isError && <ErrorState title={t("visitorData.loadError")} description={report.error.message} onRetry={() => report.refetch()} />}
        {data && !report.isFetching && isEmpty(data) && <p className="text-sm text-muted-foreground">{t("visitorData.nothing")}</p>}
        {data && !report.isFetching && !isEmpty(data) && (
          <div className="flex flex-col gap-3">
            {data.truncated && <p className="text-xs text-warning">{t("visitorData.truncated", { count: data.clickIds.length })}</p>}
            <dl className="grid max-w-xl grid-cols-2 gap-x-4 gap-y-1 text-sm tabular-nums">
              {(
                [
                  ["clickIds", data.clickIds.length],
                  ["clicks", data.clicks.length],
                  ["events", data.events.length],
                  ["conversions", data.conversions.length],
                  ["push", data.pushSubscriptions.length],
                  ["telegram", data.telegramUsers.length],
                  ["pixels", data.pixelDeliveries],
                ] as const
              ).map(([key, n]) => (
                <React.Fragment key={key}>
                  <dt className="text-muted-foreground">{t(`visitorData.counts.${key}`)}</dt>
                  <dd>{n}</dd>
                </React.Fragment>
              ))}
            </dl>
            {data.clicks.slice(0, 5).map((c) => (
              <Mono key={c.clickId + c.at} className="truncate text-xs">
                {c.at.slice(0, 19).replace("T", " ")} · {c.clickId} · {c.ip || "—"} · {c.country || "—"} · {c.userAgent || "—"}
              </Mono>
            ))}
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => download(data, label)}>
                <DownloadIcon className="size-3.5" /> {t("visitorData.download")}
              </Button>
              <Button variant="destructive" onClick={() => setConfirming(true)}>
                <Trash2Icon className="size-3.5" /> {t("visitorData.erase")}
              </Button>
            </div>
          </div>
        )}
      </CardContent>

      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("visitorData.confirmTitle", { count: data?.clickIds.length ?? 0 })}</DialogTitle>
            <DialogDescription>{t("visitorData.confirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirming(false)}>
              {t("common:actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={onErase} disabled={erase.isPending}>
              {t("visitorData.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

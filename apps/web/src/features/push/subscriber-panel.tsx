"use client";

import * as React from "react";
import type { UseQueryResult } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { EmptyState } from "@/components/ui/empty-state";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { RelativeTime } from "@/components/ui/relative-time";
import { StatCard } from "@/components/ui/stat-card";
import { Mono } from "@/components/ui/typography";
import { useCampaigns } from "@/hooks/use-campaigns";
import { usePushSubscriptions } from "@/hooks/use-push";
import { usePwas } from "@/hooks/use-pwas";
import { formatInt } from "@/lib/format";
import type { PushGroupCount, PushSubscription, PushSummary } from "@/lib/api/push";

type Names = { pwa: Map<string, string>; campaign: Map<string, string> };

function subscriberColumns(t: TFunction, names: Names): ColumnDef<typeof dataTableFeatures, PushSubscription>[] {
  return [
    {
      accessorKey: "createdAt",
      header: t("subscribers.columns.subscribed"),
      cell: ({ getValue }) => <RelativeTime date={getValue() as string} className="text-xs text-muted-foreground" />,
    },
    {
      id: "pwa",
      accessorFn: (s) => names.pwa.get(s.pwaId) ?? s.pwaId,
      header: t("subscribers.columns.pwa"),
    },
    {
      id: "campaign",
      accessorFn: (s) => (s.campaignId ? (names.campaign.get(s.campaignId) ?? s.campaignId) : t("subscribers.none")),
      header: t("subscribers.columns.campaign"),
    },
    { accessorKey: "country", header: t("subscribers.columns.country"), cell: ({ getValue }) => (getValue() as string) || t("subscribers.none") },
    { accessorKey: "device", header: t("subscribers.columns.device"), cell: ({ getValue }) => (getValue() as string) || t("subscribers.none") },
    { accessorKey: "pushService", header: t("subscribers.columns.service"), cell: ({ getValue }) => <Mono className="text-xs">{getValue() as string}</Mono> },
    {
      accessorKey: "status",
      header: t("subscribers.columns.status"),
      cell: ({ getValue }) => {
        const status = getValue() as PushSubscription["status"];
        return <Badge variant={status === "active" ? "success" : "secondary"}>{t(`subscribers.status.${status}`)}</Badge>;
      },
    },
    { accessorKey: "clickId", header: t("subscribers.columns.click"), cell: ({ getValue }) => <Mono className="text-xs">{getValue() as string}</Mono> },
  ];
}

function GroupCard({ title, groups, label }: { title: string; groups: PushGroupCount[]; label: (key: string) => string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="flex flex-col gap-1.5 text-sm">
          {groups.slice(0, 8).map((g) => (
            <li key={g.key} className="flex items-center justify-between gap-2">
              <span className="truncate">{label(g.key)}</span>
              <Mono className="tabular-nums">{formatInt(g.count)}</Mono>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}

export function SubscriberPanel({ summary }: { summary: UseQueryResult<{ summary: PushSummary; configured: boolean }> }) {
  const { t } = useTranslation("push");
  const list = usePushSubscriptions();
  const pwas = usePwas().data?.pwas;
  const campaigns = useCampaigns().data?.campaigns;
  const names = React.useMemo<Names>(
    () => ({
      pwa: new Map((pwas ?? []).map((p) => [p.id, p.name])),
      campaign: new Map((campaigns ?? []).map((c) => [c.id, c.name])),
    }),
    [pwas, campaigns],
  );

  if (summary.isPending || list.isPending) return <LoadingState label={t("subscribers.loading")} />;
  if (summary.isError || list.isError) {
    const err = summary.error ?? list.error;
    return (
      <ErrorState
        title={t("subscribers.loadError")}
        description={err?.message}
        onRetry={() => {
          summary.refetch();
          list.refetch();
        }}
      />
    );
  }
  const s = summary.data.summary;
  if (s.active + s.unsubscribed === 0) {
    return <EmptyState title={t("subscribers.emptyTitle")} description={t("subscribers.emptyDescription")} />;
  }
  const none = t("subscribers.none");

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <StatCard label={t("subscribers.active")} value={formatInt(s.active)} />
        <StatCard label={t("subscribers.unsubscribed")} value={formatInt(s.unsubscribed)} />
      </div>
      {s.active > 0 && (
        <div className="grid gap-3 md:grid-cols-3">
          <GroupCard title={t("subscribers.byPwa")} groups={s.byPwa} label={(k) => names.pwa.get(k) ?? k} />
          <GroupCard title={t("subscribers.byCampaign")} groups={s.byCampaign} label={(k) => (k ? (names.campaign.get(k) ?? k) : none)} />
          <GroupCard title={t("subscribers.byCountry")} groups={s.byCountry} label={(k) => k || none} />
        </div>
      )}
      <DataTable
        columns={subscriberColumns(t, names)}
        data={list.data.subscriptions}
        searchPlaceholder={t("subscribers.searchPlaceholder")}
        exportFilename="push-subscribers"
        emptyTitle={t("subscribers.emptyTitle")}
        emptyDescription={t("subscribers.emptyDescription")}
        pageSize={20}
      />
    </div>
  );
}

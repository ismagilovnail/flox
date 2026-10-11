"use client";

import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { RelativeTime } from "@/components/ui/relative-time";
import { usePixelDeliveries } from "@/hooks/use-pixels";
import type { PixelDeliveryStatus } from "@/lib/api/pixels";

const STATUS_VARIANT: Record<PixelDeliveryStatus, "success" | "warning" | "danger" | "secondary"> = {
  queued: "secondary",
  processing: "secondary",
  success: "success",
  retrying: "warning",
  dead: "danger",
};

/** The pixel's recent deliveries (the last 100): what was sent to the
 * platform, how it answered, and why a delivery failed — the first place
 * an operator checks a new pixel's setup. */
export function PixelDeliveryLog({ pixelId }: { pixelId: string }) {
  const { t } = useTranslation("pixels");
  const query = usePixelDeliveries(pixelId);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("deliveries.title")}</CardTitle>
        <CardDescription>{t("deliveries.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        {query.isPending ? (
          <LoadingState label={t("deliveries.loading")} />
        ) : query.isError ? (
          <ErrorState title={t("deliveries.loadError")} description={query.error.message} onRetry={() => query.refetch()} />
        ) : query.data.deliveries.length === 0 ? (
          <EmptyState title={t("deliveries.emptyTitle")} description={t("deliveries.emptyDescription")} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tabular-nums">
              <thead>
                <tr className="border-b text-left text-xs text-muted-foreground">
                  <th className="py-2 pr-3 font-medium">{t("deliveries.columns.when")}</th>
                  <th className="py-2 pr-3 font-medium">{t("deliveries.columns.event")}</th>
                  <th className="py-2 pr-3 font-medium">{t("deliveries.columns.click")}</th>
                  <th className="py-2 pr-3 font-medium">{t("deliveries.columns.status")}</th>
                  <th className="py-2 pr-3 text-right font-medium">{t("deliveries.columns.attempts")}</th>
                  <th className="py-2 pr-3 text-right font-medium">{t("deliveries.columns.http")}</th>
                  <th className="py-2 font-medium">{t("deliveries.columns.message")}</th>
                </tr>
              </thead>
              <tbody>
                {query.data.deliveries.map((d) => (
                  <tr key={d.id} className="border-b last:border-0 align-top">
                    <td className="py-2 pr-3 whitespace-nowrap text-muted-foreground">
                      <RelativeTime date={d.createdAt} />
                    </td>
                    <td className="py-2 pr-3 font-mono text-xs">{d.eventType}</td>
                    <td className="py-2 pr-3 font-mono text-xs text-muted-foreground">{d.clickId}</td>
                    <td className="py-2 pr-3">
                      <Badge variant={STATUS_VARIANT[d.status]}>{t(`deliveries.status.${d.status}`)}</Badge>
                    </td>
                    <td className="py-2 pr-3 text-right">{d.attemptCount}</td>
                    <td className="py-2 pr-3 text-right">{d.responseStatusCode ?? "—"}</td>
                    <td className="py-2 text-xs break-words text-muted-foreground">{d.message || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

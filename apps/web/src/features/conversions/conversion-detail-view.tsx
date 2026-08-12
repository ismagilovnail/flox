"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { SendIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { ErrorState } from "@/components/ui/error-state";
import { StatCard } from "@/components/ui/stat-card";
import { Mono } from "@/components/ui/typography";
import { useConversionsStore } from "@/stores/conversions";
import { useCampaignsStore } from "@/stores/campaigns";
import { useOffersStore } from "@/stores/offers";
import { useNetworksStore } from "@/stores/networks";
import { generateConversionTimeline, type CpaStatus, type PostbackDeliveryStatus } from "@/lib/mock/conversions";
import { ConversionTimeline } from "@/features/conversions/conversion-timeline";

const STATUS_VARIANT: Record<CpaStatus, "warning" | "success" | "danger" | "secondary"> = {
  CPA_HOLD: "warning",
  CPA_ACCEPT: "success",
  CPA_REDEP: "success",
  CPA_DECLINE: "danger",
  CPA_TRASH: "secondary",
};

const POSTBACK_VARIANT: Record<PostbackDeliveryStatus, "success" | "warning" | "danger" | "outline"> = {
  sent: "success",
  pending: "warning",
  failed: "danger",
  not_configured: "outline",
};

export function ConversionDetailView({ id }: { id: string }) {
  const conversion = useConversionsStore((s) => s.getById(id));
  const resendPostback = useConversionsStore((s) => s.resendPostback);
  const campaign = useCampaignsStore((s) => (conversion ? s.getById(conversion.campaignId) : undefined));
  const offer = useOffersStore((s) => (conversion ? s.getById(conversion.offerId) : undefined));
  const network = useNetworksStore((s) => (conversion ? s.getById(conversion.networkId) : undefined));
  const router = useRouter();

  const timeline = React.useMemo(() => (conversion ? generateConversionTimeline(conversion) : []), [conversion]);

  if (!conversion) {
    return (
      <ErrorState
        title="Conversion not found"
        description="It may have been created in a previous session — mock data resets when the app reloads."
        onRetry={() => router.push("/conversions")}
      />
    );
  }

  function handleResend() {
    resendPostback(conversion!.id);
    toast("Postback resent", { description: conversion!.clickId });
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              <Mono>{conversion.clickId}</Mono>
            </h1>
            <Badge variant={STATUS_VARIANT[conversion.status]}>{conversion.status.replace("CPA_", "")}</Badge>
          </div>
          <p className="text-xs text-muted-foreground">
            {campaign?.name ?? conversion.campaignId} · {offer?.name ?? conversion.offerId} ·{" "}
            {network?.name ?? conversion.networkId}
          </p>
        </div>
        <Button variant="outline" onClick={handleResend} disabled={conversion.postbackStatus === "not_configured"}>
          <SendIcon className="size-4" />
          Resend postback
        </Button>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard
          label="Revenue"
          value={conversion.revenue === 0 ? "—" : `${conversion.revenue.toFixed(2)} ${conversion.currency}`}
        />
        <StatCard
          label="Postback"
          value={
            <Badge variant={POSTBACK_VARIANT[conversion.postbackStatus]} className="text-base">
              {conversion.postbackStatus.replace("_", " ")}
            </Badge>
          }
        />
        <StatCard label="Event time" value={new Date(conversion.eventAt).toLocaleString("en-US")} />
        <StatCard label="Network" value={network?.name ?? conversion.networkId} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Timeline</CardTitle>
          <CardDescription>Click → Landing → PWA → Offer → Conversion → Postback (§29).</CardDescription>
        </CardHeader>
        <CardContent>
          <ConversionTimeline steps={timeline} />
        </CardContent>
      </Card>
    </div>
  );
}

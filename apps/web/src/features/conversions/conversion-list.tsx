"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useConversions } from "@/hooks/use-conversions";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useNetworks } from "@/hooks/use-networks";
import { useFlows } from "@/hooks/use-flows";
import { useOffers } from "@/hooks/use-offers";
import { conversionColumns, CONVERSION_HIDDEN_BY_DEFAULT } from "@/features/conversions/conversion-columns";

export function ConversionList() {
  const { t } = useTranslation("conversions");
  const conversionsQuery = useConversions();
  const campaignsQuery = useCampaigns();
  const networksQuery = useNetworks();
  const flowsQuery = useFlows();
  const offersQuery = useOffers();

  const campaignNameById = React.useMemo(
    () => Object.fromEntries((campaignsQuery.data?.campaigns ?? []).map((c) => [c.id, c.name])),
    [campaignsQuery.data],
  );
  const networkNameById = React.useMemo(
    () => Object.fromEntries((networksQuery.data?.networks ?? []).map((n) => [n.id, n.name])),
    [networksQuery.data],
  );

  const flowNameById = React.useMemo(
    () => Object.fromEntries((flowsQuery.data?.flows ?? []).map((f) => [f.id, f.name])),
    [flowsQuery.data],
  );
  const offerNameById = React.useMemo(
    () => Object.fromEntries((offersQuery.data?.offers ?? []).map((o) => [o.id, o.name])),
    [offersQuery.data],
  );

  const columns = React.useMemo(
    () => conversionColumns(t, campaignNameById, networkNameById, flowNameById, offerNameById),
    [t, campaignNameById, networkNameById, flowNameById, offerNameById],
  );

  if (conversionsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">{t("list.title")}</h1>
        <LoadingState label={t("list.loading")} />
      </div>
    );
  }

  if (conversionsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">{t("list.title")}</h1>
        <ErrorState
          title={t("list.loadError")}
          description={conversionsQuery.error.message}
          onRetry={() => conversionsQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title")}</h1>

      <DataTable
        columns={columns}
        data={conversionsQuery.data.conversions}
        getRowId={(row) => `${row.clickId}:${row.type}:${row.eventAt}`}
        exportFilename="conversions"
        searchPlaceholder={t("list.searchPlaceholder")}
        emptyTitle={t("list.emptyTitle")}
        emptyDescription={t("list.emptyDescription")}
        pageSize={15}
        initialColumnVisibility={CONVERSION_HIDDEN_BY_DEFAULT}
        enableColumnReorder
        persistKey="conversions"
      />
    </div>
  );
}

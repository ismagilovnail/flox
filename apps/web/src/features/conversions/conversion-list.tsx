"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { useRouter } from "next/navigation";

import { LogGrid } from "@/components/shared/log-grid";
import { useConversions } from "@/hooks/use-conversions";
import { useLogGrid } from "@/hooks/use-log-grid";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import { listConversions, type Conversion, type ListConversionsParams } from "@/lib/api/conversions";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useNetworks } from "@/hooks/use-networks";
import { useFlows } from "@/hooks/use-flows";
import { useOffers } from "@/hooks/use-offers";
import { conversionColumns, CONVERSION_HIDDEN_BY_DEFAULT } from "@/features/conversions/conversion-columns";

const fetchConversionPage = (filter: ListConversionsParams, offset: number, limit: number) =>
  listConversions({ ...filter, offset, limit }).then((r) => ({ rows: r.conversions, total: r.total }));

export function ConversionList() {
  const { t } = useTranslation("conversions");
  const router = useRouter();
  const errorMessage = useApiErrorMessage();
  const grid = useLogGrid();
  const conversionsQuery = useConversions(grid.params);
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

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title")}</h1>

      <LogGrid<Conversion>
        grid={grid}
        query={conversionsQuery}
        select={() => conversionsQuery.data && { rows: conversionsQuery.data.conversions, total: conversionsQuery.data.total }}
        fetchPage={fetchConversionPage}
        loadingLabel={t("list.loading")}
        errorTitle={t("list.loadError")}
        errorMessage={errorMessage}
        ariaLabel={t("list.title")}
        columns={columns}
        getRowId={(row) => `${row.clickId}:${row.type}:${row.eventAt}`}
        onRowActivate={(row) => router.push(`/conversions/${row.clickId}`)}
        exportFilename="conversions"
        emptyTitle={t("list.emptyTitle")}
        emptyDescription={t("list.emptyDescription")}
        initialColumnVisibility={CONVERSION_HIDDEN_BY_DEFAULT}
        enableColumnReorder
        persistKey="conversions"
      />
    </div>
  );
}

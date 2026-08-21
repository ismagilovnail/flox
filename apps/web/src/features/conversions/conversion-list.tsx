"use client";

import * as React from "react";

import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useConversions } from "@/hooks/use-conversions";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useNetworks } from "@/hooks/use-networks";
import { conversionColumns } from "@/features/conversions/conversion-columns";

export function ConversionList() {
  const conversionsQuery = useConversions();
  const campaignsQuery = useCampaigns();
  const networksQuery = useNetworks();

  const campaignNameById = React.useMemo(
    () => Object.fromEntries((campaignsQuery.data?.campaigns ?? []).map((c) => [c.id, c.name])),
    [campaignsQuery.data],
  );
  const networkNameById = React.useMemo(
    () => Object.fromEntries((networksQuery.data?.networks ?? []).map((n) => [n.id, n.name])),
    [networksQuery.data],
  );

  const columns = React.useMemo(
    () => conversionColumns(campaignNameById, networkNameById),
    [campaignNameById, networkNameById],
  );

  if (conversionsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">Conversions</h1>
        <LoadingState label="Loading conversions…" />
      </div>
    );
  }

  if (conversionsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">Conversions</h1>
        <ErrorState
          title="Couldn't load conversions"
          description={conversionsQuery.error.message}
          onRetry={() => conversionsQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">Conversions</h1>

      <DataTable
        columns={columns}
        data={conversionsQuery.data.conversions}
        getRowId={(row) => `${row.clickId}:${row.type}:${row.eventAt}`}
        searchPlaceholder="Search by click ID..."
        emptyTitle="No conversions yet"
        emptyDescription="CPA events will show up here once the tracker starts recording postbacks."
        pageSize={15}
      />
    </div>
  );
}

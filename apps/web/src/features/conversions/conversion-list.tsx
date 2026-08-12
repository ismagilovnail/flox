"use client";

import * as React from "react";

import { DataTable } from "@/components/ui/data-table";
import { useConversionsStore } from "@/stores/conversions";
import { useCampaignsStore } from "@/stores/campaigns";
import { useOffersStore } from "@/stores/offers";
import { conversionColumns } from "@/features/conversions/conversion-columns";

export function ConversionList() {
  const conversions = useConversionsStore((s) => s.conversions);
  const campaigns = useCampaignsStore((s) => s.campaigns);
  const offers = useOffersStore((s) => s.offers);

  const campaignNameById = React.useMemo(
    () => Object.fromEntries(campaigns.map((c) => [c.id, c.name])),
    [campaigns],
  );
  const offerNameById = React.useMemo(() => Object.fromEntries(offers.map((o) => [o.id, o.name])), [offers]);

  const columns = React.useMemo(
    () => conversionColumns(campaignNameById, offerNameById),
    [campaignNameById, offerNameById],
  );

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">Conversions</h1>

      <DataTable
        columns={columns}
        data={conversions}
        searchPlaceholder="Search by click ID..."
        emptyTitle="No conversions yet"
        emptyDescription="CPA events will show up here once the tracker starts recording postbacks."
        pageSize={15}
      />
    </div>
  );
}

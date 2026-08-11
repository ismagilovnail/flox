"use client";

import Link from "next/link";
import { PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useCampaignsStore } from "@/stores/campaigns";
import { campaignColumns } from "@/features/campaigns/campaign-columns";

export function CampaignList() {
  const campaigns = useCampaignsStore((s) => s.campaigns);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Campaigns</h1>
        <Button asChild>
          <Link href="/campaigns/new">
            <PlusIcon className="size-4" />
            New Campaign
          </Link>
        </Button>
      </div>

      <DataTable
        columns={campaignColumns}
        data={campaigns}
        searchPlaceholder="Search campaigns..."
        emptyTitle="No campaigns yet"
        emptyDescription="Create your first campaign to start routing traffic."
        pageSize={10}
      />
    </div>
  );
}

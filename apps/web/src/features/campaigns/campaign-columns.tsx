import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Mono, Caption } from "@/components/ui/typography";
import { formatUsd, formatInt } from "@/lib/format";
import type { Campaign, CampaignStatus } from "@/lib/mock/campaigns";
import { CampaignRowActions } from "@/features/campaigns/campaign-row-actions";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

const STATUS_VARIANT: Record<CampaignStatus, "success" | "warning" | "outline" | "secondary"> = {
  active: "success",
  paused: "warning",
  draft: "outline",
  archived: "secondary",
};

export const campaignColumns: ColumnDef<typeof dataTableFeatures, Campaign>[] = [
  {
    accessorKey: "name",
    header: "Name",
    cell: ({ row }) => (
      <Link href={`/campaigns/${row.original.id}`} className="font-medium text-foreground hover:underline">
        {row.original.name}
      </Link>
    ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ getValue }) => {
      const status = getValue() as CampaignStatus;
      return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
    },
  },
  { accessorKey: "source", header: "Source" },
  {
    id: "tags",
    header: "Tags",
    enableSorting: false,
    cell: ({ row }) => <TagBadgeList entityType="campaign" entityId={row.original.id} />,
  },
  {
    accessorKey: "clicks",
    header: "Clicks",
    cell: ({ getValue }) => <Mono>{formatInt(getValue() as number)}</Mono>,
  },
  {
    accessorKey: "conversions",
    header: "Conversions",
    cell: ({ getValue }) => <Mono>{formatInt(getValue() as number)}</Mono>,
  },
  {
    accessorKey: "revenue",
    header: "Revenue",
    cell: ({ getValue }) => <Mono>{formatUsd(getValue() as number, 2)}</Mono>,
  },
  {
    accessorKey: "spend",
    header: "Spend",
    cell: ({ getValue }) => {
      const spend = getValue() as number | null;
      return <Mono>{spend === null ? "—" : formatUsd(spend, 2)}</Mono>;
    },
  },
  {
    id: "profit",
    header: "Profit",
    accessorFn: (row) => (row.spend === null ? null : row.revenue - row.spend),
    cell: ({ getValue }) => {
      const profit = getValue() as number | null;
      return <Mono>{profit === null ? "—" : formatUsd(profit, 2)}</Mono>;
    },
  },
  {
    id: "roi",
    header: "ROI",
    accessorFn: (row) => (row.spend && row.spend > 0 ? ((row.revenue - row.spend) / row.spend) * 100 : null),
    cell: ({ getValue }) => {
      const roi = getValue() as number | null;
      return (
        <Mono className={roi === null ? "text-muted-foreground" : roi >= 0 ? "text-success" : "text-danger"}>
          {roi === null ? "—" : `${roi > 0 ? "+" : ""}${roi.toFixed(1)}%`}
        </Mono>
      );
    },
  },
  {
    accessorKey: "updatedAt",
    header: "Updated",
    cell: ({ getValue }) => (
      <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>
    ),
  },
  {
    id: "actions",
    header: "",
    enableSorting: false,
    enableHiding: false,
    cell: ({ row }) => (
      <div className="flex justify-end">
        <CampaignRowActions campaign={row.original} />
      </div>
    ),
  },
];

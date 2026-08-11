import type { ColumnDef } from "@tanstack/react-table";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Mono } from "@/components/ui/typography";
import { formatUsd, formatInt } from "@/lib/format";
import type { PerformanceRow } from "@/lib/mock/dashboard";

function columns(nameHeader: string): ColumnDef<typeof dataTableFeatures, PerformanceRow>[] {
  return [
    { accessorKey: "name", header: nameHeader },
    {
      accessorKey: "clicks",
      header: "Clicks",
      cell: ({ getValue }) => <Mono>{formatInt(getValue() as number)}</Mono>,
    },
    {
      accessorKey: "cvr",
      header: "CVR",
      cell: ({ getValue }) => <Mono>{((getValue() as number) * 100).toFixed(2)}%</Mono>,
    },
    {
      accessorKey: "revenue",
      header: "Revenue",
      cell: ({ getValue }) => <Mono>{formatUsd(getValue() as number)}</Mono>,
    },
    {
      id: "roi",
      accessorKey: "roi",
      header: "ROI",
      cell: ({ row }) => {
        const roi = row.original.roi;
        return (
          <Mono className={roi === null ? "text-muted-foreground" : roi >= 0 ? "text-success" : "text-danger"}>
            {roi === null ? "—" : `${roi > 0 ? "+" : ""}${roi.toFixed(1)}%`}
          </Mono>
        );
      },
    },
  ];
}

export function TopTable({
  title,
  nameHeader,
  rows,
}: {
  title: string;
  nameHeader: string;
  rows: PerformanceRow[];
}) {
  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-semibold text-foreground">{title}</h3>
      <DataTable
        columns={columns(nameHeader)}
        data={rows}
        searchPlaceholder={`Search ${nameHeader.toLowerCase()}s...`}
        emptyTitle={`No ${nameHeader.toLowerCase()}s`}
        pageSize={5}
      />
    </div>
  );
}

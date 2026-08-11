import type { ColumnDef } from "@tanstack/react-table";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Mono } from "@/components/ui/typography";
import { DIMENSIONS, METRICS, formatMetric, type DimensionKey, type MetricKey } from "@/features/analytics/registry";
import type { ReportRow } from "@/features/analytics/aggregate";

export function ReportTable({
  rows,
  dimensions,
  metrics,
}: {
  rows: ReportRow[];
  dimensions: DimensionKey[];
  metrics: MetricKey[];
}) {
  const columns: ColumnDef<typeof dataTableFeatures, ReportRow>[] = [
    ...(dimensions.length === 0
      ? [
          {
            id: "all",
            header: "All traffic",
            accessorFn: () => "All traffic",
          } satisfies ColumnDef<typeof dataTableFeatures, ReportRow>,
        ]
      : dimensions.map(
          (d): ColumnDef<typeof dataTableFeatures, ReportRow> => ({
            id: d,
            header: DIMENSIONS.find((x) => x.key === d)?.label ?? d,
            accessorFn: (row) => row.dims[d] ?? "",
          }),
        )),
    ...metrics.map((m): ColumnDef<typeof dataTableFeatures, ReportRow> => {
      const meta = METRICS.find((x) => x.key === m)!;
      return {
        id: m,
        header: meta.label,
        accessorFn: (row) => row.metrics[m],
        cell: ({ getValue }) => <Mono>{formatMetric(getValue() as number | null, meta.format)}</Mono>,
      };
    }),
  ];

  return (
    <DataTable
      columns={columns}
      data={rows}
      searchPlaceholder="Search rows..."
      emptyTitle="No data for this selection"
      emptyDescription="Widen the date range or remove a filter."
      pageSize={10}
    />
  );
}

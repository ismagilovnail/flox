import type { ColumnDef } from "@tanstack/react-table";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Mono } from "@/components/ui/typography";
import { DIMENSIONS, METRICS, formatMetric, type DimensionKey, type MetricKey } from "@/features/analytics/registry";
import type { ReportRow } from "@/features/analytics/aggregate";
import { evaluateFormula } from "@/lib/formula-engine";
import type { CustomMetric } from "@/lib/mock/custom-metrics";

export function ReportTable({
  rows,
  dimensions,
  metrics,
  customMetrics = [],
}: {
  rows: ReportRow[];
  dimensions: DimensionKey[];
  metrics: MetricKey[];
  /** Published + active custom metrics targeting the report_builder surface (§30.5) —
   * evaluated live against each row's own metrics, same formula engine everywhere. */
  customMetrics?: CustomMetric[];
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
    ...customMetrics.map((cm): ColumnDef<typeof dataTableFeatures, ReportRow> => ({
      id: `cm_${cm.id}`,
      header: cm.name,
      accessorFn: (row) => evaluateFormula(cm.formula, row.metrics),
      cell: ({ getValue }) => <Mono>{formatMetric(getValue() as number | null, cm.format)}</Mono>,
    })),
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

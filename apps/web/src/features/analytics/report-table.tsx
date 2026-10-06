import type { ColumnDef } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Mono } from "@/components/ui/typography";
import { METRICS, dimensionLabel, metricLabel, formatMetric, type DimensionKey, type MetricKey } from "@/features/analytics/registry";
import type { ReportRow } from "@/features/analytics/aggregate";
import { evaluateFormula } from "@/lib/formula-engine";
import type { CustomMetric } from "@/lib/custom-metrics";
import { useDimensionValueLabel } from "@/features/analytics/use-dimension-value-label";

export function ReportTable({
  rows,
  dimensions,
  metrics,
  customMetrics = [],
  columnOrder,
  onColumnOrderChange,
  columnSizing,
  onColumnSizingChange,
}: {
  rows: ReportRow[];
  dimensions: DimensionKey[];
  metrics: MetricKey[];
  /** Published + active custom metrics targeting the report_builder surface (§30.5) —
   * evaluated live against each row's own metrics, same formula engine everywhere. */
  customMetrics?: CustomMetric[];
  /** The dragged column order (ids); [] is the natural order. Lives in the report state so a preset can save it. */
  columnOrder?: string[];
  onColumnOrderChange?: (order: string[]) => void;
  /** Dragged column widths in px by column id; {} is natural widths. Lives in the report state so a preset can save it. */
  columnSizing?: Record<string, number>;
  onColumnSizingChange?: (sizing: Record<string, number>) => void;
}) {
  const { t } = useTranslation("analytics");
  const valueLabel = useDimensionValueLabel();

  const columns: ColumnDef<typeof dataTableFeatures, ReportRow>[] = [
    ...(dimensions.length === 0
      ? [
          {
            id: "all",
            header: t("table.allTraffic"),
            accessorFn: () => t("table.allTraffic"),
          } satisfies ColumnDef<typeof dataTableFeatures, ReportRow>,
        ]
      : dimensions.map(
          (d): ColumnDef<typeof dataTableFeatures, ReportRow> => ({
            id: d,
            header: dimensionLabel(t, d),
            // The label, not the raw id: what the cell shows is also what
            // the table's search matches and its CSV export writes.
            accessorFn: (row) => valueLabel(d, row.dims[d] ?? ""),
          }),
        )),
    ...metrics.map((m): ColumnDef<typeof dataTableFeatures, ReportRow> => {
      const meta = METRICS.find((x) => x.key === m)!;
      return {
        id: m,
        header: metricLabel(t, m),
        meta: { hint: t(`metricHints.${m}`, { defaultValue: "" }) || undefined },
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
      searchPlaceholder={t("table.searchPlaceholder")}
      exportFilename="analytics-report"
      enableColumnReorder
      columnOrder={columnOrder}
      columnSizing={columnSizing}
      onColumnSizingChange={onColumnSizingChange}
      onColumnOrderChange={(order) => {
        // Dragged back to the natural arrangement = no custom order at all, so
        // the preset isn't shown as modified for a change that changed nothing.
        const natural = columns.map((c) => c.id as string)
        const same = order.length === natural.length && order.every((id, i) => id === natural[i])
        onColumnOrderChange?.(same ? [] : order)
      }}
      emptyTitle={t("table.emptyTitle")}
      emptyDescription={t("table.emptyDescription")}
      pageSize={10}
    />
  );
}

import type { ColumnDef } from "@tanstack/react-table";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Mono } from "@/components/ui/typography";
import { useReportingCurrency } from "@/hooks/use-organization";
import { formatMoney, formatInt } from "@/lib/format";
import type { PerformanceRow } from "@/features/dashboard/metrics";

type Entity = "campaigns" | "countries" | "flows" | "offers";

const NAME_HEADER_KEY: Record<Entity, string> = {
  campaigns: "campaignHeader",
  countries: "countryHeader",
  flows: "flowHeader",
  offers: "offerHeader",
};

function columns(t: TFunction, nameHeader: string, nameHint: string, currency: string): ColumnDef<typeof dataTableFeatures, PerformanceRow>[] {
  return [
    { accessorKey: "name", header: nameHeader, meta: { hint: nameHint } },
    {
      accessorKey: "clicks",
      header: t("topTables.clicksHeader", { ns: "dashboard" }),
      meta: { hint: t("topTables.clicksHint", { ns: "dashboard" }) },
      cell: ({ getValue }) => <Mono>{formatInt(getValue() as number)}</Mono>,
    },
    {
      accessorKey: "cvr",
      header: t("topTables.cvrHeader", { ns: "dashboard" }),
      meta: { hint: t("topTables.cvrHint", { ns: "dashboard" }) },
      cell: ({ getValue }) => <Mono>{((getValue() as number) * 100).toFixed(2)}%</Mono>,
    },
    {
      accessorKey: "revenue",
      header: t("topTables.revenueHeader", { ns: "dashboard" }),
      meta: { hint: t("topTables.revenueHint", { ns: "dashboard" }) },
      cell: ({ getValue }) => {
        const v = getValue() as number | null;
        return <Mono className={v === null ? "text-muted-foreground" : undefined}>{v === null ? "—" : formatMoney(v, currency)}</Mono>;
      },
    },
    {
      id: "roi",
      accessorKey: "roi",
      header: t("topTables.roiHeader", { ns: "dashboard" }),
      meta: { hint: t("topTables.roiHint", { ns: "dashboard" }) },
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

export function TopTable({ entity, rows }: { entity: Entity; rows: PerformanceRow[] }) {
  const { t } = useTranslation("dashboard");
  const currency = useReportingCurrency();
  const nameHeader = t(`topTables.${NAME_HEADER_KEY[entity]}`);

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-semibold text-foreground">{t(`topTables.${entity}Title`)}</h3>
      <DataTable
        columns={columns(t, nameHeader, t(`topTables.${entity}Hint`), currency)}
        data={rows}
        searchPlaceholder={t(`topTables.${entity}SearchPlaceholder`)}
        emptyTitle={t(`topTables.${entity}EmptyTitle`)}
        pageSize={5}
        exportFilename={`top-${entity}`}
        persistKey={`dashboard.top-${entity}`}
      />
    </div>
  );
}

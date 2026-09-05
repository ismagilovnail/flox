import type { ColumnDef } from "@tanstack/react-table";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import type { ClickLog } from "@/lib/api/click-logs";

const SUB_COUNT = 10;

/** Always-visible core columns plus every remaining click_events field
 * (geo/device detail, UTM, subs, ids) reachable via the Columns menu — a
 * click row has far more fields than fit a default view, but nothing on
 * the schema is left un-surfaceable (§48, docs/click-log.md). */
export const CLICK_LOG_INITIAL_COLUMN_VISIBILITY: Record<string, boolean> = {
  region: false,
  platform: false,
  osVersion: false,
  browserVersion: false,
  language: false,
  isBot: false,
  isProxy: false,
  asn: false,
  connectionType: false,
  userAgent: false,
  referrer: false,
  utmSource: false,
  utmMedium: false,
  utmCampaign: false,
  utmContent: false,
  utmTerm: false,
  ...Object.fromEntries(Array.from({ length: SUB_COUNT }, (_, i) => [`sub${i + 1}`, false])),
  externalClickId: false,
  fbClickId: false,
  ttClickId: false,
  destination: false,
  streamSetId: false,
  flowId: false,
  stickyApplied: false,
  filterReason: false,
};

function Mut({ value }: { value?: string }) {
  return value ? <span className="text-xs">{value}</span> : <span className="text-xs text-muted-foreground">—</span>;
}

export function clickLogColumns(
  t: TFunction,
  campaignNameById: Record<string, string>,
): ColumnDef<typeof dataTableFeatures, ClickLog>[] {
  const subColumns: ColumnDef<typeof dataTableFeatures, ClickLog>[] = Array.from({ length: SUB_COUNT }, (_, i) => {
    const n = i + 1;
    const key = `sub${n}` as keyof ClickLog;
    return {
      accessorKey: key,
      header: t("columns.sub", { ns: "clickLog", n }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    };
  });

  return [
    {
      accessorKey: "eventAt",
      header: t("columns.time", { ns: "clickLog" }),
      cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
    },
    {
      accessorKey: "type",
      header: t("columns.type", { ns: "clickLog" }),
      cell: ({ getValue }) => {
        const type = getValue() as ClickLog["type"];
        return (
          <Badge variant={type === "SOURCE_FILTER" ? "danger" : "success"}>
            {t(`type.${type}`, { ns: "clickLog" })}
          </Badge>
        );
      },
    },
    {
      id: "campaign",
      header: t("columns.campaign", { ns: "clickLog" }),
      // `cell`, not `accessorFn` — see campaign-columns.tsx's own comment on
      // this exact pattern: accessorFn's return value is cached per row and
      // only re-evaluated when the `data` array's reference changes, not
      // when campaignNameById (populated by a separate, later-resolving
      // query) changes.
      cell: ({ row }) => campaignNameById[row.original.campaignId] ?? row.original.campaignId,
    },
    {
      accessorKey: "clickId",
      header: t("columns.clickId", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mono className="text-xs">{getValue() as string}</Mono>,
    },
    {
      accessorKey: "country",
      header: t("columns.country", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "city",
      header: t("columns.city", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "device",
      header: t("columns.device", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "os",
      header: t("columns.os", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "browser",
      header: t("columns.browser", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "ip",
      header: t("columns.ip", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mono className="text-xs">{(getValue() as string | undefined) || "—"}</Mono>,
    },
    {
      accessorKey: "region",
      header: t("columns.region", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "platform",
      header: t("columns.platform", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "osVersion",
      header: t("columns.osVersion", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "browserVersion",
      header: t("columns.browserVersion", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "language",
      header: t("columns.language", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "isBot",
      header: t("columns.bot", { ns: "clickLog" }),
      cell: ({ getValue }) => (getValue() ? <Badge variant="warning">{t("yes", { ns: "clickLog" })}</Badge> : <Mut />),
    },
    {
      accessorKey: "isProxy",
      header: t("columns.proxy", { ns: "clickLog" }),
      cell: ({ getValue }) => (getValue() ? <Badge variant="warning">{t("yes", { ns: "clickLog" })}</Badge> : <Mut />),
    },
    {
      accessorKey: "asn",
      header: t("columns.asn", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "connectionType",
      header: t("columns.connectionType", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "userAgent",
      header: t("columns.userAgent", { ns: "clickLog" }),
      cell: ({ getValue }) => <Caption className="block max-w-xs truncate">{(getValue() as string) || "—"}</Caption>,
    },
    {
      accessorKey: "referrer",
      header: t("columns.referrer", { ns: "clickLog" }),
      cell: ({ getValue }) => <Caption className="block max-w-xs truncate">{(getValue() as string) || "—"}</Caption>,
    },
    {
      accessorKey: "utmSource",
      header: t("columns.utmSource", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "utmMedium",
      header: t("columns.utmMedium", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "utmCampaign",
      header: t("columns.utmCampaign", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "utmContent",
      header: t("columns.utmContent", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    {
      accessorKey: "utmTerm",
      header: t("columns.utmTerm", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mut value={getValue() as string | undefined} />,
    },
    ...subColumns,
    {
      accessorKey: "externalClickId",
      header: t("columns.externalClickId", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mono className="text-xs">{(getValue() as string) || "—"}</Mono>,
    },
    {
      accessorKey: "fbClickId",
      header: t("columns.fbClickId", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mono className="text-xs">{(getValue() as string) || "—"}</Mono>,
    },
    {
      accessorKey: "ttClickId",
      header: t("columns.ttClickId", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mono className="text-xs">{(getValue() as string) || "—"}</Mono>,
    },
    {
      accessorKey: "destination",
      header: t("columns.destination", { ns: "clickLog" }),
      cell: ({ getValue }) => <Caption className="block max-w-xs truncate">{(getValue() as string) || "—"}</Caption>,
    },
    {
      accessorKey: "streamSetId",
      header: t("columns.streamSetId", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mono className="text-xs">{(getValue() as string) || "—"}</Mono>,
    },
    {
      accessorKey: "flowId",
      header: t("columns.flowId", { ns: "clickLog" }),
      cell: ({ getValue }) => <Mono className="text-xs">{(getValue() as string) || "—"}</Mono>,
    },
    {
      accessorKey: "stickyApplied",
      header: t("columns.sticky", { ns: "clickLog" }),
      cell: ({ getValue }) => (getValue() ? <Badge variant="secondary">{t("yes", { ns: "clickLog" })}</Badge> : <Mut />),
    },
    {
      accessorKey: "filterReason",
      header: t("columns.filterReason", { ns: "clickLog" }),
      cell: ({ getValue }) => <Caption className="block max-w-xs truncate">{(getValue() as string) || "—"}</Caption>,
    },
  ];
}

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { format } from "date-fns";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { Domain, DomainStatus, SslStatus } from "@/lib/api/domains";
import { DomainRowActions } from "@/features/domains/domain-row-actions";

export const STATUS_VARIANT: Record<DomainStatus, "success" | "warning" | "danger" | "secondary"> = {
  active: "success",
  pending: "warning",
  error: "danger",
  expired: "secondary",
};

export const SSL_VARIANT: Record<SslStatus, "success" | "warning" | "danger" | "outline"> = {
  issued: "success",
  pending: "warning",
  error: "danger",
  none: "outline",
};

const SSL_SOON_MS = 1000 * 60 * 60 * 24 * 14;

function expiresSoon(expires: Date): boolean {
  return expires.getTime() - Date.now() < SSL_SOON_MS;
}

/** The SSL status from the last real TLS check, with the certificate's
 * expiry under it and the check's error (or what "none"/"pending" means)
 * on hover. */
export function SslBadge({ domain }: { domain: Domain }) {
  const { t } = useTranslation("domains");
  const expires = domain.sslExpiresAt ? new Date(domain.sslExpiresAt) : null;
  const soon = expires !== null && expiresSoon(expires);
  const hint =
    domain.sslError ||
    (domain.ssl === "issued"
      ? t("ssl.hints.issued")
      : domain.ssl === "none" && !domain.sslCheckedAt
        ? t("ssl.hints.unchecked")
        : domain.ssl === "pending"
          ? t("ssl.hints.pending")
          : t("ssl.hints.none"));
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="flex flex-col items-start gap-0.5">
          <Badge variant={SSL_VARIANT[domain.ssl]}>{t(`ssl.statuses.${domain.ssl}`)}</Badge>
          {domain.ssl === "issued" && expires && (
            <Caption className={soon ? "text-warning" : "text-muted-foreground"}>
              {t("ssl.until", { date: format(expires, "dd.MM.yyyy") })}
            </Caption>
          )}
        </div>
      </TooltipTrigger>
      <TooltipContent className="max-w-80 break-words">
        {hint}
        {domain.sslCheckedAt && (
          <>
            {" · "}
            {t("ssl.checked")} <RelativeTime date={domain.sslCheckedAt} className="text-inherit" />
          </>
        )}
      </TooltipContent>
    </Tooltip>
  );
}

export function domainColumns(t: TFunction<"domains">): ColumnDef<typeof dataTableFeatures, Domain>[] {
  return [
    {
      accessorKey: "domain",
      header: t("columns.domain"),
      cell: ({ row }) => (
        <Link href={`/domains/${row.original.id}`} className="font-mono text-sm text-foreground hover:underline">
          {row.original.domain}
        </Link>
      ),
    },
    {
      accessorKey: "purpose",
      header: t("columns.purpose"),
      cell: ({ getValue }) => (
        <div className="flex flex-wrap gap-1">
          {(getValue() as string[]).map((p) => (
            <Badge key={p} variant="outline">
              {t(`purposes.${p}`)}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      accessorKey: "status",
      header: t("columns.status"),
      cell: ({ getValue }) => {
        const status = getValue() as DomainStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(`statuses.${status}`)}</Badge>;
      },
    },
    {
      accessorKey: "ssl",
      header: "SSL",
      cell: ({ row }) => <SslBadge domain={row.original} />,
    },
    {
      accessorKey: "expiresAt",
      header: t("columns.expires"),
      cell: ({ getValue }) => {
        const value = getValue() as string | null;
        if (!value) return <Caption className="text-muted-foreground">—</Caption>;
        const expires = new Date(value);
        const isSoon = expires.getTime() - Date.now() < 1000 * 60 * 60 * 24 * 30;
        return <RelativeTime date={expires} className={isSoon ? "text-warning" : undefined} />;
      },
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <DomainRowActions domain={row.original} />
        </div>
      ),
    },
  ];
}

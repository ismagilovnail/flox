import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { format, formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption } from "@/components/ui/typography";
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
  const expires = domain.sslExpiresAt ? new Date(domain.sslExpiresAt) : null;
  const soon = expires !== null && expiresSoon(expires);
  const hint =
    domain.sslError ||
    (domain.ssl === "issued"
      ? "Valid certificate"
      : domain.ssl === "none" && !domain.sslCheckedAt
        ? "Not checked yet"
        : domain.ssl === "pending"
          ? "Waiting for the certificate to be issued"
          : "No HTTPS on this domain yet");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="flex flex-col items-start gap-0.5">
          <Badge variant={SSL_VARIANT[domain.ssl]}>{domain.ssl}</Badge>
          {domain.ssl === "issued" && expires && (
            <Caption className={soon ? "text-warning" : "text-muted-foreground"}>
              until {format(expires, "dd.MM.yyyy")}
            </Caption>
          )}
        </div>
      </TooltipTrigger>
      <TooltipContent className="max-w-80 break-words">
        {hint}
        {domain.sslCheckedAt && (
          <> · checked {formatDistanceToNow(new Date(domain.sslCheckedAt), { addSuffix: true })}</>
        )}
      </TooltipContent>
    </Tooltip>
  );
}

export function domainColumns(): ColumnDef<typeof dataTableFeatures, Domain>[] {
  return [
    {
      accessorKey: "domain",
      header: "Domain",
      cell: ({ row }) => (
        <Link href={`/domains/${row.original.id}`} className="font-mono text-sm text-foreground hover:underline">
          {row.original.domain}
        </Link>
      ),
    },
    {
      accessorKey: "purpose",
      header: "Purpose",
      cell: ({ getValue }) => (
        <div className="flex flex-wrap gap-1">
          {(getValue() as string[]).map((p) => (
            <Badge key={p} variant="outline">
              {p}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ getValue }) => {
        const status = getValue() as DomainStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
      },
    },
    {
      accessorKey: "ssl",
      header: "SSL",
      cell: ({ row }) => <SslBadge domain={row.original} />,
    },
    {
      accessorKey: "expiresAt",
      header: "Expires",
      cell: ({ getValue }) => {
        const value = getValue() as string | null;
        if (!value) return <Caption className="text-muted-foreground">—</Caption>;
        const expires = new Date(value);
        const isSoon = expires.getTime() - Date.now() < 1000 * 60 * 60 * 24 * 30;
        return (
          <Caption className={isSoon ? "text-warning" : undefined}>
            {formatDistanceToNow(expires, { addSuffix: true })}
          </Caption>
        );
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

import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption } from "@/components/ui/typography";
import type { Domain, DomainStatus, SslStatus } from "@/lib/mock/domains";
import { DomainRowActions } from "@/features/domains/domain-row-actions";

const STATUS_VARIANT: Record<DomainStatus, "success" | "warning" | "danger" | "secondary"> = {
  active: "success",
  pending: "warning",
  error: "danger",
  expired: "secondary",
};

const SSL_VARIANT: Record<SslStatus, "success" | "warning" | "danger" | "outline"> = {
  issued: "success",
  pending: "warning",
  error: "danger",
  none: "outline",
};

export function domainColumns(onEdit: (domain: Domain) => void): ColumnDef<typeof dataTableFeatures, Domain>[] {
  return [
    {
      accessorKey: "domain",
      header: "Domain",
      cell: ({ row }) => (
        <button
          type="button"
          onClick={() => onEdit(row.original)}
          className="font-mono text-sm text-foreground hover:underline"
        >
          {row.original.domain}
        </button>
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
      cell: ({ getValue }) => {
        const ssl = getValue() as SslStatus;
        return <Badge variant={SSL_VARIANT[ssl]}>{ssl}</Badge>;
      },
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
          <DomainRowActions domain={row.original} onEdit={() => onEdit(row.original)} />
        </div>
      ),
    },
  ];
}

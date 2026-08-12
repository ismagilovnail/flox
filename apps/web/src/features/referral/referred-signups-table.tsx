"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption } from "@/components/ui/typography";
import { useReferralStore } from "@/stores/referral";
import type { ReferredSignup, SignupStatus } from "@/lib/mock/referral";

const STATUS_VARIANT: Record<SignupStatus, "warning" | "success" | "secondary"> = {
  trial: "warning",
  active_customer: "success",
  churned: "secondary",
};

const STATUS_LABEL: Record<SignupStatus, string> = {
  trial: "Trial",
  active_customer: "Active customer",
  churned: "Churned",
};

const columns: ColumnDef<typeof dataTableFeatures, ReferredSignup>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "email", header: "Email" },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ getValue }) => {
      const status = getValue() as SignupStatus;
      return <Badge variant={STATUS_VARIANT[status]}>{STATUS_LABEL[status]}</Badge>;
    },
  },
  {
    accessorKey: "signedUpAt",
    header: "Signed up",
    cell: ({ getValue }) => (
      <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>
    ),
  },
];

export function ReferredSignupsTable() {
  const signups = useReferralStore((s) => s.signups);

  return (
    <DataTable
      columns={columns}
      data={signups}
      searchPlaceholder="Search signups..."
      emptyTitle="No referred signups yet"
      emptyDescription="People who sign up through your referral link show up here."
      pageSize={10}
    />
  );
}

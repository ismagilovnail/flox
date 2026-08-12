import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Caption } from "@/components/ui/typography";
import { ROLES, type MemberStatus, type Role, type TeamMember } from "@/lib/mock/team";
import { MemberRowActions } from "@/features/team/member-row-actions";

const STATUS_VARIANT: Record<MemberStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  invited: "warning",
  suspended: "secondary",
};

function initials(name: string) {
  return name
    .split(" ")
    .map((p) => p[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
}

export function memberColumns(onRoleChange: (member: TeamMember, role: Role) => void): ColumnDef<typeof dataTableFeatures, TeamMember>[] {
  return [
    {
      accessorKey: "name",
      header: "Member",
      cell: ({ row }) => (
        <div className="flex items-center gap-2.5">
          <Avatar size="sm">
            <AvatarFallback>{initials(row.original.name)}</AvatarFallback>
          </Avatar>
          <div className="flex flex-col">
            <span className="text-sm font-medium text-foreground">{row.original.name}</span>
            <span className="text-xs text-muted-foreground">{row.original.email}</span>
          </div>
        </div>
      ),
    },
    {
      accessorKey: "role",
      header: "Role",
      cell: ({ row }) => {
        const member = row.original;
        return (
          <Select
            value={member.role}
            onValueChange={(role) => onRoleChange(member, role as Role)}
            disabled={member.role === "Owner"}
          >
            <SelectTrigger size="sm" className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ROLES.map((r) => (
                <SelectItem key={r} value={r} disabled={r === "Owner"}>
                  {r}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        );
      },
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ getValue }) => {
        const status = getValue() as MemberStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
      },
    },
    {
      accessorKey: "lastActiveAt",
      header: "Last active",
      cell: ({ getValue }) => {
        const value = getValue() as string | null;
        return <Caption>{value ? formatDistanceToNow(new Date(value), { addSuffix: true }) : "Never"}</Caption>;
      },
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <MemberRowActions member={row.original} />
        </div>
      ),
    },
  ];
}

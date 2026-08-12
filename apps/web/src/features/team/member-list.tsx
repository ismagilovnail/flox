"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useTeamStore } from "@/stores/team";
import { memberColumns } from "@/features/team/member-columns";
import { InviteMemberSheet, type InviteMemberValues } from "@/features/team/invite-member-sheet";

export function MemberList() {
  const members = useTeamStore((s) => s.members);
  const inviteMember = useTeamStore((s) => s.inviteMember);
  const updateRole = useTeamStore((s) => s.updateRole);

  const [inviting, setInviting] = React.useState(false);

  function handleInvite(values: InviteMemberValues) {
    inviteMember(values);
    toast("Invite sent", { description: values.email });
    setInviting(false);
  }

  const columns = React.useMemo(
    () =>
      memberColumns((member, role) => {
        updateRole(member.id, role);
        toast("Role updated", { description: `${member.name} → ${role}` });
      }),
    [updateRole],
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">{members.length} members</p>
        <Button onClick={() => setInviting(true)}>
          <PlusIcon className="size-4" />
          Invite member
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={members}
        searchPlaceholder="Search members..."
        emptyTitle="No members yet"
        emptyDescription="Invite teammates to give them access to this workspace."
        pageSize={10}
      />

      <InviteMemberSheet open={inviting} onOpenChange={setInviting} onSubmit={handleInvite} />
    </div>
  );
}

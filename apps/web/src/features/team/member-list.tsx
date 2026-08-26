"use client";

import * as React from "react";
import { toast } from "sonner";
import { CopyIcon, PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { IconButton } from "@/components/ui/icon-button";
import { Mono } from "@/components/ui/typography";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useInviteMember, useTeamMembers, useUpdateMember } from "@/hooks/use-team";
import { useMe } from "@/hooks/use-auth";
import { memberColumns } from "@/features/team/member-columns";
import { InviteMemberSheet, type InviteMemberValues } from "@/features/team/invite-member-sheet";
import { ApiError } from "@/lib/api/client";
import type { Role } from "@/lib/mock/team";

export function MemberList() {
  const me = useMe();
  const canWrite = !!me.data?.permissions.includes("team.write");
  const membersQuery = useTeamMembers();
  const inviteMember = useInviteMember();
  const updateMember = useUpdateMember();

  const [inviting, setInviting] = React.useState(false);
  const [inviteUrl, setInviteUrl] = React.useState<string | null>(null);

  function handleInvite(values: InviteMemberValues) {
    inviteMember.mutate(values, {
      onSuccess: (result) => {
        setInviting(false);
        setInviteUrl(result.inviteUrl);
      },
      onError: (err) => {
        const message = err instanceof ApiError ? err.message : "Couldn't send the invite";
        toast.error("Couldn't invite member", { description: message });
      },
    });
  }

  function copyInviteUrl() {
    if (!inviteUrl) return;
    navigator.clipboard.writeText(inviteUrl);
    toast("Invite link copied");
  }

  const columns = React.useMemo(
    () =>
      memberColumns(
        (member, role: Role) => {
          updateMember.mutate(
            { id: member.id, input: { role } },
            {
              onSuccess: () => toast("Role updated", { description: `${member.name} → ${role}` }),
              onError: (err) => {
                const message = err instanceof ApiError ? err.message : "Couldn't update role";
                toast.error("Couldn't update role", { description: message });
              },
            },
          );
        },
        canWrite,
      ),
    [updateMember, canWrite],
  );

  if (membersQuery.isLoading) return <LoadingState />;
  if (membersQuery.isError) {
    return (
      <ErrorState
        description={membersQuery.error instanceof ApiError ? membersQuery.error.message : undefined}
        onRetry={() => membersQuery.refetch()}
      />
    );
  }

  const members = membersQuery.data ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">{members.length} members</p>
        {canWrite && (
          <Button onClick={() => setInviting(true)}>
            <PlusIcon className="size-4" />
            Invite member
          </Button>
        )}
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

      <Dialog open={!!inviteUrl} onOpenChange={(open) => !open && setInviteUrl(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Invite link ready</DialogTitle>
            <DialogDescription>
              There&apos;s no email delivery yet — copy this link and send it to your teammate yourself. It expires in
              7 days.
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-2">
            <Mono className="min-w-0 flex-1 truncate text-xs">{inviteUrl}</Mono>
            <IconButton aria-label="Copy invite link" size="icon-sm" onClick={copyInviteUrl}>
              <CopyIcon className="size-3.5" />
            </IconButton>
          </div>
          <DialogFooter>
            <Button onClick={() => setInviteUrl(null)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

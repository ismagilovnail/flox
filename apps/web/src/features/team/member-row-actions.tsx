"use client";

import * as React from "react";
import { toast } from "sonner";
import { MailIcon, MoreHorizontalIcon, PauseIcon, PlayIcon, Trash2Icon } from "lucide-react";

import { IconButton } from "@/components/ui/icon-button";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useTeamStore } from "@/stores/team";
import type { TeamMember } from "@/lib/mock/team";

export function MemberRowActions({ member }: { member: TeamMember }) {
  const setStatus = useTeamStore((s) => s.setStatus);
  const resendInvite = useTeamStore((s) => s.resendInvite);
  const removeMember = useTeamStore((s) => s.removeMember);
  const [confirmRemove, setConfirmRemove] = React.useState(false);

  const isOwner = member.role === "Owner";

  function handleResend() {
    resendInvite(member.id);
    toast("Invite resent", { description: member.email });
  }

  function toggleSuspend() {
    const next = member.status === "suspended" ? "active" : "suspended";
    setStatus(member.id, next);
    toast(next === "suspended" ? "Member suspended" : "Member reactivated", { description: member.name });
  }

  function handleRemove() {
    removeMember(member.id);
    setConfirmRemove(false);
    toast("Member removed", { description: member.name });
  }

  if (isOwner) {
    return <span className="inline-block w-7" aria-hidden />;
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${member.name}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {member.status === "invited" && (
            <DropdownMenuItem onSelect={handleResend}>
              <MailIcon className="size-4" /> Resend invite
            </DropdownMenuItem>
          )}
          {member.status !== "invited" && (
            <DropdownMenuItem onSelect={toggleSuspend}>
              {member.status === "suspended" ? (
                <>
                  <PlayIcon className="size-4" /> Reactivate
                </>
              ) : (
                <>
                  <PauseIcon className="size-4" /> Suspend
                </>
              )}
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setConfirmRemove(true)}>
            <Trash2Icon className="size-4" /> Remove
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={confirmRemove} onOpenChange={setConfirmRemove}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove {member.name}?</DialogTitle>
            <DialogDescription>
              They immediately lose access to this workspace. This can be reversed by inviting them again.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmRemove(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleRemove}>
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

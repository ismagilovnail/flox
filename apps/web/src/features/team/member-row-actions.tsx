"use client";

import * as React from "react";
import { toast } from "sonner";
import { CopyIcon, MailIcon, MoreHorizontalIcon, PauseIcon, PlayIcon, Trash2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";

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
import { Mono } from "@/components/ui/typography";
import { useRemoveMember, useResendInvite, useUpdateMember } from "@/hooks/use-team";
import { ApiError } from "@/lib/api/client";
import type { Membership } from "@/lib/api/team";

export function MemberRowActions({ member }: { member: Membership }) {
  const { t } = useTranslation(["team", "common"]);
  const updateMember = useUpdateMember();
  const resendInvite = useResendInvite();
  const removeMember = useRemoveMember();

  const [confirmRemove, setConfirmRemove] = React.useState(false);
  const [inviteUrl, setInviteUrl] = React.useState<string | null>(null);

  const isOwner = member.role === "Owner";

  function handleResend() {
    resendInvite.mutate(member.id, {
      onSuccess: (result) => setInviteUrl(result.inviteUrl),
      onError: (err) => {
        const message = err instanceof ApiError ? err.message : undefined;
        toast.error(t("actions.resendError"), { description: message });
      },
    });
  }

  function copyInviteUrl() {
    if (!inviteUrl) return;
    navigator.clipboard.writeText(inviteUrl);
    toast(t("members.linkCopied"));
  }

  function toggleSuspend() {
    const next = member.status === "suspended" ? "active" : "suspended";
    updateMember.mutate(
      { id: member.id, input: { status: next } },
      {
        onSuccess: () =>
          toast(next === "suspended" ? t("actions.suspended") : t("actions.reactivated"), { description: member.name }),
        onError: (err) => {
          const message = err instanceof ApiError ? err.message : undefined;
          toast.error(t("actions.statusError"), { description: message });
        },
      },
    );
  }

  function handleRemove() {
    removeMember.mutate(member.id, {
      onSuccess: () => {
        setConfirmRemove(false);
        toast(t("actions.removed"), { description: member.name });
      },
      onError: (err) => {
        setConfirmRemove(false);
        const message = err instanceof ApiError ? err.message : undefined;
        toast.error(t("actions.removeError"), { description: message });
      },
    });
  }

  if (isOwner) {
    return <span className="inline-block w-7" aria-hidden />;
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={t("actions.aria", { name: member.name })} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {member.status === "invited" && (
            <DropdownMenuItem onSelect={handleResend}>
              <MailIcon className="size-4" /> {t("actions.resend")}
            </DropdownMenuItem>
          )}
          {member.status !== "invited" && (
            <DropdownMenuItem onSelect={toggleSuspend}>
              {member.status === "suspended" ? (
                <>
                  <PlayIcon className="size-4" /> {t("actions.reactivate")}
                </>
              ) : (
                <>
                  <PauseIcon className="size-4" /> {t("actions.suspend")}
                </>
              )}
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setConfirmRemove(true)}>
            <Trash2Icon className="size-4" /> {t("actions.remove")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={confirmRemove} onOpenChange={setConfirmRemove}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("actions.removeTitle", { name: member.name })}</DialogTitle>
            <DialogDescription>{t("actions.removeDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmRemove(false)}>
              {t("common:actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleRemove}>
              {t("actions.remove")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!inviteUrl} onOpenChange={(open) => !open && setInviteUrl(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("invite.linkTitle")}</DialogTitle>
            <DialogDescription>{t("invite.resentDescription")}</DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-2">
            <Mono className="min-w-0 flex-1 truncate text-xs">{inviteUrl}</Mono>
            <IconButton aria-label={t("invite.copyAria")} size="icon-sm" onClick={copyInviteUrl}>
              <CopyIcon className="size-3.5" />
            </IconButton>
          </div>
          <DialogFooter>
            <Button onClick={() => setInviteUrl(null)}>{t("invite.done")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

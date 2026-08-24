"use client";

import * as React from "react";
import { toast } from "sonner";
import { CopyIcon, MoreHorizontalIcon, PauseIcon, PencilIcon, PlayIcon, Trash2Icon } from "lucide-react";
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
import { useActivatePostlanding, useArchivePostlanding, useDuplicatePostlanding, usePausePostlanding } from "@/hooks/use-postlandings";
import type { Postlanding } from "@/lib/api/postlanding";

export function PostlandingRowActions({ postlanding, onEdit }: { postlanding: Postlanding; onEdit: () => void }) {
  const { t } = useTranslation(["postlanding", "common"]);
  const pause = usePausePostlanding();
  const activate = useActivatePostlanding();
  const duplicate = useDuplicatePostlanding();
  const archive = useArchivePostlanding();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const action = postlanding.status === "active" ? pause : activate;
    action.mutate(postlanding.id, {
      onSuccess: () =>
        toast(t(postlanding.status === "active" ? "toast.paused" : "toast.resumed"), { description: postlanding.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(postlanding.id, {
      onSuccess: () => toast(t("toast.duplicated"), { description: t("toast.duplicatedSuffix", { name: postlanding.name }) }),
      onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(postlanding.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast(t("toast.archived"), { description: postlanding.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error(t("toast.archiveError"), { description: err.message });
      },
    });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={t("rowActions.actionsAria", { name: postlanding.name })} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> {t("rowActions.edit")}
          </DropdownMenuItem>
          {postlanding.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {postlanding.status === "active" ? (
                <>
                  <PauseIcon className="size-4" /> {t("rowActions.pause")}
                </>
              ) : (
                <>
                  <PlayIcon className="size-4" /> {t("rowActions.resume")}
                </>
              )}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={handleDuplicate}>
            <CopyIcon className="size-4" /> {t("rowActions.duplicate")}
          </DropdownMenuItem>
          {postlanding.status !== "archived" && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onSelect={() => setConfirmArchive(true)}>
                <Trash2Icon className="size-4" /> {t("rowActions.archive")}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={confirmArchive} onOpenChange={setConfirmArchive}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("rowActions.archiveConfirmTitle", { name: postlanding.name })}</DialogTitle>
            <DialogDescription>{t("rowActions.archiveConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmArchive(false)}>
              {t("actions.cancel", { ns: "common" })}
            </Button>
            <Button variant="destructive" onClick={handleArchive} disabled={archive.isPending}>
              {t("rowActions.archive")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

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
import { useActivateLanding, useArchiveLanding, useDuplicateLanding, usePauseLanding } from "@/hooks/use-landings";
import type { Landing } from "@/lib/api/landings";

export function LandingRowActions({ landing, onEdit }: { landing: Landing; onEdit: () => void }) {
  const { t } = useTranslation(["landings", "common"]);
  const pause = usePauseLanding();
  const activate = useActivateLanding();
  const duplicate = useDuplicateLanding();
  const archive = useArchiveLanding();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const action = landing.status === "active" ? pause : activate;
    action.mutate(landing.id, {
      onSuccess: () => toast(t(landing.status === "active" ? "toast.paused" : "toast.resumed"), { description: landing.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(landing.id, {
      onSuccess: () => toast(t("toast.duplicated"), { description: t("toast.duplicatedSuffix", { name: landing.name }) }),
      onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(landing.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast(t("toast.archived"), { description: landing.name });
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
          <IconButton aria-label={t("rowActions.actionsAria", { name: landing.name })} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> {t("rowActions.edit")}
          </DropdownMenuItem>
          {landing.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {landing.status === "active" ? (
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
          {landing.status !== "archived" && (
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
            <DialogTitle>{t("rowActions.archiveConfirmTitle", { name: landing.name })}</DialogTitle>
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

"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { CopyIcon, PauseIcon, PencilIcon, PlayIcon, Trash2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { IconButton } from "@/components/ui/icon-button";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useActivatePwa, useArchivePwa, useDuplicatePwa, usePausePwa } from "@/hooks/use-pwas";
import { usePermission } from "@/hooks/use-auth";
import type { Pwa } from "@/lib/api/pwa";

export function PwaRowActions({
  pwa,
  variant = "icons",
}: {
  pwa: Pwa;
  /** "icons" (default, used in the PWA table row) is tooltipped icon
   * buttons. "inline" is the same actions as labeled buttons — used in
   * the PWA edit page's own card header, where there's room for labels;
   * "Edit" is dropped there since you're already on that page. Same
   * "icons"/"inline" split already used for campaign row actions. */
  variant?: "icons" | "inline";
}) {
  const { t } = useTranslation(["pwa", "common"]);
  const router = useRouter();
  const canWrite = usePermission("content.write");
  const pause = usePausePwa();
  const activate = useActivatePwa();
  const duplicate = useDuplicatePwa();
  const archive = useArchivePwa();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const action = pwa.status === "active" ? pause : activate;
    action.mutate(pwa.id, {
      onSuccess: () => toast(t(pwa.status === "active" ? "toast.paused" : "toast.resumed"), { description: pwa.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(pwa.id, {
      onSuccess: (created) => {
        toast(t("toast.duplicated"), { description: t("toast.duplicatedSuffix", { name: pwa.name }) });
        router.push(`/pwa/${created.id}`);
      },
      onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(pwa.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast(t("toast.archived"), { description: pwa.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error(t("toast.archiveError"), { description: err.message });
      },
    });
  }

  return (
    <>
      {variant === "inline" ? (
        <div className="flex items-center gap-2">
          {canWrite && pwa.status !== "archived" && (
            <Button type="button" variant="outline" size="sm" onClick={togglePause}>
              {pwa.status === "active" ? <PauseIcon className="size-4" /> : <PlayIcon className="size-4" />}
              {pwa.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}
            </Button>
          )}
          {canWrite && (
            <Button type="button" variant="outline" size="sm" onClick={handleDuplicate}>
              <CopyIcon className="size-4" /> {t("rowActions.duplicate")}
            </Button>
          )}
          {canWrite && pwa.status !== "archived" && (
            <Button type="button" variant="destructive" size="sm" onClick={() => setConfirmArchive(true)}>
              <Trash2Icon className="size-4" /> {t("rowActions.archive")}
            </Button>
          )}
        </div>
      ) : (
        <div className="flex items-center justify-end gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("rowActions.edit")} size="icon-sm" asChild>
                <Link href={`/pwa/${pwa.id}`}>
                  <PencilIcon className="size-3.5" />
                </Link>
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("rowActions.edit")}</TooltipContent>
          </Tooltip>

          {canWrite && pwa.status !== "archived" && (
            <Tooltip>
              <TooltipTrigger asChild>
                <IconButton
                  type="button"
                  aria-label={pwa.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}
                  size="icon-sm"
                  onClick={togglePause}
                >
                  {pwa.status === "active" ? <PauseIcon className="size-3.5" /> : <PlayIcon className="size-3.5" />}
                </IconButton>
              </TooltipTrigger>
              <TooltipContent>{pwa.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}</TooltipContent>
            </Tooltip>
          )}

          {canWrite && (
            <Tooltip>
              <TooltipTrigger asChild>
                <IconButton type="button" aria-label={t("rowActions.duplicate")} size="icon-sm" onClick={handleDuplicate}>
                  <CopyIcon className="size-3.5" />
                </IconButton>
              </TooltipTrigger>
              <TooltipContent>{t("rowActions.duplicate")}</TooltipContent>
            </Tooltip>
          )}

          {canWrite && pwa.status !== "archived" && (
            <Tooltip>
              <TooltipTrigger asChild>
                <IconButton type="button" aria-label={t("rowActions.archive")} size="icon-sm" onClick={() => setConfirmArchive(true)}>
                  <Trash2Icon className="size-3.5" />
                </IconButton>
              </TooltipTrigger>
              <TooltipContent>{t("rowActions.archive")}</TooltipContent>
            </Tooltip>
          )}
        </div>
      )}

      <Dialog open={confirmArchive} onOpenChange={setConfirmArchive}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("rowActions.archiveConfirmTitle", { name: pwa.name })}</DialogTitle>
            <DialogDescription>{t("rowActions.archiveConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setConfirmArchive(false)}>
              {t("actions.cancel", { ns: "common" })}
            </Button>
            <Button type="button" variant="destructive" onClick={handleArchive} disabled={archive.isPending}>
              {t("rowActions.archive")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

"use client";

import * as React from "react";
import Link from "next/link";
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
import { useActivatePixel, useArchivePixel, useDuplicatePixel, usePausePixel } from "@/hooks/use-pixels";
import { usePermission } from "@/hooks/use-auth";
import type { Pixel } from "@/lib/api/pixels";

export function PixelRowActions({ pixel }: { pixel: Pixel }) {
  const { t } = useTranslation(["pixels", "common"]);
  const canWrite = usePermission("content.write");
  const pause = usePausePixel();
  const activate = useActivatePixel();
  const duplicate = useDuplicatePixel();
  const archive = useArchivePixel();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const action = pixel.status === "active" ? pause : activate;
    action.mutate(pixel.id, {
      onSuccess: () => toast(t(pixel.status === "active" ? "toast.paused" : "toast.resumed"), { description: pixel.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(pixel.id, {
      onSuccess: () => toast(t("toast.duplicated"), { description: t("toast.duplicatedSuffix", { name: pixel.name }) }),
      onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(pixel.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast(t("toast.archived"), { description: pixel.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error(t("toast.archiveError"), { description: err.message });
      },
    });
  }

  return (
    <>
      <div className="flex items-center justify-end gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <IconButton aria-label={t("rowActions.edit")} size="icon-sm" asChild>
              <Link href={`/pixels/${pixel.id}`}>
                <PencilIcon className="size-3.5" />
              </Link>
            </IconButton>
          </TooltipTrigger>
          <TooltipContent>{t("rowActions.edit")}</TooltipContent>
        </Tooltip>

        {canWrite && pixel.status !== "archived" && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton
                aria-label={pixel.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}
                size="icon-sm"
                onClick={togglePause}
              >
                {pixel.status === "active" ? <PauseIcon className="size-3.5" /> : <PlayIcon className="size-3.5" />}
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{pixel.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}</TooltipContent>
          </Tooltip>
        )}

        {canWrite && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("rowActions.duplicate")} size="icon-sm" onClick={handleDuplicate}>
                <CopyIcon className="size-3.5" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("rowActions.duplicate")}</TooltipContent>
          </Tooltip>
        )}

        {canWrite && pixel.status !== "archived" && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("rowActions.archive")} size="icon-sm" onClick={() => setConfirmArchive(true)}>
                <Trash2Icon className="size-3.5" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("rowActions.archive")}</TooltipContent>
          </Tooltip>
        )}
      </div>

      <Dialog open={confirmArchive} onOpenChange={setConfirmArchive}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("rowActions.archiveConfirmTitle", { name: pixel.name })}</DialogTitle>
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

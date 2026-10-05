"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { BarChart3Icon, CodeIcon, CopyIcon, PauseIcon, PencilIcon, PlayIcon, Trash2Icon } from "lucide-react";
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
import { useActivateLanding, useArchiveLanding, useDuplicateLanding, usePauseLanding } from "@/hooks/use-landings";
import { usePermission } from "@/hooks/use-auth";
import type { Landing } from "@/lib/api/landings";
import { LandingTemplateEditor } from "@/features/landings/landing-template-editor";
import { viewStatisticsHref } from "@/features/analytics/view-statistics-link";

export function LandingRowActions({ landing }: { landing: Landing }) {
  const { t } = useTranslation(["landings", "common"]);
  const canWrite = usePermission("content.write");
  const pause = usePauseLanding();
  const activate = useActivateLanding();
  const duplicate = useDuplicateLanding();
  const archive = useArchiveLanding();
  const [confirmArchive, setConfirmArchive] = React.useState(false);
  const [editingTemplate, setEditingTemplate] = React.useState(false);

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
      <div className="flex items-center justify-end gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <IconButton aria-label={t("rowActions.edit")} size="icon-sm" asChild>
              <Link href={`/landings/${landing.id}`}>
                <PencilIcon className="size-3.5" />
              </Link>
            </IconButton>
          </TooltipTrigger>
          <TooltipContent>{t("rowActions.edit")}</TooltipContent>
        </Tooltip>

        {/* landing.id, not .name: the `landing` report dimension groups
            by landing_id. */}
        <Tooltip>
          <TooltipTrigger asChild>
            <IconButton aria-label={t("rowActions.viewStatistics")} size="icon-sm" asChild>
              <Link href={viewStatisticsHref("landing", landing.id)}>
                <BarChart3Icon className="size-3.5" />
              </Link>
            </IconButton>
          </TooltipTrigger>
          <TooltipContent>{t("rowActions.viewStatistics")}</TooltipContent>
        </Tooltip>

        {canWrite && landing.type === "internal" && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("rowActions.editTemplate")} size="icon-sm" onClick={() => setEditingTemplate(true)}>
                <CodeIcon className="size-3.5" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("rowActions.editTemplate")}</TooltipContent>
          </Tooltip>
        )}

        {canWrite && landing.status !== "archived" && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton
                aria-label={landing.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}
                size="icon-sm"
                onClick={togglePause}
              >
                {landing.status === "active" ? <PauseIcon className="size-3.5" /> : <PlayIcon className="size-3.5" />}
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{landing.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}</TooltipContent>
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

        {canWrite && landing.status !== "archived" && (
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

      {editingTemplate && (
        <LandingTemplateEditor landing={landing} open={editingTemplate} onOpenChange={setEditingTemplate} />
      )}

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

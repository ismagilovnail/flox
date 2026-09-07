"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { BarChart3Icon, CopyIcon, PauseIcon, PencilIcon, PlayIcon, Trash2Icon } from "lucide-react";
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
import {
  useActivateTrafficSource,
  useArchiveTrafficSource,
  useDuplicateTrafficSource,
  usePauseTrafficSource,
} from "@/hooks/use-traffic-sources";
import { usePermission } from "@/hooks/use-auth";
import { viewStatisticsHref } from "@/features/analytics/view-statistics-link";
import type { TrafficSource } from "@/lib/api/traffic-sources";

export function SourceRowActions({ source }: { source: TrafficSource }) {
  const { t } = useTranslation(["trafficSources", "common"]);
  const canWrite = usePermission("source.write");
  const pause = usePauseTrafficSource();
  const activate = useActivateTrafficSource();
  const duplicate = useDuplicateTrafficSource();
  const archive = useArchiveTrafficSource();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const action = source.status === "active" ? pause : activate;
    action.mutate(source.id, {
      onSuccess: () => toast(t(source.status === "active" ? "toast.paused" : "toast.resumed"), { description: source.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(source.id, {
      onSuccess: () => toast(t("toast.duplicated"), { description: t("toast.duplicatedSuffix", { name: source.name }) }),
      onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(source.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast(t("toast.archived"), { description: source.name });
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
              <Link href={`/traffic-sources/${source.id}`}>
                <PencilIcon className="size-3.5" />
              </Link>
            </IconButton>
          </TooltipTrigger>
          <TooltipContent>{t("rowActions.edit")}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <IconButton aria-label={t("rowActions.viewStatistics")} size="icon-sm" asChild>
              <Link href={viewStatisticsHref("source", source.name)}>
                <BarChart3Icon className="size-3.5" />
              </Link>
            </IconButton>
          </TooltipTrigger>
          <TooltipContent>{t("rowActions.viewStatistics")}</TooltipContent>
        </Tooltip>

        {canWrite && source.status !== "archived" && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton
                aria-label={source.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}
                size="icon-sm"
                onClick={togglePause}
              >
                {source.status === "active" ? <PauseIcon className="size-3.5" /> : <PlayIcon className="size-3.5" />}
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{source.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}</TooltipContent>
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

        {canWrite && source.status !== "archived" && (
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
            <DialogTitle>{t("rowActions.archiveConfirmTitle", { name: source.name })}</DialogTitle>
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

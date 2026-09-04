"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { CopyIcon, ExternalLinkIcon, PauseIcon, PlayIcon, Trash2Icon } from "lucide-react";
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
import { useActivateCampaign, useArchiveCampaign, useDuplicateCampaign, usePauseCampaign } from "@/hooks/use-campaigns";
import type { Campaign } from "@/lib/api/campaigns";

export function CampaignRowActions({
  campaign,
  variant = "icons",
}: {
  campaign: Campaign;
  /** "icons" (default, used in the campaigns table row) is tooltipped
   * icon buttons — matches the stream-set row action pattern and fits a
   * table row's width budget. "inline" is the same actions as labeled
   * buttons — used on the campaign detail page header, where there's room
   * for labels; "Open" is dropped there since you're already on that
   * page. Both surface every action directly, no "..." menu — dropped at
   * the user's request, same call already made for stream-set rows and
   * tracking links. */
  variant?: "icons" | "inline";
}) {
  const { t } = useTranslation(["campaigns", "common"]);
  const router = useRouter();
  const pause = usePauseCampaign();
  const activate = useActivateCampaign();
  const duplicate = useDuplicateCampaign();
  const archive = useArchiveCampaign();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const action = campaign.status === "active" ? pause : activate;
    const toastKey =
      campaign.status === "active" ? "toast.paused" : campaign.status === "draft" ? "toast.activated" : "toast.resumed";
    action.mutate(campaign.id, {
      onSuccess: () => toast(t(toastKey), { description: campaign.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(campaign.id, {
      onSuccess: (created) => {
        toast(t("toast.duplicated"), { description: t("toast.duplicatedSuffix", { name: campaign.name }) });
        router.push(`/campaigns/${created.id}`);
      },
      onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(campaign.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast(t("toast.archived"), { description: campaign.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error(t("toast.archiveError"), { description: err.message });
      },
    });
  }

  const pauseLabel = t(
    campaign.status === "active" ? "rowActions.pause" : campaign.status === "draft" ? "rowActions.activate" : "rowActions.resume",
  );

  return (
    <>
      {variant === "inline" ? (
        <div className="flex items-center gap-2">
          {campaign.status !== "archived" && (
            <Button variant="outline" size="sm" onClick={togglePause}>
              {campaign.status === "active" ? <PauseIcon className="size-4" /> : <PlayIcon className="size-4" />}
              {pauseLabel}
            </Button>
          )}
          <Button variant="outline" size="sm" onClick={handleDuplicate}>
            <CopyIcon className="size-4" /> {t("rowActions.duplicate")}
          </Button>
          {campaign.status !== "archived" && (
            <Button variant="destructive" size="sm" onClick={() => setConfirmArchive(true)}>
              <Trash2Icon className="size-4" /> {t("rowActions.archive")}
            </Button>
          )}
        </div>
      ) : (
        <div className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("rowActions.open")} size="icon-sm" asChild>
                <Link href={`/campaigns/${campaign.id}`}>
                  <ExternalLinkIcon className="size-3.5" />
                </Link>
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("rowActions.open")}</TooltipContent>
          </Tooltip>

          {campaign.status !== "archived" && (
            <Tooltip>
              <TooltipTrigger asChild>
                <IconButton aria-label={pauseLabel} size="icon-sm" onClick={togglePause}>
                  {campaign.status === "active" ? <PauseIcon className="size-3.5" /> : <PlayIcon className="size-3.5" />}
                </IconButton>
              </TooltipTrigger>
              <TooltipContent>{pauseLabel}</TooltipContent>
            </Tooltip>
          )}

          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("rowActions.duplicate")} size="icon-sm" onClick={handleDuplicate}>
                <CopyIcon className="size-3.5" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("rowActions.duplicate")}</TooltipContent>
          </Tooltip>

          {campaign.status !== "archived" && (
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
      )}

      <Dialog open={confirmArchive} onOpenChange={setConfirmArchive}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("rowActions.archiveConfirmTitle", { name: campaign.name })}</DialogTitle>
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

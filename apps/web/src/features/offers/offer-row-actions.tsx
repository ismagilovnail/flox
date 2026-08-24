"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { BarChart3Icon, CopyIcon, MoreHorizontalIcon, PauseIcon, PencilIcon, PlayIcon, Trash2Icon } from "lucide-react";
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
import { useActivateOffer, useArchiveOffer, useDuplicateOffer, usePauseOffer } from "@/hooks/use-offers";
import { viewStatisticsHref } from "@/features/analytics/view-statistics-link";
import type { Offer } from "@/lib/api/offers";

export function OfferRowActions({ offer, onEdit }: { offer: Offer; onEdit: () => void }) {
  const { t } = useTranslation(["offers", "common"]);
  const pause = usePauseOffer();
  const activate = useActivateOffer();
  const duplicate = useDuplicateOffer();
  const archive = useArchiveOffer();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const action = offer.status === "active" ? pause : activate;
    action.mutate(offer.id, {
      onSuccess: () => toast(t(offer.status === "active" ? "toast.paused" : "toast.resumed"), { description: offer.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(offer.id, {
      onSuccess: () => toast(t("toast.duplicated"), { description: t("toast.duplicatedSuffix", { name: offer.name }) }),
      onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(offer.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast(t("toast.archived"), { description: offer.name });
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
          <IconButton aria-label={t("rowActions.actionsAria", { name: offer.name })} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> {t("rowActions.edit")}
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link href={viewStatisticsHref("offer", offer.name)}>
              <BarChart3Icon className="size-4" /> {t("rowActions.viewStatistics")}
            </Link>
          </DropdownMenuItem>
          {offer.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {offer.status === "active" ? (
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
          {offer.status !== "archived" && (
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
            <DialogTitle>{t("rowActions.archiveConfirmTitle", { name: offer.name })}</DialogTitle>
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

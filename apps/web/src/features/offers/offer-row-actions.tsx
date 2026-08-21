"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { BarChart3Icon, CopyIcon, MoreHorizontalIcon, PauseIcon, PencilIcon, PlayIcon, Trash2Icon } from "lucide-react";

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
  const pause = usePauseOffer();
  const activate = useActivateOffer();
  const duplicate = useDuplicateOffer();
  const archive = useArchiveOffer();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const action = offer.status === "active" ? pause : activate;
    action.mutate(offer.id, {
      onSuccess: () => toast(offer.status === "active" ? "Offer paused" : "Offer resumed", { description: offer.name }),
      onError: (err) => toast.error("Couldn't update offer", { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(offer.id, {
      onSuccess: () => toast("Offer duplicated", { description: `${offer.name} (Copy)` }),
      onError: (err) => toast.error("Couldn't duplicate offer", { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(offer.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast("Offer archived", { description: offer.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error("Couldn't archive offer", { description: err.message });
      },
    });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${offer.name}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> Edit
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link href={viewStatisticsHref("offer", offer.name)}>
              <BarChart3Icon className="size-4" /> View statistics
            </Link>
          </DropdownMenuItem>
          {offer.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {offer.status === "active" ? (
                <>
                  <PauseIcon className="size-4" /> Pause
                </>
              ) : (
                <>
                  <PlayIcon className="size-4" /> Resume
                </>
              )}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={handleDuplicate}>
            <CopyIcon className="size-4" /> Duplicate
          </DropdownMenuItem>
          {offer.status !== "archived" && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onSelect={() => setConfirmArchive(true)}>
                <Trash2Icon className="size-4" /> Archive
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={confirmArchive} onOpenChange={setConfirmArchive}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Archive &ldquo;{offer.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              Archived offers are hidden from flow pickers going forward. Existing flows that already reference
              this offer keep working. This can be reversed later.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmArchive(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleArchive} disabled={archive.isPending}>
              Archive
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

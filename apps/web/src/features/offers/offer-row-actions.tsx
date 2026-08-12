"use client";

import * as React from "react";
import { toast } from "sonner";
import { CopyIcon, MoreHorizontalIcon, PauseIcon, PencilIcon, PlayIcon, Trash2Icon } from "lucide-react";

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
import { useOffersStore } from "@/stores/offers";
import type { Offer } from "@/lib/mock/offers";

export function OfferRowActions({ offer, onEdit }: { offer: Offer; onEdit: () => void }) {
  const setStatus = useOffersStore((s) => s.setStatus);
  const duplicateOffer = useOffersStore((s) => s.duplicateOffer);
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const next = offer.status === "active" ? "paused" : "active";
    setStatus(offer.id, next);
    toast(next === "paused" ? "Offer paused" : "Offer resumed", { description: offer.name });
  }

  function duplicate() {
    duplicateOffer(offer.id);
    toast("Offer duplicated", { description: `${offer.name} (Copy)` });
  }

  function archive() {
    setStatus(offer.id, "archived");
    setConfirmArchive(false);
    toast("Offer archived", { description: offer.name });
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
          <DropdownMenuItem onSelect={duplicate}>
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
            <Button variant="destructive" onClick={archive}>
              Archive
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

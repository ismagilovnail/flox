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
import { usePixelsStore } from "@/stores/pixels";
import type { Pixel } from "@/lib/mock/pixels";

export function PixelRowActions({ pixel, onEdit }: { pixel: Pixel; onEdit: () => void }) {
  const setStatus = usePixelsStore((s) => s.setStatus);
  const duplicatePixel = usePixelsStore((s) => s.duplicatePixel);
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const next = pixel.status === "active" ? "paused" : "active";
    setStatus(pixel.id, next);
    toast(next === "paused" ? "Pixel paused" : "Pixel resumed", { description: pixel.name });
  }

  function duplicate() {
    duplicatePixel(pixel.id);
    toast("Pixel duplicated", { description: `${pixel.name} (Copy)` });
  }

  function archive() {
    setStatus(pixel.id, "archived");
    setConfirmArchive(false);
    toast("Pixel archived", { description: pixel.name });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${pixel.name}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> Edit
          </DropdownMenuItem>
          {pixel.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {pixel.status === "active" ? (
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
          {pixel.status !== "archived" && (
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
            <DialogTitle>Archive &ldquo;{pixel.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              Archived pixels stop firing on new events. This can be reversed later.
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

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
import { usePwasStore } from "@/stores/pwas";
import type { Pwa } from "@/lib/mock/pwas";

export function PwaRowActions({ pwa, onEdit }: { pwa: Pwa; onEdit: () => void }) {
  const setStatus = usePwasStore((s) => s.setStatus);
  const duplicatePwa = usePwasStore((s) => s.duplicatePwa);
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const next = pwa.status === "active" ? "paused" : "active";
    setStatus(pwa.id, next);
    toast(next === "paused" ? "PWA paused" : "PWA resumed", { description: pwa.name });
  }

  function duplicate() {
    duplicatePwa(pwa.id);
    toast("PWA duplicated", { description: `${pwa.name} (Copy)` });
  }

  function archive() {
    setStatus(pwa.id, "archived");
    setConfirmArchive(false);
    toast("PWA archived", { description: pwa.name });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${pwa.name}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> Edit
          </DropdownMenuItem>
          {pwa.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {pwa.status === "active" ? (
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
          {pwa.status !== "archived" && (
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
            <DialogTitle>Archive &ldquo;{pwa.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              Archived PWAs are hidden from flow pickers going forward. Existing flows that already reference this
              PWA keep working. This can be reversed later.
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

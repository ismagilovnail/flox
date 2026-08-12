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
import { usePostlandingsStore } from "@/stores/postlandings";
import type { Postlanding } from "@/lib/mock/postlandings";

export function PostlandingRowActions({ postlanding, onEdit }: { postlanding: Postlanding; onEdit: () => void }) {
  const setStatus = usePostlandingsStore((s) => s.setStatus);
  const duplicatePostlanding = usePostlandingsStore((s) => s.duplicatePostlanding);
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const next = postlanding.status === "active" ? "paused" : "active";
    setStatus(postlanding.id, next);
    toast(next === "paused" ? "Postlanding paused" : "Postlanding resumed", { description: postlanding.name });
  }

  function duplicate() {
    duplicatePostlanding(postlanding.id);
    toast("Postlanding duplicated", { description: `${postlanding.name} (Copy)` });
  }

  function archive() {
    setStatus(postlanding.id, "archived");
    setConfirmArchive(false);
    toast("Postlanding archived", { description: postlanding.name });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${postlanding.name}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> Edit
          </DropdownMenuItem>
          {postlanding.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {postlanding.status === "active" ? (
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
          {postlanding.status !== "archived" && (
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
            <DialogTitle>Archive &ldquo;{postlanding.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              Archived postlandings are hidden from flow pickers going forward. Existing flows that already
              reference this postlanding keep working. This can be reversed later.
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

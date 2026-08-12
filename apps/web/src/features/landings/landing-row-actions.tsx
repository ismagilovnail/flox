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
import { useLandingsStore } from "@/stores/landings";
import type { Landing } from "@/lib/mock/landings";

export function LandingRowActions({ landing, onEdit }: { landing: Landing; onEdit: () => void }) {
  const setStatus = useLandingsStore((s) => s.setStatus);
  const duplicateLanding = useLandingsStore((s) => s.duplicateLanding);
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const next = landing.status === "active" ? "paused" : "active";
    setStatus(landing.id, next);
    toast(next === "paused" ? "Landing paused" : "Landing resumed", { description: landing.name });
  }

  function duplicate() {
    duplicateLanding(landing.id);
    toast("Landing duplicated", { description: `${landing.name} (Copy)` });
  }

  function archive() {
    setStatus(landing.id, "archived");
    setConfirmArchive(false);
    toast("Landing archived", { description: landing.name });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${landing.name}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> Edit
          </DropdownMenuItem>
          {landing.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {landing.status === "active" ? (
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
          {landing.status !== "archived" && (
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
            <DialogTitle>Archive &ldquo;{landing.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              Archived landings are hidden from flow pickers going forward. Existing flows that already reference
              this landing keep working. This can be reversed later.
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

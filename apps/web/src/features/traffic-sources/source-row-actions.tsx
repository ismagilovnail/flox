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
import { useTrafficSourcesStore } from "@/stores/traffic-sources";
import { viewStatisticsHref } from "@/features/analytics/view-statistics-link";
import type { TrafficSource } from "@/lib/mock/traffic-sources";

export function SourceRowActions({ source, onEdit }: { source: TrafficSource; onEdit: () => void }) {
  const setStatus = useTrafficSourcesStore((s) => s.setStatus);
  const duplicateSource = useTrafficSourcesStore((s) => s.duplicateSource);
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const next = source.status === "active" ? "paused" : "active";
    setStatus(source.id, next);
    toast(next === "paused" ? "Source paused" : "Source resumed", { description: source.name });
  }

  function duplicate() {
    duplicateSource(source.id);
    toast("Source duplicated", { description: `${source.name} (Copy)` });
  }

  function archive() {
    setStatus(source.id, "archived");
    setConfirmArchive(false);
    toast("Source archived", { description: source.name });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${source.name}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> Edit
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link href={viewStatisticsHref("source", source.name)}>
              <BarChart3Icon className="size-4" /> View statistics
            </Link>
          </DropdownMenuItem>
          {source.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {source.status === "active" ? (
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
          {source.status !== "archived" && (
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
            <DialogTitle>Archive &ldquo;{source.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              Archived sources are hidden from campaign source pickers going forward. This can be reversed later.
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

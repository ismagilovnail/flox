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
import { useActivateNetwork, useArchiveNetwork, useDuplicateNetwork, usePauseNetwork } from "@/hooks/use-networks";
import { viewStatisticsHref } from "@/features/analytics/view-statistics-link";
import type { Network } from "@/lib/api/networks";

export function NetworkRowActions({ network, onEdit }: { network: Network; onEdit: () => void }) {
  const pause = usePauseNetwork();
  const activate = useActivateNetwork();
  const duplicate = useDuplicateNetwork();
  const archive = useArchiveNetwork();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const action = network.status === "active" ? pause : activate;
    action.mutate(network.id, {
      onSuccess: () => toast(network.status === "active" ? "Network paused" : "Network resumed", { description: network.name }),
      onError: (err) => toast.error("Couldn't update network", { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(network.id, {
      onSuccess: () => toast("Network duplicated", { description: `${network.name} (Copy)` }),
      onError: (err) => toast.error("Couldn't duplicate network", { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(network.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast("Network archived", { description: network.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error("Couldn't archive network", { description: err.message });
      },
    });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${network.name}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> Edit
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link href={viewStatisticsHref("network", network.name)}>
              <BarChart3Icon className="size-4" /> View statistics
            </Link>
          </DropdownMenuItem>
          {network.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {network.status === "active" ? (
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
          {network.status !== "archived" && (
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
            <DialogTitle>Archive &ldquo;{network.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              Archived networks are hidden from offer/flow pickers going forward. Existing flows that already
              reference this network keep working. This can be reversed later.
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

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
import { useNetworksStore } from "@/stores/networks";
import { viewStatisticsHref } from "@/features/analytics/view-statistics-link";
import type { Network } from "@/lib/mock/networks";

export function NetworkRowActions({ network, onEdit }: { network: Network; onEdit: () => void }) {
  const setStatus = useNetworksStore((s) => s.setStatus);
  const duplicateNetwork = useNetworksStore((s) => s.duplicateNetwork);
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const next = network.status === "active" ? "paused" : "active";
    setStatus(network.id, next);
    toast(next === "paused" ? "Network paused" : "Network resumed", { description: network.name });
  }

  function duplicate() {
    duplicateNetwork(network.id);
    toast("Network duplicated", { description: `${network.name} (Copy)` });
  }

  function archive() {
    setStatus(network.id, "archived");
    setConfirmArchive(false);
    toast("Network archived", { description: network.name });
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
          <DropdownMenuItem onSelect={duplicate}>
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
            <Button variant="destructive" onClick={archive}>
              Archive
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

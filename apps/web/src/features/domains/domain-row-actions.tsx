"use client";

import * as React from "react";
import { toast } from "sonner";
import { MoreHorizontalIcon, PencilIcon, Trash2Icon } from "lucide-react";

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
import { useDeleteDomain } from "@/hooks/use-domains";
import type { Domain } from "@/lib/api/domains";

export function DomainRowActions({ domain, onEdit }: { domain: Domain; onEdit: () => void }) {
  const deleteDomain = useDeleteDomain();
  const [confirmRemove, setConfirmRemove] = React.useState(false);

  function handleRemove() {
    deleteDomain.mutate(domain.id, {
      onSuccess: () => {
        setConfirmRemove(false);
        toast("Domain removed", { description: domain.domain });
      },
      onError: (err) => {
        setConfirmRemove(false);
        toast.error("Couldn't remove domain", { description: err.message });
      },
    });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${domain.domain}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> Edit
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setConfirmRemove(true)}>
            <Trash2Icon className="size-4" /> Remove
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={confirmRemove} onOpenChange={setConfirmRemove}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove &ldquo;{domain.domain}&rdquo;?</DialogTitle>
            <DialogDescription>
              Any tracking links on this domain are removed with it, and campaigns using them stop resolving.
              This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmRemove(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleRemove} disabled={deleteDomain.isPending}>
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

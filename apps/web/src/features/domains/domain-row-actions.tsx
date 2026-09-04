"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { PencilIcon, Trash2Icon } from "lucide-react";

import { IconButton } from "@/components/ui/icon-button";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
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

export function DomainRowActions({ domain }: { domain: Domain }) {
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
      <div className="flex items-center justify-end gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <IconButton aria-label="Edit" size="icon-sm" asChild>
              <Link href={`/domains/${domain.id}`}>
                <PencilIcon className="size-3.5" />
              </Link>
            </IconButton>
          </TooltipTrigger>
          <TooltipContent>Edit</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <IconButton aria-label="Remove" size="icon-sm" onClick={() => setConfirmRemove(true)}>
              <Trash2Icon className="size-3.5" />
            </IconButton>
          </TooltipTrigger>
          <TooltipContent>Remove</TooltipContent>
        </Tooltip>
      </div>

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

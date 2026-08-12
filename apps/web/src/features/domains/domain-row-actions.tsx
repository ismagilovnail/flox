"use client";

import * as React from "react";
import { toast } from "sonner";
import { BadgeCheckIcon, LockIcon, MoreHorizontalIcon, PencilIcon, Trash2Icon } from "lucide-react";

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
import { useDomainsStore } from "@/stores/domains";
import type { Domain } from "@/lib/mock/domains";

export function DomainRowActions({ domain, onEdit }: { domain: Domain; onEdit: () => void }) {
  const issueSsl = useDomainsStore((s) => s.issueSsl);
  const verify = useDomainsStore((s) => s.verify);
  const removeDomain = useDomainsStore((s) => s.removeDomain);
  const [confirmRemove, setConfirmRemove] = React.useState(false);

  function handleVerify() {
    verify(domain.id);
    toast("Domain verified", { description: domain.domain });
  }

  function handleIssueSsl() {
    issueSsl(domain.id);
    toast("SSL certificate issued", { description: domain.domain });
  }

  function handleRemove() {
    removeDomain(domain.id);
    setConfirmRemove(false);
    toast("Domain removed", { description: domain.domain });
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
          {!domain.verifiedAt && (
            <DropdownMenuItem onSelect={handleVerify}>
              <BadgeCheckIcon className="size-4" /> Verify ownership
            </DropdownMenuItem>
          )}
          {domain.ssl !== "issued" && (
            <DropdownMenuItem onSelect={handleIssueSsl}>
              <LockIcon className="size-4" /> Issue SSL
            </DropdownMenuItem>
          )}
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
              This detaches the domain from FLOX — DNS records and SSL certificates managed here stop being
              renewed. Campaigns still pointing at it will break. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmRemove(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleRemove}>
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

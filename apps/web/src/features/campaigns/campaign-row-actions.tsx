"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { CopyIcon, ExternalLinkIcon, MoreHorizontalIcon, PauseIcon, PlayIcon, Trash2Icon } from "lucide-react";

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
import { useActivateCampaign, useArchiveCampaign, useDuplicateCampaign, usePauseCampaign } from "@/hooks/use-campaigns";
import type { Campaign } from "@/lib/api/campaigns";

export function CampaignRowActions({ campaign }: { campaign: Campaign }) {
  const router = useRouter();
  const pause = usePauseCampaign();
  const activate = useActivateCampaign();
  const duplicate = useDuplicateCampaign();
  const archive = useArchiveCampaign();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  function togglePause() {
    const action = campaign.status === "active" ? pause : activate;
    action.mutate(campaign.id, {
      onSuccess: () =>
        toast(campaign.status === "active" ? "Campaign paused" : "Campaign resumed", { description: campaign.name }),
      onError: (err) => toast.error("Couldn't update campaign", { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(campaign.id, {
      onSuccess: (created) => {
        toast("Campaign duplicated", { description: `${campaign.name} (Copy)` });
        router.push(`/campaigns/${created.id}`);
      },
      onError: (err) => toast.error("Couldn't duplicate campaign", { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(campaign.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast("Campaign archived", { description: campaign.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error("Couldn't archive campaign", { description: err.message });
      },
    });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${campaign.name}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem asChild>
            <Link href={`/campaigns/${campaign.id}`}>
              <ExternalLinkIcon className="size-4" /> Open
            </Link>
          </DropdownMenuItem>
          {campaign.status !== "archived" && campaign.status !== "draft" && (
            <DropdownMenuItem onSelect={togglePause}>
              {campaign.status === "active" ? (
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
          {campaign.status !== "archived" && (
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
            <DialogTitle>Archive &ldquo;{campaign.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              Archived campaigns stop routing traffic and are hidden from the active list. This
              can be reversed later from campaign settings.
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

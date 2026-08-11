"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import {
  CopyIcon,
  ExternalLinkIcon,
  MoreHorizontalIcon,
  PauseIcon,
  PlayIcon,
  Trash2Icon,
} from "lucide-react";

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
import { useCampaignsStore } from "@/stores/campaigns";
import type { Campaign } from "@/lib/mock/campaigns";

export function CampaignRowActions({ campaign }: { campaign: Campaign }) {
  const router = useRouter();
  const setStatus = useCampaignsStore((s) => s.setStatus);
  const duplicateCampaign = useCampaignsStore((s) => s.duplicateCampaign);
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  const trackingUrl = `https://${campaign.trackingDomain}/t/${campaign.trackingId}`;

  function copyTrackingUrl() {
    navigator.clipboard.writeText(trackingUrl);
    toast("Tracking URL copied", { description: trackingUrl });
  }

  function togglePause() {
    const next = campaign.status === "active" ? "paused" : "active";
    setStatus(campaign.id, next);
    toast(next === "paused" ? "Campaign paused" : "Campaign resumed", {
      description: campaign.name,
    });
  }

  function duplicate() {
    const id = duplicateCampaign(campaign.id);
    toast("Campaign duplicated", { description: `${campaign.name} (Copy)` });
    if (id) router.push(`/campaigns/${id}`);
  }

  function archive() {
    setStatus(campaign.id, "archived");
    setConfirmArchive(false);
    toast("Campaign archived", { description: campaign.name });
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
          <DropdownMenuItem onSelect={duplicate}>
            <CopyIcon className="size-4" /> Duplicate
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={copyTrackingUrl}>
            <CopyIcon className="size-4" /> Copy tracking URL
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
            <Button variant="destructive" onClick={archive}>
              Archive
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

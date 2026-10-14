"use client";

import { toast } from "sonner";
import { LinkIcon, LockOpenIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { IconButton } from "@/components/ui/icon-button";
import { Mono } from "@/components/ui/typography";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useActiveTrackingLinksByCampaign } from "@/hooks/use-tracking-links";
import type { TrackingLink } from "@/lib/api/tracking-links";

/** "Copy tracking URL" in a campaigns table row (§20): one active link is
 * copied in one click, several open a menu of them, none leaves the button
 * disabled with a hint. Reads the one org-wide query every row shares. */
export function CopyTrackingUrlButton({ campaignId, campaignName }: { campaignId: string; campaignName: string }) {
  const { t } = useTranslation("campaigns");
  const query = useActiveTrackingLinksByCampaign();
  const links = query.data?.get(campaignId) ?? [];
  const label = t("rowActions.copyTrackingUrl");

  function copy(link: TrackingLink) {
    navigator.clipboard.writeText(link.url).then(
      () => toast(t("trackingLinks.copiedToast"), { description: link.url }),
      () => toast.error(t("rowActions.copyTrackingUrlError"), { description: link.url }),
    );
  }

  if (links.length === 0) {
    const hint = query.isPending ? label : query.isError ? t("rowActions.copyTrackingUrlLoadError") : t("rowActions.copyTrackingUrlNone");
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          {/* A disabled button gets no pointer events; the span keeps the hint. */}
          <span tabIndex={0} aria-label={hint}>
            <IconButton aria-label={label} size="icon-sm" disabled>
              <LinkIcon className="size-3.5" />
            </IconButton>
          </span>
        </TooltipTrigger>
        <TooltipContent className="max-w-64">{hint}</TooltipContent>
      </Tooltip>
    );
  }

  if (links.length === 1) {
    const link = links[0];
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <IconButton aria-label={t("rowActions.copyTrackingUrlAria", { name: campaignName })} size="icon-sm" onClick={() => copy(link)}>
            <LinkIcon className="size-3.5" />
          </IconButton>
        </TooltipTrigger>
        <TooltipContent className="max-w-96">
          <div className="flex flex-col gap-0.5">
            <span>{label}</span>
            <Mono className="text-xs break-all">{link.url}</Mono>
            {!link.secure && <span className="text-warning">{t("rowActions.copyTrackingUrlInsecure")}</span>}
          </div>
        </TooltipContent>
      </Tooltip>
    );
  }

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <IconButton aria-label={t("rowActions.copyTrackingUrlAria", { name: campaignName })} size="icon-sm">
              <LinkIcon className="size-3.5" />
            </IconButton>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{t("rowActions.copyTrackingUrlMany", { count: links.length })}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end" className="w-auto max-w-[min(36rem,90vw)]">
        <DropdownMenuLabel>{label}</DropdownMenuLabel>
        {links.map((link) => (
          <DropdownMenuItem key={link.id} onSelect={() => copy(link)}>
            {!link.secure && <LockOpenIcon className="size-3.5 text-warning" aria-label={t("rowActions.copyTrackingUrlInsecure")} />}
            <Mono className="truncate text-xs">{link.url}</Mono>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

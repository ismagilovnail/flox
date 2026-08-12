"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Trash2Icon } from "lucide-react";
import { formatDistanceToNow } from "date-fns";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Caption, Mono } from "@/components/ui/typography";
import { GalleryPreviewTile } from "@/features/content-gallery/gallery-preview-tile";
import { GALLERY_CATEGORY_LABELS, type GalleryItem } from "@/lib/mock/content-gallery";
import { useContentGalleryStore } from "@/stores/content-gallery";
import { useTeamStore } from "@/stores/team";

const BUILDER_ROUTE: Record<Exclude<GalleryItem["category"], "creative_asset">, string> = {
  landing_template: "/landings",
  pwa_template: "/pwa",
  postlanding_template: "/postlanding",
};

export function GalleryPreviewDialog({
  item,
  open,
  onOpenChange,
  canManage,
}: {
  item: GalleryItem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  canManage: boolean;
}) {
  const router = useRouter();
  const deleteTeamItem = useContentGalleryStore((s) => s.deleteTeamItem);
  const uploader = useTeamStore((s) => s.members.find((m) => m.id === item.uploadedByMemberId));

  function useThis() {
    if (item.category === "creative_asset") {
      navigator.clipboard.writeText(item.assetPayload?.fileUrl ?? "");
      toast("Asset URL copied", { description: "Paste it into any URL field, e.g. a PWA icon or offer creative." });
      return;
    }
    router.push(`${BUILDER_ROUTE[item.category]}?gallery=${item.id}`);
    onOpenChange(false);
  }

  function remove() {
    deleteTeamItem(item.id);
    toast("Removed from gallery", { description: item.title });
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="overflow-hidden p-0 sm:max-w-lg">
        <GalleryPreviewTile item={item} className="aspect-[21/9]" />
        <div className="flex flex-col gap-4 p-6 pt-2">
          <DialogHeader className="gap-1.5 p-0 text-left">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="outline">{GALLERY_CATEGORY_LABELS[item.category]}</Badge>
              {item.source === "team" && <Badge variant="info">Team</Badge>}
            </div>
            <DialogTitle>{item.title}</DialogTitle>
            <DialogDescription>{item.description}</DialogDescription>
          </DialogHeader>

          <div className="flex flex-wrap gap-1.5">
            {item.tags.map((t) => (
              <Badge key={t} variant="secondary">
                {t}
              </Badge>
            ))}
          </div>

          {item.category === "creative_asset" && item.assetPayload && (
            <div className="grid gap-1 rounded-md bg-muted/50 p-3 text-xs">
              <div className="flex justify-between">
                <Caption>File</Caption>
                <Mono>{item.assetPayload.fileType}</Mono>
              </div>
              <div className="flex justify-between">
                <Caption>URL</Caption>
                <Mono className="max-w-[60%] truncate">{item.assetPayload.fileUrl}</Mono>
              </div>
            </div>
          )}

          <Caption className="text-muted-foreground">
            {item.source === "system"
              ? "Provided by FLOX"
              : `Uploaded by ${uploader?.name ?? "a teammate"}`}{" "}
            · {formatDistanceToNow(new Date(item.createdAt), { addSuffix: true })}
          </Caption>

          <DialogFooter className="mt-0 flex-row items-center justify-between p-0">
            {item.source === "team" && canManage ? (
              <Button variant="ghost" size="sm" onClick={remove} className="text-danger hover:text-danger">
                <Trash2Icon className="size-3.5" /> Remove
              </Button>
            ) : (
              <span />
            )}
            <Button onClick={useThis}>{item.category === "creative_asset" ? "Copy asset URL" : "Use this"}</Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  );
}

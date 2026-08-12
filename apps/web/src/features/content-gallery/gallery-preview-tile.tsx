import { FileArchiveIcon, FilmIcon, ImageIcon, LayoutTemplateIcon, MousePointerClickIcon, SmartphoneIcon } from "lucide-react";

import { cn } from "@/lib/utils";
import type { GalleryCategory, GalleryItem } from "@/lib/mock/content-gallery";

const CATEGORY_ICON: Record<GalleryCategory, typeof LayoutTemplateIcon> = {
  landing_template: LayoutTemplateIcon,
  pwa_template: SmartphoneIcon,
  postlanding_template: MousePointerClickIcon,
  creative_asset: ImageIcon,
};

const ASSET_ICON: Record<"image" | "video" | "zip", typeof ImageIcon> = {
  image: ImageIcon,
  video: FilmIcon,
  zip: FileArchiveIcon,
};

/** A generated, non-photographic preview tile — this gallery has no real
 * image hosting, so we never fake a thumbnail that looks like a real asset. */
export function GalleryPreviewTile({ item, className }: { item: GalleryItem; className?: string }) {
  const Icon = item.category === "creative_asset" && item.assetPayload ? ASSET_ICON[item.assetPayload.fileType] : CATEGORY_ICON[item.category];
  return (
    <div
      className={cn("flex aspect-video w-full items-center justify-center", className)}
      style={{ background: `linear-gradient(135deg, ${item.previewColor}33, ${item.previewColor}0d)` }}
    >
      <Icon className="size-8" style={{ color: item.previewColor }} />
    </div>
  );
}

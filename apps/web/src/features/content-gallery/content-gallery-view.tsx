"use client";

import * as React from "react";
import { SearchIcon, UploadIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { GalleryItemCard } from "@/features/content-gallery/gallery-item-card";
import { GalleryPreviewDialog } from "@/features/content-gallery/gallery-preview-dialog";
import { UploadAssetDialog } from "@/features/content-gallery/upload-asset-dialog";
import { GALLERY_CATEGORY_LABELS, type GalleryCategory, type GalleryItem, type GallerySource } from "@/lib/mock/content-gallery";
import { useContentGalleryStore } from "@/stores/content-gallery";
import { useCurrentMember } from "@/hooks/use-current-member";

const CATEGORY_OPTIONS: Array<{ value: GalleryCategory | "all"; label: string }> = [
  { value: "all", label: "All categories" },
  ...(Object.entries(GALLERY_CATEGORY_LABELS) as Array<[GalleryCategory, string]>).map(([value, label]) => ({ value, label })),
];

export function ContentGalleryView() {
  const items = useContentGalleryStore((s) => s.items);
  const { memberId: CURRENT_USER_MEMBER_ID, isOwnerOrAdmin: canManage } = useCurrentMember();

  const [search, setSearch] = React.useState("");
  const [category, setCategory] = React.useState<GalleryCategory | "all">("all");
  const [source, setSource] = React.useState<GallerySource | "all">("all");
  const [previewItem, setPreviewItem] = React.useState<GalleryItem | null>(null);
  const [uploading, setUploading] = React.useState(false);

  const filtered = React.useMemo(() => {
    const q = search.trim().toLowerCase();
    return items.filter((item) => {
      if (category !== "all" && item.category !== category) return false;
      if (source !== "all" && item.source !== source) return false;
      if (!q) return true;
      return (
        item.title.toLowerCase().includes(q) ||
        item.description.toLowerCase().includes(q) ||
        item.tags.some((t) => t.toLowerCase().includes(q))
      );
    });
  }, [items, search, category, source]);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Content Gallery</h1>
        <Button onClick={() => setUploading(true)}>
          <UploadIcon className="size-4" /> Upload asset
        </Button>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <Tabs value={source} onValueChange={(v) => setSource(v as GallerySource | "all")}>
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="system">System</TabsTrigger>
            <TabsTrigger value="team">Team</TabsTrigger>
          </TabsList>
        </Tabs>

        <div className="flex items-center gap-2">
          <div className="relative">
            <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search gallery..."
              className="w-56 pl-8"
            />
          </div>
          <Select value={category} onValueChange={(v) => setCategory(v as GalleryCategory | "all")}>
            <SelectTrigger className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CATEGORY_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {filtered.length === 0 ? (
        <EmptyState
          icon={SearchIcon}
          title="No items match"
          description="Try a different search term, category, or source."
        />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {filtered.map((item) => (
            <GalleryItemCard key={item.id} item={item} onClick={() => setPreviewItem(item)} />
          ))}
        </div>
      )}

      {previewItem && (
        <GalleryPreviewDialog
          item={previewItem}
          open
          onOpenChange={(open) => !open && setPreviewItem(null)}
          canManage={canManage || previewItem.uploadedByMemberId === CURRENT_USER_MEMBER_ID}
        />
      )}

      {uploading && (
        <UploadAssetDialog open onOpenChange={(open) => !open && setUploading(false)} memberId={CURRENT_USER_MEMBER_ID} />
      )}
    </div>
  );
}

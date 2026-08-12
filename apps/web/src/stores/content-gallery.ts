import { create } from "zustand";

import { genId } from "@/lib/id";
import {
  CONTENT_GALLERY_ITEMS,
  GALLERY_PREVIEW_COLORS,
  type GalleryItem,
} from "@/lib/mock/content-gallery";

type UploadInput = {
  title: string;
  description: string;
  fileType: "image" | "video" | "zip";
  fileUrl: string;
  tags: string[];
  memberId: string;
};

type ContentGalleryState = {
  items: GalleryItem[];
  /** Team uploads are always `creative_asset` — there is no authoring flow for
   * templates yet (those are curated/system), see mock/content-gallery.ts. */
  uploadTeamAsset: (input: UploadInput) => string;
  deleteTeamItem: (id: string) => void;
};

export const useContentGalleryStore = create<ContentGalleryState>()((set) => ({
  items: [...CONTENT_GALLERY_ITEMS],

  uploadTeamAsset: (input) => {
    const id = genId();
    const item: GalleryItem = {
      id,
      title: input.title,
      description: input.description,
      category: "creative_asset",
      source: "team",
      previewColor: GALLERY_PREVIEW_COLORS[Math.floor(Math.random() * GALLERY_PREVIEW_COLORS.length)],
      tags: input.tags,
      createdAt: new Date().toISOString(),
      uploadedByMemberId: input.memberId,
      assetPayload: { fileType: input.fileType, fileUrl: input.fileUrl, fileSizeKb: 0 },
    };
    set((s) => ({ items: [...s.items, item] }));
    return id;
  },

  deleteTeamItem: (id) => {
    set((s) => ({ items: s.items.filter((i) => !(i.id === id && i.source === "team")) }));
  },
}));

"use client";

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { GalleryBrowser } from "@/features/content-gallery/gallery-browser";
import type { GalleryFile } from "@/lib/api/content-gallery";

/** A single-select embedding of the Content Gallery, for any form field
 * that needs an already-uploaded (or freshly-uploaded) image URL — first
 * consumer: pwa-form.tsx's icon field. Matches the reference's own
 * embedded file-selection dialog used across its platform (e.g. its PWA
 * cover picker). */
export function GalleryFilePickerDialog({
  open,
  onOpenChange,
  onPick,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onPick: (file: GalleryFile) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Choose an image</DialogTitle>
          <DialogDescription>Pick a file already in your Content Gallery, or upload a new one.</DialogDescription>
        </DialogHeader>
        <GalleryBrowser
          mode="pick"
          onPick={(file) => {
            onPick(file);
            onOpenChange(false);
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

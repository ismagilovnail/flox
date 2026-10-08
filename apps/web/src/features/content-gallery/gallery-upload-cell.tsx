"use client";

import * as React from "react";
import { Loader2Icon, UploadCloudIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { cn } from "@/lib/utils";
import { useUploadGalleryFile } from "@/hooks/use-content-gallery";
import type { GalleryFile } from "@/lib/api/content-gallery";

const ACCEPT = ".webp,.jpg,.jpeg,.png,.gif,image/webp,image/jpeg,image/png,image/gif";

/** The upload cell always renders first in the grid (matches the
 * reference's own layout) — click it or drag a file onto it. Uploads go
 * straight to storage as-is, no crop step: the reference's own crop tool
 * only ever appears when a file is *attached* to a field that requires a
 * specific aspect ratio (see image-crop-dialog.tsx's own doc), not at
 * upload time — a plain Content Gallery upload is meant to keep the
 * file's original dimensions. */
export function GalleryUploadCell({
  folderId,
  onUploaded,
  className,
}: {
  folderId?: string;
  onUploaded?: (file: GalleryFile) => void;
  /** Merged over the default `aspect-square` via `cn` (tailwind-merge) —
   * pass e.g. `aspect-[9/16]` to fit a differently-shaped grid (PWA
   * screenshots' own portrait thumbnails, matching the reference's own
   * 1080x1920-recommended framing). */
  className?: string;
}) {
  const { t } = useTranslation("contentGallery");
  const upload = useUploadGalleryFile();
  const [dragging, setDragging] = React.useState(false);
  const inputRef = React.useRef<HTMLInputElement>(null);

  function submit(file: File) {
    upload.mutate(
      { file, folderId },
      {
        onSuccess: onUploaded,
        onError: (err) => toast.error(t("upload.error"), { description: err.message }),
      },
    );
  }

  return (
    <button
      type="button"
      disabled={upload.isPending}
      onClick={() => inputRef.current?.click()}
      onDragOver={(e) => {
        e.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragging(false);
        const file = e.dataTransfer.files?.[0];
        if (file) submit(file);
      }}
      className={cn(
        "flex aspect-square flex-col items-center justify-center gap-1.5 rounded-lg border-2 border-dashed text-muted-foreground transition-colors",
        dragging ? "border-primary bg-primary/5 text-primary" : "border-border hover:border-muted-foreground/50 hover:text-foreground",
        className,
      )}
    >
      {upload.isPending ? <Loader2Icon className="size-6 animate-spin" /> : <UploadCloudIcon className="size-6" />}
      <span className="text-xs font-medium">{upload.isPending ? t("upload.pending") : t("upload.label")}</span>
      <input
        ref={inputRef}
        type="file"
        accept={ACCEPT}
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) submit(file);
          e.target.value = "";
        }}
      />
    </button>
  );
}

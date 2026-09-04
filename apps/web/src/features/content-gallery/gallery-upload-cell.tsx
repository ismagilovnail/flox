"use client";

import * as React from "react";
import { Loader2Icon, UploadCloudIcon } from "lucide-react";
import { toast } from "sonner";

import { cn } from "@/lib/utils";
import { useUploadGalleryFile } from "@/hooks/use-content-gallery";

const ACCEPT = ".webp,.jpg,.jpeg,.png,.gif,image/webp,image/jpeg,image/png,image/gif";

/** The upload cell always renders first in the grid (matches the
 * reference's own layout) — click it or drag a file onto it. */
export function GalleryUploadCell({ folderId }: { folderId?: string }) {
  const upload = useUploadGalleryFile();
  const [dragging, setDragging] = React.useState(false);
  const inputRef = React.useRef<HTMLInputElement>(null);

  function submit(file: File) {
    upload.mutate(
      { file, folderId },
      {
        onError: (err) => toast.error("Upload failed", { description: err.message }),
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
      )}
    >
      {upload.isPending ? <Loader2Icon className="size-6 animate-spin" /> : <UploadCloudIcon className="size-6" />}
      <span className="text-xs font-medium">{upload.isPending ? "Uploading…" : "Upload"}</span>
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

"use client";

import { CopyIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Mono } from "@/components/ui/typography";
import { formatDuration } from "@/features/content-gallery/gallery-file-card";
import type { GalleryFile } from "@/lib/api/content-gallery";

/** A gallery video in a player, with its link to copy (to use it in a
 * creative or a landing). Played as stored — FLOX does not transcode. */
export function GalleryVideoDialog({ file, onOpenChange }: { file: GalleryFile | null; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation("contentGallery");
  const facts = file
    ? [file.width && file.height ? `${file.width}×${file.height}` : null, file.durationMs ? formatDuration(file.durationMs) : null, file.mimeType]
        .filter(Boolean)
        .join(" · ")
    : "";
  return (
    <Dialog open={file !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="truncate">{file?.originalFilename}</DialogTitle>
          <DialogDescription className="font-mono text-xs">{facts}</DialogDescription>
        </DialogHeader>
        {file && (
          <video
            key={file.id}
            src={file.url}
            poster={file.posterUrl}
            controls
            playsInline
            preload="metadata"
            className="max-h-[60vh] w-full rounded-md bg-black"
          >
            {t("video.unsupported")}
          </video>
        )}
        {file && (
          <div className="flex min-w-0 items-center gap-2">
            <Mono className="truncate text-xs text-muted-foreground">{file.url}</Mono>
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={() =>
                navigator.clipboard.writeText(file.url).then(
                  () => toast(t("video.copied")),
                  () => toast.error(file.url),
                )
              }
            >
              <CopyIcon className="size-3.5" /> {t("video.copyLink")}
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

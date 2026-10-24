"use client";

import {
  ArchiveIcon,
  ArchiveRestoreIcon,
  FilmIcon,
  FolderInputIcon,
  Loader2Icon,
  MoreVerticalIcon,
  PlayIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { IconButton } from "@/components/ui/icon-button";
import { cn } from "@/lib/utils";
import type { GalleryFile, GalleryFolder } from "@/lib/api/content-gallery";

/** A video's length as m:ss. */
export function formatDuration(ms: number): string {
  const total = Math.round(ms / 1000);
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, "0")}`;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const kb = bytes / 1024;
  if (kb < 1024) return `${kb.toFixed(0)} KB`;
  return `${(kb / 1024).toFixed(1)} MB`;
}

export function GalleryFileCard({
  file,
  folders,
  selected,
  onToggleSelect,
  onClick,
  onMove,
  onArchive,
  onRestore,
  onDeletePermanently,
  onPreview,
}: {
  file: GalleryFile;
  /** A video's play button: opens it in a player. */
  onPreview?: () => void;
  folders: GalleryFolder[];
  selected: boolean;
  onToggleSelect: () => void;
  onClick?: () => void;
  onMove?: (folderId: string | null) => void;
  onArchive?: () => void;
  onRestore?: () => void;
  onDeletePermanently?: () => void;
}) {
  const { t } = useTranslation("contentGallery");
  // A video being converted, or whose conversion failed, has nothing to
  // show or play yet (docs/content-gallery.md "Transcoding").
  const pending = file.processingStatus === "processing";
  const failed = file.processingStatus === "failed";
  return (
    <div
      className={cn(
        "group relative aspect-square overflow-hidden rounded-lg border bg-muted/30 transition-shadow",
        selected ? "border-primary ring-2 ring-primary" : "border-border hover:shadow-md",
      )}
    >
      <button type="button" onClick={onClick} className="absolute inset-0 flex items-center justify-center">
        {pending ? (
          <span className="flex flex-col items-center gap-1.5 px-2 text-center text-xs text-muted-foreground">
            <Loader2Icon className="size-6 animate-spin motion-reduce:animate-none" aria-hidden />
            {t("card.processing")}
          </span>
        ) : failed ? (
          <span className="flex flex-col items-center gap-1.5 px-2 text-center text-xs text-destructive" title={file.processingError}>
            <TriangleAlertIcon className="size-6" aria-hidden />
            {t("card.failed")}
            {file.processingError && <span className="line-clamp-3 text-[10px] text-muted-foreground">{file.processingError}</span>}
          </span>
        ) : file.fileType === "video" ? (
          file.posterUrl ? (
            // eslint-disable-next-line @next/next/no-img-element -- an object-storage URL, not a Next-optimizable static asset
            <img src={file.posterUrl} alt={file.originalFilename} className="size-full object-cover" loading="lazy" />
          ) : (
            <FilmIcon className="size-8 text-muted-foreground" aria-label={file.originalFilename} />
          )
        ) : (
          // eslint-disable-next-line @next/next/no-img-element -- a presigned object-storage URL, not a Next-optimizable static asset
          <img src={file.url} alt={file.originalFilename} className="size-full object-cover" loading="lazy" />
        )}
      </button>
      {file.fileType === "video" && !pending && !failed && (
        <>
          {onPreview && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onPreview();
              }}
              aria-label={t("card.play", { name: file.originalFilename })}
              className="absolute top-1/2 left-1/2 flex size-10 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full bg-black/55 text-white transition-colors hover:bg-black/75"
            >
              <PlayIcon className="size-4 translate-x-px fill-current" />
            </button>
          )}
          {file.durationMs ? (
            <span className="pointer-events-none absolute right-1.5 bottom-1.5 rounded bg-black/65 px-1 font-mono text-[10px] text-white tabular-nums group-hover:opacity-0">
              {formatDuration(file.durationMs)}
            </span>
          ) : null}
        </>
      )}

      <div
        className={cn(
          "absolute top-1.5 left-1.5 transition-opacity",
          selected ? "opacity-100" : "opacity-0 group-hover:opacity-100",
        )}
      >
        <Checkbox
          checked={selected}
          onCheckedChange={onToggleSelect}
          onClick={(e) => e.stopPropagation()}
          className="border-white bg-black/30 data-checked:bg-primary"
          aria-label={selected ? t("card.deselect") : t("card.select")}
        />
      </div>

      {(onMove || onArchive || onRestore || onDeletePermanently) && (
        <div className="absolute top-1 right-1 opacity-0 transition-opacity group-hover:opacity-100">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <IconButton size="icon-sm" variant="secondary" aria-label={t("card.actions")} onClick={(e) => e.stopPropagation()}>
                <MoreVerticalIcon className="size-3.5" />
              </IconButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {onMove && (
                <DropdownMenuSub>
                  <DropdownMenuSubTrigger>
                    <FolderInputIcon className="size-3.5" /> {t("card.moveTo")}
                  </DropdownMenuSubTrigger>
                  <DropdownMenuSubContent>
                    <DropdownMenuItem onClick={() => onMove(null)}>{t("folders.unfiled")}</DropdownMenuItem>
                    {folders.map((f) => (
                      <DropdownMenuItem key={f.id} onClick={() => onMove(f.id)}>
                        {f.name}
                      </DropdownMenuItem>
                    ))}
                  </DropdownMenuSubContent>
                </DropdownMenuSub>
              )}
              {onArchive && (
                <DropdownMenuItem onClick={onArchive}>
                  <ArchiveIcon className="size-3.5" /> {t("actions.archive")}
                </DropdownMenuItem>
              )}
              {onRestore && (
                <DropdownMenuItem onClick={onRestore}>
                  <ArchiveRestoreIcon className="size-3.5" /> {t("actions.restore")}
                </DropdownMenuItem>
              )}
              {onDeletePermanently && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive" onClick={onDeletePermanently}>
                    <Trash2Icon className="size-3.5" /> {t("actions.deletePermanently")}
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}

      <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/70 to-transparent p-1.5 pt-4 opacity-0 transition-opacity group-hover:opacity-100">
        <p className="truncate text-[11px] font-medium text-white">{file.originalFilename}</p>
        <p className="text-[10px] text-white/70">{formatBytes(file.sizeBytes)}</p>
      </div>
    </div>
  );
}

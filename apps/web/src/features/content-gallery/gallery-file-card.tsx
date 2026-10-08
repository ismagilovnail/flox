"use client";

import { ArchiveIcon, ArchiveRestoreIcon, FolderInputIcon, MoreVerticalIcon, Trash2Icon } from "lucide-react";
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
}: {
  file: GalleryFile;
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
  return (
    <div
      className={cn(
        "group relative aspect-square overflow-hidden rounded-lg border bg-muted/30 transition-shadow",
        selected ? "border-primary ring-2 ring-primary" : "border-border hover:shadow-md",
      )}
    >
      <button type="button" onClick={onClick} className="absolute inset-0 flex items-center justify-center">
        {/* eslint-disable-next-line @next/next/no-img-element -- a presigned object-storage URL, not a Next-optimizable static asset */}
        <img src={file.url} alt={file.originalFilename} className="size-full object-cover" loading="lazy" />
      </button>

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

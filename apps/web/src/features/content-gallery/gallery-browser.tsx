"use client";

import * as React from "react";
import { ChevronLeftIcon, ChevronRightIcon, ImageOffIcon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { usePermission } from "@/hooks/use-auth";
import {
  useArchiveGalleryFiles,
  useDeleteGalleryFiles,
  useGalleryFiles,
  useGalleryFolders,
  useMoveGalleryFiles,
  useRestoreGalleryFiles,
  useUploadGalleryFile,
} from "@/hooks/use-content-gallery";
import { useTeamMembers } from "@/hooks/use-team";
import { GalleryFileCard } from "@/features/content-gallery/gallery-file-card";
import { GalleryFolderPanel, GALLERY_ROOT } from "@/features/content-gallery/gallery-folder-panel";
import { GalleryUploadCell } from "@/features/content-gallery/gallery-upload-cell";
import { GalleryVideoDialog } from "@/features/content-gallery/gallery-video-dialog";
import { CROPPABLE_MIME_TYPES, ImageCropDialog } from "@/features/content-gallery/image-crop-dialog";
import type { GalleryFile, GalleryFileStatus, GalleryFileType, GalleryFolder } from "@/lib/api/content-gallery";

const PAGE_SIZE = 50;

/** Shared by the full Content Gallery page and the embedded file-picker
 * dialog (this package's own reference: both are the same grid/folder/
 * filter machinery, just with a different selection mode and toolbar). */
export function GalleryBrowser({
  mode,
  onPick,
  requiredAspect,
}: {
  mode: "manage" | "pick";
  /** Called with the chosen file when mode === "pick" and the caller
   * confirms a selection. */
  onPick?: (file: GalleryFile) => void;
  /** When set (mode === "pick" only), confirming a selection opens
   * ImageCropDialog locked to this aspect before onPick fires — the
   * reference's own "attach an icon" flow: pick/upload as-is, crop once
   * you actually attach it to a field that needs a specific ratio. */
  requiredAspect?: number;
}) {
  // §52 "frontend permissions are UX only" — content.write is the same
  // key apps/internal/tags.Handler and the other content-gallery routes
  // (migration 00035) actually require server-side (Owner/Admin/Manager),
  // replacing a stale Owner/Admin-only role check that (a) used the wrong
  // tier and (b) never gated the real mutations below (upload, move,
  // archive, restore, permanent delete) — only the "Uploaded by" filter,
  // which isn't sensitive and is now shown to every role regardless.
  const { t } = useTranslation(["contentGallery", "common"]);
  const canWrite = usePermission("content.write");
  const { data: members } = useTeamMembers();
  const { data: folderData } = useGalleryFolders();

  const [folder, setFolderRaw] = React.useState<string | undefined>(undefined);
  const [status, setStatusRaw] = React.useState<GalleryFileStatus>("active");
  const [uploadedBy, setUploadedByRaw] = React.useState<string | undefined>(undefined);
  // Manage mode filters by type; pick mode only ever offers images (every
  // field that picks from the gallery — icon, screenshots, logo, avatar —
  // needs one).
  const [typeFilter, setTypeFilterRaw] = React.useState<GalleryFileType | undefined>(undefined);
  const fileType: GalleryFileType | undefined = mode === "pick" ? "image" : typeFilter;
  const [previewing, setPreviewing] = React.useState<GalleryFile | null>(null);
  const [page, setPage] = React.useState(0);
  const [selected, setSelected] = React.useState<Set<string>>(new Set());
  // Pick mode's actual chosen file object, kept alongside `selected`
  // (which only tracks ids, for card highlighting) — needed so a file
  // just uploaded via GalleryUploadCell can become "picked" immediately,
  // before the list query has even refetched to include it.
  const [pickedFile, setPickedFile] = React.useState<GalleryFile | null>(null);
  // Non-null while the crop-before-attach dialog is open (requiredAspect
  // only) — the file the "Use this image" confirm is currently cropping.
  const [cropTarget, setCropTarget] = React.useState<GalleryFile | null>(null);
  // Non-null while the permanent-delete confirm dialog is open — the ids
  // it would delete. Shared by the bulk bar and each card's own menu
  // (single-file: an array of one).
  const [confirmDeleteIds, setConfirmDeleteIds] = React.useState<string[] | null>(null);

  // Each filter setter also resets pagination — done here, not via a
  // useEffect watching all three, so changing a filter never causes an
  // extra cascading render (react-hooks/set-state-in-effect).
  function setFolder(value: string | undefined) {
    setFolderRaw(value);
    setPage(0);
  }
  function setStatus(value: GalleryFileStatus) {
    setStatusRaw(value);
    setPage(0);
  }
  function setUploadedBy(value: string | undefined) {
    setUploadedByRaw(value);
    setPage(0);
  }
  function setTypeFilter(value: GalleryFileType | undefined) {
    setTypeFilterRaw(value);
    setPage(0);
  }

  const folderId = folder === GALLERY_ROOT ? "root" : folder;
  const { data, isPending, isError, error, refetch } = useGalleryFiles({
    folderId,
    status: mode === "pick" ? "active" : status,
    type: fileType,
    uploadedBy,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  });
  const files = data?.files ?? [];
  const total = data?.total ?? 0;
  const pageCount = Math.max(Math.ceil(total / PAGE_SIZE), 1);

  const moveFiles = useMoveGalleryFiles();
  const archiveFiles = useArchiveGalleryFiles();
  const restoreFiles = useRestoreGalleryFiles();
  const deleteFiles = useDeleteGalleryFiles();
  const uploadFile = useUploadGalleryFile();

  function toggleSelect(id: string) {
    if (mode === "pick") {
      setSelected((prev) => {
        if (prev.has(id)) {
          setPickedFile(null);
          return new Set();
        }
        setPickedFile(files.find((f) => f.id === id) ?? null);
        return new Set([id]);
      });
      return;
    }
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  // A file just uploaded through the picker's own upload cell becomes
  // the pick immediately — it isn't in `files` yet (the list query
  // hasn't refetched), so this is the only place that reference exists.
  function handleUploaded(file: GalleryFile) {
    setSelected(new Set([file.id]));
    setPickedFile(file);
  }

  function confirmPick() {
    if (!pickedFile) return;
    if (requiredAspect && CROPPABLE_MIME_TYPES.has(pickedFile.mimeType)) {
      setCropTarget(pickedFile);
    } else {
      onPick?.(pickedFile);
    }
  }

  function skipCrop() {
    const file = cropTarget;
    setCropTarget(null);
    if (file) onPick?.(file);
  }

  function applyCrop(cropped: File) {
    uploadFile.mutate(
      { file: cropped },
      {
        onSuccess: (newFile) => {
          setCropTarget(null);
          onPick?.(newFile);
        },
        onError: (err) => {
          toast.error(t("crop.saveError"), { description: err.message });
          setCropTarget(null);
        },
      },
    );
  }

  function runBulk(action: (ids: string[]) => Promise<unknown>, verb: "archived" | "restored" | "moved") {
    const count = selected.size;
    action(Array.from(selected))
      .then(() => {
        toast(t(`bulk.done.${verb}`, { count }));
        setSelected(new Set());
      })
      .catch((err: Error) => toast.error(t(`bulk.error.${verb}`), { description: err.message }));
  }

  function confirmDelete() {
    const ids = confirmDeleteIds;
    if (!ids) return;
    deleteFiles.mutate(ids, {
      onSuccess: () => {
        toast(t("bulk.done.deleted", { count: ids.length }));
        setSelected((prev) => {
          const next = new Set(prev);
          for (const id of ids) next.delete(id);
          return next;
        });
        setConfirmDeleteIds(null);
      },
      onError: (err) => toast.error(t("bulk.error.deleted"), { description: err.message }),
    });
  }

  return (
    <div className="flex flex-col gap-4">
      {mode === "manage" && (
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Tabs value={status} onValueChange={(v) => setStatus(v as GalleryFileStatus)}>
            <TabsList>
              <TabsTrigger value="active">{t("tabs.active")}</TabsTrigger>
              <TabsTrigger value="archived">{t("tabs.archived")}</TabsTrigger>
            </TabsList>
          </Tabs>

          <div className="flex flex-wrap items-center gap-2">
            <Select value={typeFilter ?? "all"} onValueChange={(v) => setTypeFilter(v === "all" ? undefined : (v as GalleryFileType))}>
              <SelectTrigger className="w-36" aria-label={t("typeFilter.label")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("typeFilter.all")}</SelectItem>
                <SelectItem value="image">{t("typeFilter.image")}</SelectItem>
                <SelectItem value="video">{t("typeFilter.video")}</SelectItem>
              </SelectContent>
            </Select>
            <Select value={uploadedBy ?? "all"} onValueChange={(v) => setUploadedBy(v === "all" ? undefined : v)}>
              <SelectTrigger className="w-48">
                <SelectValue placeholder={t("uploadedBy")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("everyone")}</SelectItem>
                {members?.map((m) => (
                  <SelectItem key={m.userId} value={m.userId}>
                    {m.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      )}

      <div className="flex gap-4">
        <GalleryFolderPanel selected={folder} onSelect={setFolder} />

        <div className="flex min-w-0 flex-1 flex-col gap-3">
          {mode === "manage" && selected.size > 0 && (
            <BulkActionBar
              count={selected.size}
              status={status}
              folders={folderData?.folders ?? []}
              canWrite={canWrite}
              onDeselect={() => setSelected(new Set())}
              onArchive={() => runBulk((ids) => archiveFiles.mutateAsync(ids), "archived")}
              onRestore={() => runBulk((ids) => restoreFiles.mutateAsync(ids), "restored")}
              onMove={(folderId) => runBulk((ids) => moveFiles.mutateAsync({ ids, folderId }), "moved")}
              onDeletePermanently={() => setConfirmDeleteIds(Array.from(selected))}
            />
          )}

          {isPending ? (
            <LoadingState label={t("loading")} />
          ) : isError ? (
            <ErrorState title={t("loadError")} description={error.message} onRetry={() => refetch()} />
          ) : (
            <>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
                {/* Uploading is mode === "pick" (any role can attach an existing/new image
                    to a field they already have content.write on elsewhere, e.g. a PWA
                    icon) or a manage-mode upload, which DOES need content.write. */}
                {(mode === "pick" || (status === "active" && canWrite)) && (
                  <GalleryUploadCell
                    folderId={folderId === "root" ? undefined : folderId}
                    onUploaded={mode === "pick" ? handleUploaded : undefined}
                    allowVideo={mode === "manage"}
                  />
                )}
                {files.length === 0 && !(mode === "pick" || status === "active") && (
                  <div className="col-span-full">
                    <EmptyState icon={ImageOffIcon} title={t("emptyTitle")} description={t("emptyDescription")} />
                  </div>
                )}
                {files.map((file) => (
                  <GalleryFileCard
                    key={file.id}
                    file={file}
                    folders={folderData?.folders ?? []}
                    selected={selected.has(file.id)}
                    onToggleSelect={() => toggleSelect(file.id)}
                    onClick={() => toggleSelect(file.id)}
                    onPreview={file.fileType === "video" ? () => setPreviewing(file) : undefined}
                    onMove={
                      mode === "manage" && status === "active" && canWrite
                        ? (folderId) => moveFiles.mutate({ ids: [file.id], folderId })
                        : undefined
                    }
                    onArchive={
                      mode === "manage" && status === "active" && canWrite ? () => archiveFiles.mutate([file.id]) : undefined
                    }
                    onRestore={
                      mode === "manage" && status === "archived" && canWrite ? () => restoreFiles.mutate([file.id]) : undefined
                    }
                    onDeletePermanently={
                      mode === "manage" && status === "archived" && canWrite ? () => setConfirmDeleteIds([file.id]) : undefined
                    }
                  />
                ))}
              </div>

              {total > PAGE_SIZE && (
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span className="font-mono">{t("pagination", { page: page + 1, pages: pageCount, count: total })}</span>
                  <div className="flex items-center gap-1">
                    <Button variant="outline" size="icon-sm" disabled={page === 0} onClick={() => setPage((p) => p - 1)} aria-label={t("common:dataTable.previousPageAria")}>
                      <ChevronLeftIcon className="size-3.5" />
                    </Button>
                    <Button
                      variant="outline"
                      size="icon-sm"
                      disabled={page + 1 >= pageCount}
                      onClick={() => setPage((p) => p + 1)}
                      aria-label={t("common:dataTable.nextPageAria")}
                    >
                      <ChevronRightIcon className="size-3.5" />
                    </Button>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      </div>

      {mode === "pick" && (
        <div className="flex justify-end border-t border-border pt-3">
          <Button disabled={!pickedFile} onClick={confirmPick}>
            {t("picker.use")}
          </Button>
        </div>
      )}

      {requiredAspect && (
        <ImageCropDialog
          imageUrl={cropTarget?.url ?? null}
          filename={cropTarget?.originalFilename ?? "image"}
          mimeType={cropTarget?.mimeType ?? "image/png"}
          aspect={requiredAspect}
          open={cropTarget !== null}
          onOpenChange={(open) => !open && skipCrop()}
          onConfirm={applyCrop}
          onSkip={skipCrop}
        />
      )}

      <GalleryVideoDialog file={previewing} onOpenChange={(open) => !open && setPreviewing(null)} />

      <Dialog open={confirmDeleteIds !== null} onOpenChange={(open) => !open && setConfirmDeleteIds(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("deleteDialog.title", { count: confirmDeleteIds?.length ?? 0 })}</DialogTitle>
            <DialogDescription>{t("deleteDialog.description")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDeleteIds(null)}>
              {t("common:actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={deleteFiles.isPending}>
              {t("actions.deletePermanently")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function BulkActionBar({
  count,
  status,
  folders,
  canWrite,
  onDeselect,
  onArchive,
  onRestore,
  onMove,
  onDeletePermanently,
}: {
  count: number;
  status: GalleryFileStatus;
  folders: GalleryFolder[];
  canWrite: boolean;
  onDeselect: () => void;
  onArchive: () => void;
  onRestore: () => void;
  onMove: (folderId: string | null) => void;
  onDeletePermanently: () => void;
}) {
  const { t } = useTranslation("contentGallery");
  return (
    <div className="flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-2 text-sm">
      <span className="font-medium">{t("bulk.selected", { count })}</span>
      <div className="ml-auto flex items-center gap-2">
        {canWrite && (
          <>
            <Select onValueChange={(v) => onMove(v === GALLERY_ROOT ? null : v)}>
              <SelectTrigger className="h-7 w-40 text-xs">
                <SelectValue placeholder={t("bulk.moveTo")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={GALLERY_ROOT}>{t("folders.unfiled")}</SelectItem>
                {folders.map((f) => (
                  <SelectItem key={f.id} value={f.id}>
                    {f.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {status === "active" ? (
              <Button variant="outline" size="sm" onClick={onArchive}>
                {t("actions.archive")}
              </Button>
            ) : (
              <>
                <Button variant="outline" size="sm" onClick={onRestore}>
                  {t("actions.restore")}
                </Button>
                <Button variant="destructive" size="sm" onClick={onDeletePermanently}>
                  {t("actions.deletePermanently")}
                </Button>
              </>
            )}
          </>
        )}
        <Button variant="ghost" size="sm" onClick={onDeselect}>
          {t("bulk.deselect")}
        </Button>
      </div>
    </div>
  );
}

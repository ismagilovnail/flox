import { apiFetch } from "@/lib/api/client";
import type { VideoInfo } from "@/lib/video-poster";

/** Mirrors apps/internal/contentgallery's real shape — a tenant-scoped
 * media library modeled on the reference tracker's own Content Gallery
 * (https://adset.pro/guide_ru/galereya-kontenta), replacing the former
 * lib/mock/content-gallery.ts "template gallery" mock. Images and videos
 * (MP4/WebM, stored as uploaded — docs/content-gallery.md "Video"; MOV and
other formats are converted to MP4 by apps/worker — "Transcoding"). */
export type GalleryFileType = "image" | "video";
export type GalleryFileStatus = "active" | "archived";
/** processing: a video waiting to be converted to MP4 (no url yet);
 * failed: conversion failed, processingError says why. */
export type GalleryProcessingStatus = "ready" | "processing" | "failed";

export type GalleryFolder = {
  id: string;
  organizationId: string;
  name: string;
  createdAt: string;
  updatedAt: string;
};

export type GalleryFile = {
  id: string;
  organizationId: string;
  folderId: string | null;
  uploadedByUserId: string;
  fileType: GalleryFileType;
  originalFilename: string;
  mimeType: string;
  sizeBytes: number;
  status: GalleryFileStatus;
  createdAt: string;
  updatedAt: string;
  /** Presigned, time-limited — refetch the list rather than caching this
   * across a long-lived page (apps/internal/contentgallery.Service's own
   * doc explains why it's never a permanent URL). */
  url: string;
  /** A video's poster (its first frame, made by the uploading browser),
   * duration and frame size — absent when unknown and for images. */
  posterUrl?: string;
  durationMs?: number;
  width?: number;
  height?: number;
  processingStatus: GalleryProcessingStatus;
  processingError?: string;
};

export type ListFilesParams = {
  /** "root" = unfiled only, a folder id = that folder, omitted = every
   * folder — matches ListFilesParams.FolderID's tri-state server-side. */
  folderId?: string;
  type?: GalleryFileType;
  status?: GalleryFileStatus;
  uploadedBy?: string;
  limit?: number;
  offset?: number;
};

export type ListFilesResult = {
  files: GalleryFile[];
  total: number;
};

export function listFolders(): Promise<{ folders: GalleryFolder[] }> {
  return apiFetch("/content-gallery/folders");
}

export function createFolder(name: string): Promise<GalleryFolder> {
  return apiFetch("/content-gallery/folders", { method: "POST", body: { name } });
}

export function renameFolder(id: string, name: string): Promise<GalleryFolder> {
  return apiFetch(`/content-gallery/folders/${id}`, { method: "PATCH", body: { name } });
}

export function deleteFolder(id: string): Promise<void> {
  return apiFetch(`/content-gallery/folders/${id}`, { method: "DELETE" });
}

export function listFiles(params: ListFilesParams = {}): Promise<ListFilesResult> {
  return apiFetch("/content-gallery/files", {
    searchParams: {
      folderId: params.folderId,
      type: params.type,
      status: params.status,
      uploadedBy: params.uploadedBy,
      limit: params.limit?.toString(),
      offset: params.offset?.toString(),
    },
  });
}

/** video: what the browser read from a video file (lib/video-poster.ts). */
export function uploadFile(file: File, folderId?: string, video?: VideoInfo): Promise<GalleryFile> {
  const form = new FormData();
  form.append("file", file);
  if (folderId) form.append("folderId", folderId);
  if (video?.poster) form.append("poster", video.poster, "poster.jpg");
  if (video?.durationMs) form.append("durationMs", String(video.durationMs));
  if (video?.width && video.height) {
    form.append("width", String(video.width));
    form.append("height", String(video.height));
  }
  return apiFetch("/content-gallery/files", { method: "POST", body: form });
}

export function moveFiles(ids: string[], folderId: string | null): Promise<{ moved: number }> {
  return apiFetch("/content-gallery/files/move", { method: "POST", body: { ids, folderId } });
}

export function archiveFiles(ids: string[]): Promise<{ archived: number }> {
  return apiFetch("/content-gallery/files/archive", { method: "POST", body: { ids } });
}

export function restoreFiles(ids: string[]): Promise<{ restored: number }> {
  return apiFetch("/content-gallery/files/restore", { method: "POST", body: { ids } });
}

/** Permanent — the server only ever deletes ids that are already
 * archived (apps/internal/contentgallery.Repository.DeleteFiles' own
 * WHERE clause enforces this), so this can't be called on an active
 * file to skip the Inactive step. */
export function deleteFiles(ids: string[]): Promise<{ deleted: number }> {
  return apiFetch("/content-gallery/files/delete", { method: "POST", body: { ids } });
}

// --- System content (§30.8; Go contentgallery/system.go) ---

export type SystemItemKind = "creative" | "landing" | "pwa";

export type SystemGalleryItem = {
  id: string;
  kind: SystemItemKind;
  /** Localized: { en, ru }. */
  name: Record<string, string>;
  description: Record<string, string>;
  tags: string[];
  /** Relative to the API origin — pass through apiUrl(). */
  previewPath: string;
  /** Landing templates: where the template's CSS and images are served. */
  assetBasePath?: string;
};

/** The hand-off: `file` for a creative (copied into the library), `landing`
 * for a landing template (created), `pwa` for a PWA template (the PWA form's
 * fields, images already copied into the library). */
export type SystemUseResult = {
  kind: SystemItemKind;
  file?: GalleryFile;
  landing?: { id: string; name: string };
  pwa?: Record<string, unknown>;
};

export function listSystemItems(): Promise<{ items: SystemGalleryItem[] }> {
  return apiFetch("/content-gallery/system");
}

export function previewSystemLanding(id: string, assetBase: string): Promise<{ html: string }> {
  return apiFetch(`/content-gallery/system/${id}/preview`, { searchParams: { assetBase } });
}

export function applySystemItem(id: string, input: { name?: string } = {}): Promise<SystemUseResult> {
  return apiFetch(`/content-gallery/system/${id}/use`, { method: "POST", body: input });
}

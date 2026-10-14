"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  applySystemItem,
  archiveFiles,
  createFolder,
  deleteFiles,
  deleteFolder,
  listFiles,
  listFolders,
  listSystemItems,
  previewSystemLanding,
  moveFiles,
  renameFolder,
  restoreFiles,
  uploadFile,
  type ListFilesParams,
} from "@/lib/api/content-gallery";

const foldersKey = ["content-gallery", "folders"] as const;
const filesKey = (params: ListFilesParams) => ["content-gallery", "files", params] as const;

export function useGalleryFolders() {
  return useQuery({ queryKey: foldersKey, queryFn: listFolders });
}

export function useGalleryFiles(params: ListFilesParams) {
  return useQuery({ queryKey: filesKey(params), queryFn: () => listFiles(params) });
}

function invalidateFiles(qc: ReturnType<typeof useQueryClient>) {
  return qc.invalidateQueries({ queryKey: ["content-gallery", "files"] });
}

export function useCreateGalleryFolder() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => createFolder(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: foldersKey }),
  });
}

export function useRenameGalleryFolder() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => renameFolder(id, name),
    onSuccess: () => qc.invalidateQueries({ queryKey: foldersKey }),
  });
}

export function useDeleteGalleryFolder() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteFolder(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: foldersKey });
      return invalidateFiles(qc);
    },
  });
}

export function useUploadGalleryFile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ file, folderId }: { file: File; folderId?: string }) => uploadFile(file, folderId),
    onSuccess: () => invalidateFiles(qc),
  });
}

export function useMoveGalleryFiles() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ ids, folderId }: { ids: string[]; folderId: string | null }) => moveFiles(ids, folderId),
    onSuccess: () => invalidateFiles(qc),
  });
}

export function useArchiveGalleryFiles() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids: string[]) => archiveFiles(ids),
    onSuccess: () => invalidateFiles(qc),
  });
}

export function useRestoreGalleryFiles() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids: string[]) => restoreFiles(ids),
    onSuccess: () => invalidateFiles(qc),
  });
}

export function useDeleteGalleryFiles() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids: string[]) => deleteFiles(ids),
    onSuccess: () => invalidateFiles(qc),
  });
}

/** The system catalog is the same for everyone and ships with the API —
 * it changes only on deploy. */
export function useSystemGalleryItems() {
  return useQuery({ queryKey: ["content-gallery", "system"], queryFn: listSystemItems, staleTime: Infinity });
}

export function useSystemLandingPreview(id: string | undefined, assetBase: string | undefined) {
  return useQuery({
    queryKey: ["content-gallery", "system", id, "preview"],
    queryFn: () => previewSystemLanding(id!, assetBase!),
    enabled: !!id && !!assetBase,
    staleTime: Infinity,
  });
}

/** Hands a system item to the team; every kind may add files to the library. */
export function useApplySystemItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name?: string }) => applySystemItem(id, name ? { name } : {}),
    onSuccess: (res) => {
      if (res.kind === "landing") qc.invalidateQueries({ queryKey: ["landings"] });
      return invalidateFiles(qc);
    },
  });
}

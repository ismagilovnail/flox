import { apiFetch } from "@/lib/api/client";
import { fetchAllPages } from "@/lib/api/pagination";

/** Mirrors apps/internal/landing's real shape. */
export type LandingType = "internal" | "external";
export type LandingStatus = "active" | "paused" | "archived";

export type LandingEntryFile = "index.html" | "index.twig";

export type Landing = {
  id: string;
  organizationId: string;
  name: string;
  /** Display title of the page. Required for `internal` landings. */
  title: string;
  /** Square logo (Content Gallery URL). Required for `internal` landings. */
  logoUrl: string;
  type: LandingType;
  /** The advertiser's own URL for `external`. For `internal`, the public
   * base URL the uploaded bundle's assets are served from (server-derived;
   * empty when the bundle has no assets). */
  url: string;
  /** `internal` only: the entry file's source — what the template editor
   * edits. Empty for `external`. */
  content: string;
  /** `internal` only: which entry file the uploaded archive carried. */
  entryFile: LandingEntryFile;
  fileCount: number;
  bundleSize: number;
  status: LandingStatus;
  createdAt: string;
  updatedAt: string;
};

/** JSON create — `external` landings only; `internal` ones are created by
 * uploading a ZIP (`uploadLanding`). */
export type CreateLandingInput = {
  name: string;
  title?: string;
  logoUrl?: string;
  type: "external";
  url: string;
  status?: "active" | "paused";
};

/** `type` can't change after creation; `url` applies to `external`,
 * `content` (the template) to `internal`. */
export type UpdateLandingInput = {
  name?: string;
  title?: string;
  logoUrl?: string;
  url?: string;
  content?: string;
  status?: LandingStatus;
};

export type UploadLandingInput = {
  name: string;
  title: string;
  logoUrl: string;
  status: "active" | "paused";
  archive: File;
};

/** An archive upload's result: `warnings` lists root-absolute asset
 * references ("/style.css") in the entry file — they won't resolve
 * against the bundle and should be made relative. */
export type LandingUploadResult = { landing: Landing; warnings: string[] };

export function listLandings(): Promise<{ landings: Landing[]; total: number }> {
  return fetchAllPages<"landings", Landing>("/landings", "landings");
}

export function getLanding(id: string): Promise<Landing> {
  return apiFetch(`/landings/${id}`);
}

export function createLanding(input: CreateLandingInput): Promise<Landing> {
  return apiFetch("/landings", { method: "POST", body: input });
}

export function uploadLanding(input: UploadLandingInput): Promise<LandingUploadResult> {
  const body = new FormData();
  body.set("name", input.name);
  body.set("title", input.title);
  body.set("logoUrl", input.logoUrl);
  body.set("status", input.status);
  body.set("archive", input.archive);
  return apiFetch("/landings/upload", { method: "POST", body });
}

/** Re-uploads an internal landing's archive — same id, new content. */
export function replaceLandingArchive(id: string, archive: File): Promise<LandingUploadResult> {
  const body = new FormData();
  body.set("archive", archive);
  return apiFetch(`/landings/${id}/archive`, { method: "POST", body });
}

/** Renders `content` (or the saved template when omitted) exactly as a
 * visitor would get it, for the editor's sandboxed preview frame. */
export function previewLanding(id: string, content?: string): Promise<{ html: string }> {
  return apiFetch(`/landings/${id}/preview`, { method: "POST", body: content === undefined ? {} : { content } });
}

export function updateLanding(id: string, input: UpdateLandingInput): Promise<Landing> {
  return apiFetch(`/landings/${id}`, { method: "PATCH", body: input });
}

export function deleteLanding(id: string): Promise<void> {
  return apiFetch(`/landings/${id}`, { method: "DELETE" });
}

export function duplicateLanding(id: string): Promise<Landing> {
  return apiFetch(`/landings/${id}/duplicate`, { method: "POST" });
}

export function pauseLanding(id: string): Promise<Landing> {
  return apiFetch(`/landings/${id}/pause`, { method: "POST" });
}

export function activateLanding(id: string): Promise<Landing> {
  return apiFetch(`/landings/${id}/activate`, { method: "POST" });
}

export function archiveLanding(id: string): Promise<Landing> {
  return apiFetch(`/landings/${id}`, { method: "PATCH", body: { status: "archived" } });
}

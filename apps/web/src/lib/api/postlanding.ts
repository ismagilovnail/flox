import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/postlanding's real shape. */
export type PostlandingStatus = "active" | "paused" | "archived";

/**
 * Curated subset of the full §43 event model a postlanding can plausibly
 * fire on (PWA_INSTALL, NOTIFICATION_*, TG_JOIN, TG_START) — matches
 * postlanding.ValidEventTypes exactly. The full canonical event enum
 * belongs to the Conversions/Postbacks domain; don't duplicate it here,
 * just reference the same string values.
 */
export const POSTLANDING_EVENT_TYPES = [
  "PWA_INSTALL",
  "NOTIFICATION_REQUEST",
  "NOTIFICATION_SUBSCRIBE",
  "NOTIFICATION_DECLINE",
  "TG_JOIN",
  "TG_START",
] as const;

export type PostlandingEventType = (typeof POSTLANDING_EVENT_TYPES)[number];

export type Postlanding = {
  id: string;
  organizationId: string;
  name: string;
  url: string;
  events: PostlandingEventType[];
  status: PostlandingStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreatePostlandingInput = {
  name: string;
  url: string;
  events: PostlandingEventType[];
};

export type UpdatePostlandingInput = Partial<CreatePostlandingInput> & { status?: PostlandingStatus };

export function listPostlandings(): Promise<{ postlandings: Postlanding[] }> {
  return apiFetch("/postlandings");
}

export function getPostlanding(id: string): Promise<Postlanding> {
  return apiFetch(`/postlandings/${id}`);
}

export function createPostlanding(input: CreatePostlandingInput): Promise<Postlanding> {
  return apiFetch("/postlandings", { method: "POST", body: input });
}

export function updatePostlanding(id: string, input: UpdatePostlandingInput): Promise<Postlanding> {
  return apiFetch(`/postlandings/${id}`, { method: "PATCH", body: input });
}

export function deletePostlanding(id: string): Promise<void> {
  return apiFetch(`/postlandings/${id}`, { method: "DELETE" });
}

export function duplicatePostlanding(id: string): Promise<Postlanding> {
  return apiFetch(`/postlandings/${id}/duplicate`, { method: "POST" });
}

export function pausePostlanding(id: string): Promise<Postlanding> {
  return apiFetch(`/postlandings/${id}/pause`, { method: "POST" });
}

export function activatePostlanding(id: string): Promise<Postlanding> {
  return apiFetch(`/postlandings/${id}/activate`, { method: "POST" });
}

export function archivePostlanding(id: string): Promise<Postlanding> {
  return apiFetch(`/postlandings/${id}`, { method: "PATCH", body: { status: "archived" } });
}

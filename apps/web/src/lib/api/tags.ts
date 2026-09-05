import { apiFetch } from "@/lib/api/client";
import type { Tag, TagAssignment, TagColorId, TaggableEntityType } from "@/lib/tags";

export function listTags(): Promise<{ tags: Tag[] }> {
  return apiFetch("/tags");
}

export function createTag(input: { name: string; color: TagColorId }): Promise<Tag> {
  return apiFetch("/tags", { method: "POST", body: input });
}

export function updateTag(id: string, input: { name?: string; color?: TagColorId }): Promise<Tag> {
  return apiFetch(`/tags/${id}`, { method: "PATCH", body: input });
}

export function deleteTag(id: string): Promise<void> {
  return apiFetch(`/tags/${id}`, { method: "DELETE" });
}

export function listTagAssignments(entityType: TaggableEntityType): Promise<{ assignments: TagAssignment[] }> {
  return apiFetch("/tags/assignments", { searchParams: { entityType } });
}

/** Replaces the full tag set on one entity — used by the single-item picker. */
export function setEntityTags(entityType: TaggableEntityType, entityId: string, tagIds: string[]): Promise<void> {
  return apiFetch("/tags/assignments/set", { method: "POST", body: { entityType, entityId, tagIds } });
}

/** §30.6 bulk semantics: `toAdd` tags are assigned to every entity in `entityIds`
 * (no-op where already present); `toRemove` tags are unassigned from every entity
 * in `entityIds` (no-op where absent). Tags untouched by the caller are left alone
 * on every entity, even ones only some of the selection had. */
export function bulkEditTags(
  entityType: TaggableEntityType,
  entityIds: string[],
  toAdd: string[],
  toRemove: string[],
): Promise<void> {
  return apiFetch("/tags/assignments/bulk", { method: "POST", body: { entityType, entityIds, toAdd, toRemove } });
}

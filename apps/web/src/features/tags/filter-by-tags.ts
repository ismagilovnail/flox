import type { TagAssignment } from "@/lib/mock/tag-assignments";
import type { TaggableEntityType } from "@/lib/mock/tags";

/** §30.6: multi-tag filter uses OR semantics — item shown if it has AT LEAST
 * ONE selected tag. Empty selection means no filtering. The one filter
 * implementation every taggable list view calls, per spec. */
export function filterByTags<T extends { id: string }>(
  entityType: TaggableEntityType,
  items: T[],
  selectedTagIds: string[],
  assignments: TagAssignment[],
): T[] {
  if (selectedTagIds.length === 0) return items;
  const selected = new Set(selectedTagIds);
  const matchingIds = new Set(
    assignments.filter((a) => a.entityType === entityType && selected.has(a.tagId)).map((a) => a.entityId),
  );
  return items.filter((item) => matchingIds.has(item.id));
}

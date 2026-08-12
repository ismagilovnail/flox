import { create } from "zustand";

import { genId } from "@/lib/id";
import { TAGS, type Tag, type TagColorId } from "@/lib/mock/tags";
import { TAG_ASSIGNMENTS, type TagAssignment } from "@/lib/mock/tag-assignments";
import type { TaggableEntityType } from "@/lib/mock/tags";

type TagsState = {
  tags: Tag[];
  assignments: TagAssignment[];

  createTag: (name: string, color: TagColorId) => string;
  updateTag: (id: string, input: { name: string; color: TagColorId }) => void;
  deleteTag: (id: string) => void;

  /** Replace the full tag set on one entity — used by the single-item picker. */
  setEntityTags: (entityType: TaggableEntityType, entityId: string, tagIds: string[]) => void;

  /** §30.6 bulk semantics: `toAdd` tags are assigned to every entity in `entityIds`
   * (no-op where already present); `toRemove` tags are unassigned from every entity
   * in `entityIds` (no-op where absent). Tags untouched by the caller are left alone
   * on every entity, even ones only some of the selection had. */
  bulkEditTags: (entityType: TaggableEntityType, entityIds: string[], toAdd: string[], toRemove: string[]) => void;
};

export const useTagsStore = create<TagsState>()((set) => ({
  tags: [...TAGS],
  assignments: [...TAG_ASSIGNMENTS],

  createTag: (name, color) => {
    const id = genId();
    const now = new Date().toISOString();
    const tag: Tag = { id, name, color, createdAt: now, updatedAt: now };
    set((s) => ({ tags: [...s.tags, tag] }));
    return id;
  },

  updateTag: (id, input) => {
    set((s) => ({
      tags: s.tags.map((t) => (t.id === id ? { ...t, ...input, updatedAt: new Date().toISOString() } : t)),
    }));
  },

  deleteTag: (id) => {
    set((s) => ({
      tags: s.tags.filter((t) => t.id !== id),
      assignments: s.assignments.filter((a) => a.tagId !== id),
    }));
  },

  setEntityTags: (entityType, entityId, tagIds) => {
    set((s) => ({
      assignments: [
        ...s.assignments.filter((a) => !(a.entityType === entityType && a.entityId === entityId)),
        ...tagIds.map((tagId) => ({ tagId, entityType, entityId })),
      ],
    }));
  },

  bulkEditTags: (entityType, entityIds, toAdd, toRemove) => {
    if (toAdd.length === 0 && toRemove.length === 0) return;
    const entityIdSet = new Set(entityIds);
    const removeSet = new Set(toRemove);

    set((s) => {
      const kept = s.assignments.filter(
        (a) => !(a.entityType === entityType && entityIdSet.has(a.entityId) && removeSet.has(a.tagId)),
      );
      const existingKeys = new Set(kept.map((a) => `${a.entityType}:${a.entityId}:${a.tagId}`));
      const added: TagAssignment[] = [];
      for (const entityId of entityIds) {
        for (const tagId of toAdd) {
          const key = `${entityType}:${entityId}:${tagId}`;
          if (!existingKeys.has(key)) {
            added.push({ tagId, entityType, entityId });
            existingKeys.add(key);
          }
        }
      }
      return { assignments: [...kept, ...added] };
    });
  },
}));

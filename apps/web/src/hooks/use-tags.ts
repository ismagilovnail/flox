"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  bulkEditTags,
  createTag,
  deleteTag,
  listTagAssignments,
  listTags,
  setEntityTags,
  updateTag,
} from "@/lib/api/tags";
import type { Tag, TagAssignment, TagColorId, TaggableEntityType } from "@/lib/tags";

const tagsKey = ["tags"] as const;
const assignmentsKey = (entityType: TaggableEntityType) => ["tags", "assignments", entityType] as const;

// Referentially-stable fallbacks for `useTags().data?.tags ?? ...` /
// `useTagAssignments(...).data?.assignments ?? ...` call sites — a fresh
// `[]` literal there gets a new identity every render while the query is
// still loading, which breaks a useMemo/useEffect dependency array built
// from it (exactly the snapshot-stability trap use-entity-tags.ts's own
// doc comment already warns about, reintroduced here if these aren't
// used).
export const EMPTY_TAGS: Tag[] = [];
export const EMPTY_TAG_ASSIGNMENTS: TagAssignment[] = [];

export function useTags() {
  return useQuery({ queryKey: tagsKey, queryFn: listTags });
}

/** Every assignment for one entity type, org-wide — a list page hydrates
 * its whole "which tags are on which row" state from this one query,
 * same shape the old mock store held in memory (§30.6). */
export function useTagAssignments(entityType: TaggableEntityType) {
  return useQuery({ queryKey: assignmentsKey(entityType), queryFn: () => listTagAssignments(entityType) });
}

// Every mutation below invalidates the whole "tags" key prefix (list +
// every mounted assignment query), not just its own slice — this is a
// low-traffic admin feature where correctness across every open tab
// (a tag renamed while another tab's list still shows its old name,
// a tag deleted while its badge is still visible elsewhere) matters
// more than shaving a handful of refetches.
function useInvalidateTags() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: tagsKey });
}

export function useCreateTag() {
  const invalidate = useInvalidateTags();
  return useMutation({
    mutationFn: (input: { name: string; color: TagColorId }) => createTag(input),
    onSuccess: invalidate,
  });
}

export function useUpdateTag() {
  const invalidate = useInvalidateTags();
  return useMutation({
    mutationFn: ({ id, ...input }: { id: string; name?: string; color?: TagColorId }) => updateTag(id, input),
    onSuccess: invalidate,
  });
}

export function useDeleteTag() {
  const invalidate = useInvalidateTags();
  return useMutation({
    mutationFn: (id: string) => deleteTag(id),
    onSuccess: invalidate,
  });
}

export function useSetEntityTags() {
  const invalidate = useInvalidateTags();
  return useMutation({
    mutationFn: ({
      entityType,
      entityId,
      tagIds,
    }: {
      entityType: TaggableEntityType;
      entityId: string;
      tagIds: string[];
    }) => setEntityTags(entityType, entityId, tagIds),
    onSuccess: invalidate,
  });
}

export function useBulkEditTags() {
  const invalidate = useInvalidateTags();
  return useMutation({
    mutationFn: ({
      entityType,
      entityIds,
      toAdd,
      toRemove,
    }: {
      entityType: TaggableEntityType;
      entityIds: string[];
      toAdd: string[];
      toRemove: string[];
    }) => bulkEditTags(entityType, entityIds, toAdd, toRemove),
    onSuccess: invalidate,
  });
}

import { create } from "zustand";

import { genId } from "@/lib/id";
import { POSTLANDINGS, type Postlanding, type PostlandingStatus } from "@/lib/mock/postlandings";

export type PostlandingInput = {
  name: string;
  url: string;
  events: Postlanding["events"];
  status: PostlandingStatus;
};

type PostlandingsState = {
  postlandings: Postlanding[];
  getById: (id: string) => Postlanding | undefined;
  addPostlanding: (input: PostlandingInput) => string;
  updatePostlanding: (id: string, input: PostlandingInput) => void;
  setStatus: (id: string, status: PostlandingStatus) => void;
  duplicatePostlanding: (id: string) => string | undefined;
};

export const usePostlandingsStore = create<PostlandingsState>()((set, get) => ({
  postlandings: [...POSTLANDINGS],

  getById: (id) => get().postlandings.find((p) => p.id === id),

  addPostlanding: (input) => {
    const id = genId();
    const now = new Date().toISOString();
    const postlanding: Postlanding = { id, ...input, createdAt: now, updatedAt: now };
    set((s) => ({ postlandings: [postlanding, ...s.postlandings] }));
    return id;
  },

  updatePostlanding: (id, input) => {
    set((s) => ({
      postlandings: s.postlandings.map((p) =>
        p.id === id ? { ...p, ...input, updatedAt: new Date().toISOString() } : p,
      ),
    }));
  },

  setStatus: (id, status) => {
    set((s) => ({
      postlandings: s.postlandings.map((p) =>
        p.id === id ? { ...p, status, updatedAt: new Date().toISOString() } : p,
      ),
    }));
  },

  duplicatePostlanding: (id) => {
    const source = get().postlandings.find((p) => p.id === id);
    if (!source) return undefined;
    const newId = genId();
    const now = new Date().toISOString();
    const copy: Postlanding = { ...source, id: newId, name: `${source.name} (Copy)`, createdAt: now, updatedAt: now };
    set((s) => ({ postlandings: [copy, ...s.postlandings] }));
    return newId;
  },
}));

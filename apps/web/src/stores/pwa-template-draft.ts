import { create } from "zustand";

/** A PWA template on its way from the content gallery to the new-PWA form
 * (contentgallery "use", kind "pwa"): the form's fields, images already in
 * the team library. In memory only — the form copies it into its own state
 * and clears it, so a reload or a later visit starts from a blank form. */
export type PwaTemplateDraft = { templateName: string; fields: Record<string, unknown> };

type PwaTemplateDraftState = {
  draft: PwaTemplateDraft | null;
  set: (draft: PwaTemplateDraft) => void;
  clear: () => void;
};

export const usePwaTemplateDraft = create<PwaTemplateDraftState>()((set) => ({
  draft: null,
  set: (draft) => set({ draft }),
  clear: () => set({ draft: null }),
}));

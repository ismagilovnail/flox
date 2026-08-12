import { create } from "zustand";

import { generateConversions, type Conversion } from "@/lib/mock/conversions";

type ConversionsState = {
  conversions: Conversion[];
  getById: (id: string) => Conversion | undefined;
  /** Mock resend — a real retry is Phase 24 (Postback Engine); this just reflects intent in the UI. */
  resendPostback: (id: string) => void;
};

export const useConversionsStore = create<ConversionsState>()((set, get) => ({
  conversions: generateConversions(),

  getById: (id) => get().conversions.find((c) => c.id === id),

  resendPostback: (id) => {
    set((s) => ({
      conversions: s.conversions.map((c) => (c.id === id ? { ...c, postbackStatus: "sent" } : c)),
    }));
  },
}));

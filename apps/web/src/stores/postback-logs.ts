import { create } from "zustand";

import { genId } from "@/lib/id";
import { generatePostbackLogs, type PostbackLog } from "@/lib/mock/postback-logs";

type PostbackLogsState = {
  logs: PostbackLog[];
  getById: (id: string) => PostbackLog | undefined;
  /** Mock replay — appends a fresh attempt at the top, per §45's "log every postback with replay ability". */
  replay: (id: string) => void;
};

export const usePostbackLogsStore = create<PostbackLogsState>()((set, get) => ({
  logs: generatePostbackLogs(),

  getById: (id) => get().logs.find((l) => l.id === id),

  replay: (id) => {
    const source = get().logs.find((l) => l.id === id);
    if (!source) return;
    const replayed: PostbackLog = {
      ...source,
      id: genId(),
      result: "success",
      message: `Replayed — ${source.direction === "incoming" ? "conversion re-recorded" : "redelivered"}`,
      createdAt: new Date().toISOString(),
    };
    set((s) => ({ logs: [replayed, ...s.logs] }));
  },
}));

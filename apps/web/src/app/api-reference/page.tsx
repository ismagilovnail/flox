import type { Metadata } from "next";

import { ApiReferenceView } from "@/features/api-reference/api-reference-view";

export const metadata: Metadata = { title: "API reference — FLOX" };

/** Public: the reference needs no session (proxy.ts PUBLIC_PATHS). */
export default function Page() {
  return <ApiReferenceView />;
}

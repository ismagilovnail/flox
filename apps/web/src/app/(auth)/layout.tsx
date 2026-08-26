import type { ReactNode } from "react";

import { Caption } from "@/components/ui/typography";

/** Deliberately no Sidebar/Topbar (the (app) group's shell) — nothing to
 * navigate to before a session exists. proxy.ts redirects an already-
 * signed-in visitor away from these routes back into (app), so this
 * layout only ever renders for someone who isn't authenticated yet. */
export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-8 p-6">
      <Caption className="uppercase tracking-widest text-muted-foreground">FLOX</Caption>
      <div className="w-full max-w-sm">{children}</div>
    </div>
  );
}

"use client";

import * as React from "react";
import { useRouter } from "next/navigation";

import { useMe } from "@/hooks/use-auth";
import { LoadingState } from "@/components/ui/loading-state";

/**
 * Client-side defense in depth behind proxy.ts's cookie-presence redirect
 * (fast, but doesn't validate the cookie — see proxy.ts's own doc
 * comment). This is what actually calls GET /auth/me, so a *present but
 * expired/revoked* session cookie (suspended/removed mid-session, or just
 * past its 30-day expiry) gets caught here instead of leaving a signed-
 * out visitor staring at an app shell whose every data fetch silently
 * 401s. `me.data === null` is a normal, expected outcome (see useMe's own
 * doc comment) — not an error state to render.
 */
export function AuthGuard({ children }: { children: React.ReactNode }) {
  const me = useMe();
  const router = useRouter();

  React.useEffect(() => {
    if (me.data === null) router.replace("/login");
  }, [me.data, router]);

  if (me.isLoading) {
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <LoadingState />
      </div>
    );
  }

  // Redirecting (effect above) — render nothing rather than a flash of
  // protected content the visitor isn't actually authorized to see.
  if (!me.data) return null;

  return <>{children}</>;
}

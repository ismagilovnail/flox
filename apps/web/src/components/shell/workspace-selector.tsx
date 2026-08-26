"use client";

import { cn } from "@/lib/utils";
import { useMe } from "@/hooks/use-auth";

/**
 * Real workspace name (Phase 28B) — no dropdown of other workspaces
 * anymore: a session is scoped to exactly one organization
 * (apps/internal/tenant, §36-TENANCY), and nothing in this backend
 * supports switching organizations mid-session yet (see docs/auth.md's
 * "Known limitations" — a user invited into a second org logs into
 * either one by choosing at login time, not by switching after the
 * fact). This is a read-only display now, not a stand-in for a feature
 * that doesn't exist server-side.
 */
export function WorkspaceSelector({ collapsed }: { collapsed?: boolean }) {
  const me = useMe();
  if (!me.data) return null;
  const name = me.data.organization.name;

  return (
    <div className={cn("flex h-9 w-full items-center gap-2 px-2", collapsed && "justify-center px-0")}>
      <span className="flex size-5 shrink-0 items-center justify-center rounded-md bg-primary text-[0.6875rem] font-semibold text-primary-foreground">
        {name.slice(0, 1).toUpperCase()}
      </span>
      {!collapsed && <span className="flex-1 truncate text-left text-sm font-medium">{name}</span>}
    </div>
  );
}

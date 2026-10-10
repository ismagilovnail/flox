"use client";

import * as React from "react";
import { LockIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { usePermission } from "@/hooks/use-auth";

/** "Your role does not include this section" — a section's whole page for
 * a member whose role lacks its read permission. The API refuses the data
 * anyway (403); this says why instead of an error. */
export function NoAccess() {
  const { t } = useTranslation("common");
  return (
    <div
      data-slot="no-access"
      className="flex flex-col items-center justify-center gap-3 rounded-lg border border-border bg-muted/30 px-6 py-12 text-center"
    >
      <div className="flex size-10 items-center justify-center rounded-full bg-muted">
        <LockIcon className="size-5 text-muted-foreground" aria-hidden="true" />
      </div>
      <div className="flex flex-col gap-1">
        <p className="text-sm font-medium text-foreground">{t("states.noAccessTitle")}</p>
        <p className="max-w-sm text-sm text-muted-foreground">{t("states.noAccessDescription")}</p>
      </div>
    </div>
  );
}

/** Renders children only for a member holding `permission` (AuthGuard has
 * already loaded GET /auth/me inside the app), NoAccess otherwise. */
export function RequirePermission({ permission, children }: { permission: string; children: React.ReactNode }) {
  const allowed = usePermission(permission);
  return allowed ? <>{children}</> : <NoAccess />;
}

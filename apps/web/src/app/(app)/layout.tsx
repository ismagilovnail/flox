import type { ReactNode } from "react";

import { AuthGuard } from "@/components/shell/auth-guard";
import { Sidebar } from "@/components/shell/sidebar";
import { Topbar } from "@/components/shell/topbar";
import { DeletionBanner } from "@/components/shell/deletion-banner";
import { PlanBanner } from "@/components/shell/plan-banner";

export default function AppLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGuard>
      <div className="flex min-h-dvh">
        <Sidebar />
        <div className="flex min-w-0 flex-1 flex-col">
          <Topbar />
          <DeletionBanner />
          <PlanBanner />
          <main className="flex-1 p-6">{children}</main>
        </div>
      </div>
    </AuthGuard>
  );
}

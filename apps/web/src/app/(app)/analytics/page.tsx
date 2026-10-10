import { Suspense } from "react";

import { AnalyticsView } from "@/features/analytics/analytics-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="analytics.read">
      <Suspense>
        <AnalyticsView />
      </Suspense>
    </RequirePermission>
  );
}

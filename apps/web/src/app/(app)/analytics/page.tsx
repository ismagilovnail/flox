import { Suspense } from "react";

import { AnalyticsView } from "@/features/analytics/analytics-view";

export default function Page() {
  return (
    <Suspense>
      <AnalyticsView />
    </Suspense>
  );
}

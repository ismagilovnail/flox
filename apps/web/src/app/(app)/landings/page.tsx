import { Suspense } from "react";

import { LandingList } from "@/features/landings/landing-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="content.read">
      <Suspense>
        <LandingList />
      </Suspense>
    </RequirePermission>
  );
}

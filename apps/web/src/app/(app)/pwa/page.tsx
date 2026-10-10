import { Suspense } from "react";

import { PwaList } from "@/features/pwa/pwa-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="content.read">
      <Suspense>
        <PwaList />
      </Suspense>
    </RequirePermission>
  );
}

import { Suspense } from "react";

import { PwaList } from "@/features/pwa/pwa-list";

export default function Page() {
  return (
    <Suspense>
      <PwaList />
    </Suspense>
  );
}

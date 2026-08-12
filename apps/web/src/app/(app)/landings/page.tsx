import { Suspense } from "react";

import { LandingList } from "@/features/landings/landing-list";

export default function Page() {
  return (
    <Suspense>
      <LandingList />
    </Suspense>
  );
}

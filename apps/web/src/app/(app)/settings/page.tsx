import { Suspense } from "react";

import { SettingsView } from "@/features/settings/settings-view";

export default function Page() {
  return (
    <Suspense>
      <SettingsView />
    </Suspense>
  );
}

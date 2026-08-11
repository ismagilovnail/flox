import { HammerIcon } from "lucide-react";

import { EmptyState } from "@/components/ui/empty-state";
import { H1 } from "@/components/ui/typography";

export function PageStub({ title }: { title: string }) {
  return (
    <div className="flex flex-col gap-4">
      <H1>{title}</H1>
      <EmptyState
        icon={HammerIcon}
        title="Not built yet"
        description="This surface lands in a later phase — see ROADMAP.md for the build order."
      />
    </div>
  );
}

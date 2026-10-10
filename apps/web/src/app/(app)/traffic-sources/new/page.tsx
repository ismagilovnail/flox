import { NewSourceView } from "@/features/traffic-sources/new-source-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="source.read">
      <NewSourceView />
    </RequirePermission>
  );
}

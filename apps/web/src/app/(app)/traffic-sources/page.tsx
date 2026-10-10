import { SourceList } from "@/features/traffic-sources/source-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="source.read">
      <SourceList />
    </RequirePermission>
  );
}

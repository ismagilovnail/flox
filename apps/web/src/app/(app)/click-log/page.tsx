import { ClickLogView } from "@/features/clicklogs/click-log-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="analytics.read">
      <ClickLogView />
    </RequirePermission>
  );
}

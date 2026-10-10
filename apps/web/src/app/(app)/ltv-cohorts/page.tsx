import { LtvView } from "@/features/ltv/ltv-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="analytics.read">
      <LtvView />
    </RequirePermission>
  );
}

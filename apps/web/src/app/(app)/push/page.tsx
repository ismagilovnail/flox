import { PushView } from "@/features/push/push-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="push.read">
      <PushView />
    </RequirePermission>
  );
}

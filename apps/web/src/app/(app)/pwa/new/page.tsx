import { NewPwaView } from "@/features/pwa/new-pwa-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="content.read">
      <NewPwaView />
    </RequirePermission>
  );
}

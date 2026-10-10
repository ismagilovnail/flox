import { NewPixelView } from "@/features/pixels/new-pixel-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="content.read">
      <NewPixelView />
    </RequirePermission>
  );
}

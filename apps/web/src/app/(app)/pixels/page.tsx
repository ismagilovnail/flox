import { PixelList } from "@/features/pixels/pixel-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="content.read">
      <PixelList />
    </RequirePermission>
  );
}

import { PostbacksView } from "@/features/postbacks/postbacks-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="offer.read">
      <PostbacksView />
    </RequirePermission>
  );
}

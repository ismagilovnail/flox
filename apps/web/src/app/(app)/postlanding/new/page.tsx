import { NewPostlandingView } from "@/features/postlanding/new-postlanding-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="content.read">
      <NewPostlandingView />
    </RequirePermission>
  );
}

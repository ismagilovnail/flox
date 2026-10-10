import { NewLandingView } from "@/features/landings/new-landing-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="content.read">
      <NewLandingView />
    </RequirePermission>
  );
}

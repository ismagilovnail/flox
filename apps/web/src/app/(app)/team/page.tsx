import { TeamView } from "@/features/team/team-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="team.read">
      <TeamView />
    </RequirePermission>
  );
}

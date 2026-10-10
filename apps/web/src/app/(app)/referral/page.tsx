import { ReferralDashboard } from "@/features/referral/referral-dashboard";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="referral.read">
      <ReferralDashboard />
    </RequirePermission>
  );
}

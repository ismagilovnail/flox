import { NetworkList } from "@/features/networks/network-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="offer.read">
      <NetworkList />
    </RequirePermission>
  );
}

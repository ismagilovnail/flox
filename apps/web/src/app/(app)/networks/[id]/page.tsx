import { NetworkDetailView } from "@/features/networks/network-detail-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default async function Page(props: PageProps<"/networks/[id]">) {
  const { id } = await props.params;
  return (
    <RequirePermission permission="offer.read">
      <NetworkDetailView id={id} />
    </RequirePermission>
  );
}

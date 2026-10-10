import { DomainDetailView } from "@/features/domains/domain-detail-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default async function Page(props: PageProps<"/domains/[id]">) {
  const { id } = await props.params;
  return (
    <RequirePermission permission="campaign.read">
      <DomainDetailView id={id} />
    </RequirePermission>
  );
}

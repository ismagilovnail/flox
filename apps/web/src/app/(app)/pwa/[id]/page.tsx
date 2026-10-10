import { PwaDetailView } from "@/features/pwa/pwa-detail-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default async function Page(props: PageProps<"/pwa/[id]">) {
  const { id } = await props.params;
  return (
    <RequirePermission permission="content.read">
      <PwaDetailView id={id} />
    </RequirePermission>
  );
}

import { SourceDetailView } from "@/features/traffic-sources/source-detail-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default async function Page(props: PageProps<"/traffic-sources/[id]">) {
  const { id } = await props.params;
  return (
    <RequirePermission permission="source.read">
      <SourceDetailView id={id} />
    </RequirePermission>
  );
}

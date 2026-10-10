import { PixelDetailView } from "@/features/pixels/pixel-detail-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default async function Page(props: PageProps<"/pixels/[id]">) {
  const { id } = await props.params;
  return (
    <RequirePermission permission="content.read">
      <PixelDetailView id={id} />
    </RequirePermission>
  );
}

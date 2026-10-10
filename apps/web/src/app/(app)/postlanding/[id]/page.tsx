import { PostlandingDetailView } from "@/features/postlanding/postlanding-detail-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default async function Page(props: PageProps<"/postlanding/[id]">) {
  const { id } = await props.params;
  return (
    <RequirePermission permission="content.read">
      <PostlandingDetailView id={id} />
    </RequirePermission>
  );
}

import { LandingDetailView } from "@/features/landings/landing-detail-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default async function Page(props: PageProps<"/landings/[id]">) {
  const { id } = await props.params;
  return (
    <RequirePermission permission="content.read">
      <LandingDetailView id={id} />
    </RequirePermission>
  );
}

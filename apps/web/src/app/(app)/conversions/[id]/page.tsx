import { ConversionDetailView } from "@/features/conversions/conversion-detail-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default async function Page(props: PageProps<"/conversions/[id]">) {
  const { id } = await props.params;
  return (
    <RequirePermission permission="analytics.read">
      <ConversionDetailView id={id} />
    </RequirePermission>
  );
}

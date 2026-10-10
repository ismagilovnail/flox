import { ConversionList } from "@/features/conversions/conversion-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="analytics.read">
      <ConversionList />
    </RequirePermission>
  );
}

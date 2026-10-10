import { OfferList } from "@/features/offers/offer-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="offer.read">
      <OfferList />
    </RequirePermission>
  );
}

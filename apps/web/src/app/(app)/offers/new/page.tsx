import { NewOfferView } from "@/features/offers/new-offer-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="offer.read">
      <NewOfferView />
    </RequirePermission>
  );
}

import { OfferDetailView } from "@/features/offers/offer-detail-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default async function Page(props: PageProps<"/offers/[id]">) {
  const { id } = await props.params;
  return (
    <RequirePermission permission="offer.read">
      <OfferDetailView id={id} />
    </RequirePermission>
  );
}

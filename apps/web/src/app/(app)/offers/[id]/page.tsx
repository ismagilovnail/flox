import { OfferDetailView } from "@/features/offers/offer-detail-view";

export default async function Page(props: PageProps<"/offers/[id]">) {
  const { id } = await props.params;
  return <OfferDetailView id={id} />;
}

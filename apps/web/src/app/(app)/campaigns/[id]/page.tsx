import { CampaignDetailView } from "@/features/campaigns/campaign-detail-view";

export default async function Page(props: PageProps<"/campaigns/[id]">) {
  const { id } = await props.params;
  return <CampaignDetailView id={id} />;
}

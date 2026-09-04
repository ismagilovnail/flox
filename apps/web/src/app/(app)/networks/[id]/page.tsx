import { NetworkDetailView } from "@/features/networks/network-detail-view";

export default async function Page(props: PageProps<"/networks/[id]">) {
  const { id } = await props.params;
  return <NetworkDetailView id={id} />;
}

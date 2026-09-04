import { PwaDetailView } from "@/features/pwa/pwa-detail-view";

export default async function Page(props: PageProps<"/pwa/[id]">) {
  const { id } = await props.params;
  return <PwaDetailView id={id} />;
}

import { DomainDetailView } from "@/features/domains/domain-detail-view";

export default async function Page(props: PageProps<"/domains/[id]">) {
  const { id } = await props.params;
  return <DomainDetailView id={id} />;
}

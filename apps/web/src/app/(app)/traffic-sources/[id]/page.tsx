import { SourceDetailView } from "@/features/traffic-sources/source-detail-view";

export default async function Page(props: PageProps<"/traffic-sources/[id]">) {
  const { id } = await props.params;
  return <SourceDetailView id={id} />;
}

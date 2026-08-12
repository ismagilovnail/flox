import { ConversionDetailView } from "@/features/conversions/conversion-detail-view";

export default async function Page(props: PageProps<"/conversions/[id]">) {
  const { id } = await props.params;
  return <ConversionDetailView id={id} />;
}

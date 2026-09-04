import { PixelDetailView } from "@/features/pixels/pixel-detail-view";

export default async function Page(props: PageProps<"/pixels/[id]">) {
  const { id } = await props.params;
  return <PixelDetailView id={id} />;
}

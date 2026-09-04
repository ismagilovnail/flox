import { LandingDetailView } from "@/features/landings/landing-detail-view";

export default async function Page(props: PageProps<"/landings/[id]">) {
  const { id } = await props.params;
  return <LandingDetailView id={id} />;
}

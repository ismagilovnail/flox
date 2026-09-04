import { PostlandingDetailView } from "@/features/postlanding/postlanding-detail-view";

export default async function Page(props: PageProps<"/postlanding/[id]">) {
  const { id } = await props.params;
  return <PostlandingDetailView id={id} />;
}

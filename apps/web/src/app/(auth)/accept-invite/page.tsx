import { AcceptInviteForm } from "@/features/auth/accept-invite-form";

export default async function Page(props: PageProps<"/accept-invite">) {
  const { token } = await props.searchParams;
  return <AcceptInviteForm token={typeof token === "string" ? token : ""} />;
}

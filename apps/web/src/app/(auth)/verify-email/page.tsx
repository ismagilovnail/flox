import { VerifyEmailPanel } from "@/features/auth/verify-email-panel";

export default async function Page(props: PageProps<"/verify-email">) {
  const { token } = await props.searchParams;
  return <VerifyEmailPanel token={typeof token === "string" ? token : ""} />;
}

import { redirect } from "next/navigation";

/** The app has no landing page of its own: signed-in visitors go to the
 * Overview, everyone else is sent on to /login by proxy.ts. */
export default function Home() {
  redirect("/overview");
}

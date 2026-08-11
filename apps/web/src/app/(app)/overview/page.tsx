import { DashboardView } from "@/features/dashboard/dashboard-view";
import { generateDashboardMock } from "@/lib/mock/dashboard";

export default function Page() {
  const mock = generateDashboardMock(60);
  return <DashboardView mock={mock} />;
}

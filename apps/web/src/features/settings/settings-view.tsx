"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useTranslation } from "react-i18next";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { OrganizationPanel } from "@/features/settings/organization-panel";
import { ApiKeysPanel } from "@/features/settings/api-keys-panel";
import { IntegrationsPanel } from "@/features/settings/integrations-panel";
import { SecurityPanel } from "@/features/settings/security-panel";
import { CustomMetricsList } from "@/features/custom-metrics/custom-metrics-list";
import { AlertsPanel } from "@/features/alerts/alerts-panel";

const TABS = ["organization", "api-keys", "integrations", "alerts", "security", "custom-metrics"];

export function SettingsView() {
  const { t } = useTranslation("alerts");
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const requested = searchParams.get("tab") ?? "";
  const tab = TABS.includes(requested) ? requested : "organization";

  // The tab lives in the URL so a link (the notification bell's) can open one.
  function selectTab(value: string) {
    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", value);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>

      <Tabs value={tab} onValueChange={selectTab}>
        <TabsList>
          <TabsTrigger value="organization">Organization</TabsTrigger>
          <TabsTrigger value="api-keys">API Keys</TabsTrigger>
          <TabsTrigger value="integrations">Integrations</TabsTrigger>
          <TabsTrigger value="alerts">{t("tab")}</TabsTrigger>
          <TabsTrigger value="security">Security</TabsTrigger>
          <TabsTrigger value="custom-metrics">Custom Metrics</TabsTrigger>
        </TabsList>

        <TabsContent value="organization">
          <OrganizationPanel />
        </TabsContent>
        <TabsContent value="api-keys">
          <ApiKeysPanel />
        </TabsContent>
        <TabsContent value="integrations">
          <IntegrationsPanel />
        </TabsContent>
        <TabsContent value="alerts">
          <AlertsPanel />
        </TabsContent>
        <TabsContent value="security">
          <SecurityPanel />
        </TabsContent>
        <TabsContent value="custom-metrics">
          <CustomMetricsList />
        </TabsContent>
      </Tabs>
    </div>
  );
}

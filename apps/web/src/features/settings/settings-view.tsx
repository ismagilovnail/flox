"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useTranslation } from "react-i18next";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { usePermission } from "@/hooks/use-auth";
import { OrganizationPanel } from "@/features/settings/organization-panel";
import { ApiKeysPanel } from "@/features/settings/api-keys-panel";
import { IntegrationsPanel } from "@/features/settings/integrations-panel";
import { SecurityPanel } from "@/features/settings/security-panel";
import { CustomMetricsList } from "@/features/custom-metrics/custom-metrics-list";
import { AlertsPanel } from "@/features/alerts/alerts-panel";

const TABS = ["organization", "api-keys", "integrations", "alerts", "security", "custom-metrics"];
/** Tabs whose data the API serves only with settings.write. */
const SETTINGS_WRITE_TABS = ["api-keys", "integrations", "alerts"];

export function SettingsView() {
  const { t } = useTranslation(["settings", "alerts"]);
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const canManage = usePermission("settings.write");
  const tabs = canManage ? TABS : TABS.filter((tab) => !SETTINGS_WRITE_TABS.includes(tab));
  const requested = searchParams.get("tab") ?? "";
  const tab = tabs.includes(requested) ? requested : "organization";

  // The tab lives in the URL so a link (the notification bell's) can open one.
  function selectTab(value: string) {
    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", value);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>

      <Tabs value={tab} onValueChange={selectTab}>
        <TabsList className="h-auto flex-wrap">
          <TabsTrigger value="organization">{t("tabs.organization")}</TabsTrigger>
          {canManage && <TabsTrigger value="api-keys">{t("tabs.apiKeys")}</TabsTrigger>}
          {canManage && <TabsTrigger value="integrations">{t("tabs.integrations")}</TabsTrigger>}
          {canManage && <TabsTrigger value="alerts">{t("alerts:tab")}</TabsTrigger>}
          <TabsTrigger value="security">{t("tabs.security")}</TabsTrigger>
          <TabsTrigger value="custom-metrics">{t("tabs.customMetrics")}</TabsTrigger>
        </TabsList>

        <TabsContent value="organization">
          <OrganizationPanel />
        </TabsContent>
        {canManage && (
          <>
            <TabsContent value="api-keys">
              <ApiKeysPanel />
            </TabsContent>
            <TabsContent value="integrations">
              <IntegrationsPanel />
            </TabsContent>
            <TabsContent value="alerts">
              <AlertsPanel />
            </TabsContent>
          </>
        )}
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

"use client";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { OrganizationPanel } from "@/features/settings/organization-panel";
import { ApiKeysPanel } from "@/features/settings/api-keys-panel";
import { IntegrationsPanel } from "@/features/settings/integrations-panel";
import { SecurityPanel } from "@/features/settings/security-panel";

export function SettingsView() {
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>

      <Tabs defaultValue="organization">
        <TabsList>
          <TabsTrigger value="organization">Organization</TabsTrigger>
          <TabsTrigger value="api-keys">API Keys</TabsTrigger>
          <TabsTrigger value="integrations">Integrations</TabsTrigger>
          <TabsTrigger value="security">Security</TabsTrigger>
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
        <TabsContent value="security">
          <SecurityPanel />
        </TabsContent>
      </Tabs>
    </div>
  );
}

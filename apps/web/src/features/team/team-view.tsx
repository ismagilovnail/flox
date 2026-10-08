"use client";

import { useTranslation } from "react-i18next";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { MemberList } from "@/features/team/member-list";
import { RolesPermissionsPanel } from "@/features/team/roles-permissions-panel";
import { ActivityPanel } from "@/features/team/activity-panel";

export function TeamView() {
  const { t } = useTranslation("team");
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>

      <Tabs defaultValue="members">
        <TabsList>
          <TabsTrigger value="members">{t("tabs.members")}</TabsTrigger>
          <TabsTrigger value="roles">{t("tabs.roles")}</TabsTrigger>
          <TabsTrigger value="activity">{t("tabs.activity")}</TabsTrigger>
        </TabsList>

        <TabsContent value="members">
          <MemberList />
        </TabsContent>
        <TabsContent value="roles">
          <RolesPermissionsPanel />
        </TabsContent>
        <TabsContent value="activity">
          <ActivityPanel />
        </TabsContent>
      </Tabs>
    </div>
  );
}

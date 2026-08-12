"use client";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { MemberList } from "@/features/team/member-list";
import { RolesPermissionsPanel } from "@/features/team/roles-permissions-panel";
import { ActivityPanel } from "@/features/team/activity-panel";

export function TeamView() {
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">Team</h1>

      <Tabs defaultValue="members">
        <TabsList>
          <TabsTrigger value="members">Members</TabsTrigger>
          <TabsTrigger value="roles">Roles & Permissions</TabsTrigger>
          <TabsTrigger value="activity">Activity</TabsTrigger>
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

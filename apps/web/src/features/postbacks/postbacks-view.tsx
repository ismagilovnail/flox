"use client";

import { useTranslation } from "react-i18next";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { OutgoingPostbacksPanel } from "@/features/postbacks/outgoing-postbacks-panel";
import { IncomingPostbacksPanel } from "@/features/postbacks/incoming-postbacks-panel";
import { EventMappingPanel } from "@/features/postbacks/event-mapping-panel";
import { PostbackLogsPanel } from "@/features/postbacks/postback-logs-panel";

export function PostbacksView() {
  const { t } = useTranslation("postbacks");
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">{t("view.title")}</h1>

      <Tabs defaultValue="outgoing">
        <TabsList>
          <TabsTrigger value="outgoing">{t("view.tabs.outgoing")}</TabsTrigger>
          <TabsTrigger value="incoming">{t("view.tabs.incoming")}</TabsTrigger>
          <TabsTrigger value="mapping">{t("view.tabs.mapping")}</TabsTrigger>
          <TabsTrigger value="logs">{t("view.tabs.logs")}</TabsTrigger>
        </TabsList>

        <TabsContent value="outgoing">
          <OutgoingPostbacksPanel />
        </TabsContent>
        <TabsContent value="incoming">
          <IncomingPostbacksPanel />
        </TabsContent>
        <TabsContent value="mapping">
          <EventMappingPanel />
        </TabsContent>
        <TabsContent value="logs">
          <PostbackLogsPanel />
        </TabsContent>
      </Tabs>
    </div>
  );
}

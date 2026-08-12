"use client";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { OutgoingPostbacksPanel } from "@/features/postbacks/outgoing-postbacks-panel";
import { IncomingPostbacksPanel } from "@/features/postbacks/incoming-postbacks-panel";
import { EventMappingPanel } from "@/features/postbacks/event-mapping-panel";
import { PostbackLogsPanel } from "@/features/postbacks/postback-logs-panel";

export function PostbacksView() {
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">Postbacks</h1>

      <Tabs defaultValue="outgoing">
        <TabsList>
          <TabsTrigger value="outgoing">Outgoing</TabsTrigger>
          <TabsTrigger value="incoming">Incoming</TabsTrigger>
          <TabsTrigger value="mapping">Event Mapping</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
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

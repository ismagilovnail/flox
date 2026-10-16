"use client";

import { useSearchParams } from "next/navigation";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { OutgoingPostbacksPanel } from "@/features/postbacks/outgoing-postbacks-panel";
import { IncomingPostbacksPanel } from "@/features/postbacks/incoming-postbacks-panel";
import { EventMappingPanel } from "@/features/postbacks/event-mapping-panel";
import { PostbackLogsPanel } from "@/features/postbacks/postback-logs-panel";
import { useWaitingStatuses } from "@/hooks/use-event-mappings";

const TABS = ["outgoing", "incoming", "mapping", "logs"] as const;

export function PostbacksView() {
  const { t } = useTranslation("postbacks");
  // ?tab=mapping: the dashboard's "waiting for a mapping" notice links here.
  const tab = useSearchParams().get("tab");
  const initialTab = TABS.find((x) => x === tab) ?? "outgoing";
  const waiting = useWaitingStatuses().data?.statuses.length ?? 0;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">{t("view.title")}</h1>

      <Tabs defaultValue={initialTab}>
        <TabsList>
          <TabsTrigger value="outgoing">{t("view.tabs.outgoing")}</TabsTrigger>
          <TabsTrigger value="incoming">{t("view.tabs.incoming")}</TabsTrigger>
          <TabsTrigger value="mapping">
            {t("view.tabs.mapping")}
            {waiting > 0 && (
              <Badge variant="warning" className="ml-1.5 tabular-nums" aria-label={t("view.waitingBadgeAria", { count: waiting })}>
                {waiting}
              </Badge>
            )}
          </TabsTrigger>
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

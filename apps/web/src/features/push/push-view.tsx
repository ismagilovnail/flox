"use client";

import * as React from "react";
import { PlusIcon, TriangleAlertIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { usePermission } from "@/hooks/use-auth";
import { usePushSummary } from "@/hooks/use-push";
import { BroadcastList } from "@/features/push/broadcast-list";
import { BroadcastFormSheet } from "@/features/push/broadcast-form-sheet";
import { SubscriberPanel } from "@/features/push/subscriber-panel";
import { PushStatsPanel } from "@/features/push/push-stats-panel";

/** The Push section (§17, docs/push.md): broadcasts sent to installed PWAs'
 * subscribers (now or scheduled), what they did (Statistics), and who
 * those subscribers are. */
export function PushView() {
  const { t } = useTranslation("push");
  const canWrite = usePermission("push.write");
  const summary = usePushSummary();
  const [composing, setComposing] = React.useState(false);
  const configured = summary.data?.configured ?? true;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>
        {canWrite && (
          <Button onClick={() => setComposing(true)} disabled={!configured}>
            <PlusIcon className="size-4" />
            {t("broadcasts.newButton")}
          </Button>
        )}
      </div>

      {!configured && (
        <Alert>
          <TriangleAlertIcon className="size-4 text-warning" />
          <AlertDescription>{t("notConfigured")}</AlertDescription>
        </Alert>
      )}

      <Tabs defaultValue="broadcasts">
        <TabsList>
          <TabsTrigger value="broadcasts">{t("tabs.broadcasts")}</TabsTrigger>
          <TabsTrigger value="stats">{t("tabs.stats")}</TabsTrigger>
          <TabsTrigger value="subscribers">{t("tabs.subscribers")}</TabsTrigger>
        </TabsList>
        <TabsContent value="broadcasts" className="mt-4">
          <BroadcastList />
        </TabsContent>
        <TabsContent value="stats" className="mt-4">
          <PushStatsPanel />
        </TabsContent>
        <TabsContent value="subscribers" className="mt-4">
          <SubscriberPanel summary={summary} />
        </TabsContent>
      </Tabs>

      {canWrite && <BroadcastFormSheet open={composing} onOpenChange={setComposing} />}
    </div>
  );
}

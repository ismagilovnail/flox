"use client";

import { useTranslation } from "react-i18next";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Card, CardContent } from "@/components/ui/card";
import { Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useTeamActivity } from "@/hooks/use-team";
import { ApiError } from "@/lib/api/client";

function initials(name: string) {
  return name
    .split(" ")
    .map((p) => p[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
}

/** A named action ("team.invited", "api_key.rotated") is a translation
 * under team:activity.actions, with "." replaced by "_". Phrased without
 * the actor ("Member invited"), who is shown under it, so the sentence
 * needs no grammatical gender. */
function actionKey(action: string) {
  return `activity.actions.${action.replace(".", "_")}`;
}

/** An unnamed change is recorded as "METHOD /route" (apps/internal/audit). */
const ROUTE_ACTION = /^(POST|PUT|PATCH|DELETE) (\/\S*)$/;

export function ActivityPanel() {
  const { t } = useTranslation("team");
  const activityQuery = useTeamActivity();

  if (activityQuery.isLoading) return <LoadingState label={t("activity.loading")} />;
  if (activityQuery.isError) {
    return (
      <ErrorState
        title={t("activity.loadError")}
        description={activityQuery.error instanceof ApiError ? activityQuery.error.message : undefined}
        onRetry={() => activityQuery.refetch()}
      />
    );
  }

  const entries = activityQuery.data ?? [];

  if (entries.length === 0) {
    return (
      <Card>
        <CardContent className="p-6 text-center text-sm text-muted-foreground">{t("activity.empty")}</CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="flex flex-col divide-y divide-border p-0">
        {entries.map((entry) => (
          <div key={entry.id} className="flex items-start gap-3 px-4 py-3">
            <Avatar size="sm" className="mt-0.5">
              <AvatarFallback>{entry.actorName ? initials(entry.actorName) : "?"}</AvatarFallback>
            </Avatar>
            <div className="flex min-w-0 flex-1 flex-col">
              <ActionLabel action={entry.action} entityId={entry.entityId} />
              <span className="text-xs text-muted-foreground">
                {entry.actorName ?? t("activity.someone")}
                {entry.apiKeyName && <> · {t("activity.viaApiKey", { name: entry.apiKeyName })}</>} ·{" "}
                <RelativeTime date={entry.createdAt} />
              </span>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function ActionLabel({ action, entityId }: { action: string; entityId: string }) {
  const { t } = useTranslation("team");
  const route = ROUTE_ACTION.exec(action);
  if (!route) {
    return <p className="text-sm text-foreground">{t(actionKey(action), { defaultValue: action })}</p>;
  }
  return (
    <p className="flex min-w-0 flex-wrap items-baseline gap-x-1.5 text-sm text-foreground">
      <span>{t(`activity.methods.${route[1]}`)}</span>
      <Mono className="truncate text-xs text-muted-foreground">{route[2]}</Mono>
      {entityId && <Mono className="truncate text-xs text-muted-foreground">{entityId}</Mono>}
    </p>
  );
}

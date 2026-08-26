"use client";

import { formatDistanceToNow } from "date-fns";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Card, CardContent } from "@/components/ui/card";
import { Caption } from "@/components/ui/typography";
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

/** apps/internal/auth writes a fixed, small set of action keys (see
 * docs/auth.md) — mapped here to a short sentence fragment, rendered
 * after the actor's name ("Owner Curl invited a new member"). */
const ACTION_LABELS: Record<string, string> = {
  "team.invited": "invited a new member",
  "team.invite_resent": "resent an invite",
  "team.invite_accepted": "accepted their invite",
  "team.role_changed": "changed a member's role",
  "team.suspended": "suspended a member",
  "team.reactivated": "reactivated a member",
  "team.removed": "removed a member",
};

export function ActivityPanel() {
  const activityQuery = useTeamActivity();

  if (activityQuery.isLoading) return <LoadingState />;
  if (activityQuery.isError) {
    return (
      <ErrorState
        description={activityQuery.error instanceof ApiError ? activityQuery.error.message : undefined}
        onRetry={() => activityQuery.refetch()}
      />
    );
  }

  const entries = activityQuery.data ?? [];

  if (entries.length === 0) {
    return (
      <Card>
        <CardContent className="p-6 text-center text-sm text-muted-foreground">No activity yet.</CardContent>
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
              <p className="text-sm">
                <span className="font-medium text-foreground">{entry.actorName ?? "Someone"}</span>{" "}
                <span className="text-muted-foreground">{ACTION_LABELS[entry.action] ?? entry.action}</span>
              </p>
              <Caption>{formatDistanceToNow(new Date(entry.createdAt), { addSuffix: true })}</Caption>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

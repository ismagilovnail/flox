"use client";

import * as React from "react";
import { formatDistanceToNow } from "date-fns";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Card, CardContent } from "@/components/ui/card";
import { Caption } from "@/components/ui/typography";
import { useTeamStore } from "@/stores/team";
import { TEAM_ACTIVITY } from "@/lib/mock/team";

function initials(name: string) {
  return name
    .split(" ")
    .map((p) => p[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
}

export function ActivityPanel() {
  const members = useTeamStore((s) => s.members);

  const memberById = React.useMemo(() => Object.fromEntries(members.map((m) => [m.id, m])), [members]);

  const entries = React.useMemo(
    () => [...TEAM_ACTIVITY].sort((a, b) => b.createdAt.localeCompare(a.createdAt)),
    [],
  );

  return (
    <Card>
      <CardContent className="flex flex-col divide-y divide-border p-0">
        {entries.map((entry) => {
          const member = memberById[entry.memberId];
          return (
            <div key={entry.id} className="flex items-start gap-3 px-4 py-3">
              <Avatar size="sm" className="mt-0.5">
                <AvatarFallback>{member ? initials(member.name) : "?"}</AvatarFallback>
              </Avatar>
              <div className="flex min-w-0 flex-1 flex-col">
                <p className="text-sm">
                  <span className="font-medium text-foreground">{member?.name ?? "Unknown member"}</span>{" "}
                  <span className="text-muted-foreground">{entry.action}</span>
                </p>
                <Caption>{formatDistanceToNow(new Date(entry.createdAt), { addSuffix: true })}</Caption>
              </div>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}

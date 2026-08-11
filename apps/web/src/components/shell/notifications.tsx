"use client";

import * as React from "react";
import { BellIcon } from "lucide-react";

import { cn } from "@/lib/utils";
import { IconButton } from "@/components/ui/icon-button";
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Caption } from "@/components/ui/typography";

const NOTIFICATIONS = [
  {
    id: "n1",
    title: "Postback dedup collision",
    description: "click_id=9f21, status=ACCEPT already recorded.",
    time: "2m ago",
    unread: true,
  },
  {
    id: "n2",
    title: "Campaign paused",
    description: "US Sweeps — FB hit its daily budget cap.",
    time: "1h ago",
    unread: true,
  },
  {
    id: "n3",
    title: "Domain SSL renewed",
    description: "track.floxlink.io certificate renewed automatically.",
    time: "Yesterday",
    unread: false,
  },
];

export function Notifications() {
  const unreadCount = NOTIFICATIONS.filter((n) => n.unread).length;

  return (
    <Popover>
      <PopoverTrigger asChild>
        <IconButton aria-label="Notifications" variant="ghost" className="relative">
          <BellIcon className="size-4" />
          {unreadCount > 0 && (
            <span className="absolute right-1 top-1 flex size-1.5 rounded-full bg-info" />
          )}
        </IconButton>
      </PopoverTrigger>
      <PopoverContent className="w-80" align="end">
        <PopoverHeader>
          <PopoverTitle>Notifications</PopoverTitle>
        </PopoverHeader>
        <div className="flex flex-col gap-1">
          {NOTIFICATIONS.map((n) => (
            <div
              key={n.id}
              className={cn(
                "flex flex-col gap-0.5 rounded-md p-2 hover:bg-muted",
                n.unread && "bg-accent/40",
              )}
            >
              <div className="flex items-center justify-between gap-2">
                <span className="text-sm font-medium text-foreground">
                  {n.title}
                </span>
                <Caption className="shrink-0">{n.time}</Caption>
              </div>
              <Caption>{n.description}</Caption>
            </div>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}

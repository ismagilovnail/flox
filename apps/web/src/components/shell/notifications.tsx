"use client";

import * as React from "react";
import Link from "next/link";
import { BellIcon, SettingsIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Popover, PopoverContent, PopoverHeader, PopoverTitle, PopoverTrigger } from "@/components/ui/popover";
import { Caption } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useMarkNotificationsRead, useNotifications } from "@/hooks/use-alerts";
import { notificationHref } from "@/lib/alerts";
import { useNotificationText } from "@/hooks/use-notification-text";

export function Notifications() {
  const { t } = useTranslation("alerts");
  const [open, setOpen] = React.useState(false);
  const query = useNotifications();
  const markRead = useMarkNotificationsRead();
  const text = useNotificationText();
  const unread = query.data?.unread ?? 0;
  const items = query.data?.notifications ?? [];

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <IconButton
          aria-label={unread > 0 ? t("bell.ariaUnread", { count: unread }) : t("bell.aria")}
          variant="ghost"
          className="relative"
        >
          <BellIcon className="size-4" />
          {unread > 0 && (
            <span className="absolute right-0.5 top-0.5 flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-danger px-0.5 text-[9px] font-semibold leading-none text-white tabular-nums">
              {unread > 9 ? "9+" : unread}
            </span>
          )}
        </IconButton>
      </PopoverTrigger>
      <PopoverContent className="w-96 max-w-[calc(100vw-2rem)]" align="end">
        <PopoverHeader className="flex flex-row items-center justify-between gap-2">
          <PopoverTitle>{t("bell.title")}</PopoverTitle>
          {unread > 0 && (
            <Button size="sm" variant="ghost" disabled={markRead.isPending} onClick={() => markRead.mutate()}>
              {t("bell.markRead")}
            </Button>
          )}
        </PopoverHeader>
        <div className="flex max-h-[60vh] flex-col gap-1 overflow-y-auto">
          {query.isPending && <LoadingState label={t("bell.title")} />}
          {query.isError && (
            <ErrorState title={t("bell.error")} description={query.error.message} onRetry={() => query.refetch()} />
          )}
          {query.isSuccess && items.length === 0 && <Caption className="p-2">{t("bell.empty")}</Caption>}
          {items.map((n) => {
            const { title, body } = text(n);
            const href = notificationHref(n);
            const content = (
              <>
                <div className="flex items-center justify-between gap-2">
                  <span className="flex items-center gap-1.5 text-sm font-medium text-foreground">
                    {!n.read && <span className="size-1.5 shrink-0 rounded-full bg-info" aria-hidden />}
                    {title}
                  </span>
                  <RelativeTime date={n.createdAt} className="shrink-0" />
                </div>
                <Caption>{body}</Caption>
              </>
            );
            const className = cn("flex flex-col gap-0.5 rounded-md p-2 hover:bg-muted", !n.read && "bg-accent/40");
            return href ? (
              <Link key={n.id} href={href} className={className} onClick={() => setOpen(false)}>
                {content}
              </Link>
            ) : (
              <div key={n.id} className={className}>
                {content}
              </div>
            );
          })}
        </div>
        <Link
          href="/settings?tab=alerts"
          onClick={() => setOpen(false)}
          className="mt-1 flex items-center gap-1.5 border-t border-border px-2 pt-2 text-xs text-muted-foreground hover:text-foreground"
        >
          <SettingsIcon className="size-3.5" />
          {t("bell.settings")}
        </Link>
      </PopoverContent>
    </Popover>
  );
}

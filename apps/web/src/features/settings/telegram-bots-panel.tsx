"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { CopyIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ErrorState } from "@/components/ui/error-state";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/ui/loading-state";
import { Textarea } from "@/components/ui/textarea";
import { Mono } from "@/components/ui/typography";
import { useConnectTelegramBot, useDeleteTelegramBot, useTelegramBots } from "@/hooks/use-telegram";
import { ApiError } from "@/lib/api/client";
import type { TelegramBot } from "@/lib/api/telegram";

/** Mirrors telegram.Service's validation (internal/telegram/service.go). */
function connectSchema(t: TFunction<"settings">) {
  return z.object({
    token: z.string().trim().regex(/^\d{5,15}:[A-Za-z0-9_-]{30,50}$/, t("telegram.errors.token")),
    channel: z
      .string()
      .trim()
      .refine((v) => v === "" || /^(@[A-Za-z][A-Za-z0-9_]{4,31}|-100\d{5,15})$/.test(v), t("telegram.errors.channel")),
    welcomeText: z.string().trim().max(2000, t("telegram.errors.welcomeText")),
  });
}

type ConnectValues = z.infer<ReturnType<typeof connectSchema>>;

function copy(text: string, done: string) {
  navigator.clipboard.writeText(text).then(
    () => toast(done, { description: text }),
    () => toast.error(text),
  );
}

/** Settings → Integrations → Telegram bots (docs/telegram.md): connect an
 * organization's own bot (and its channel) so visits that start the bot
 * record TG_START and joins of its channel TG_JOIN. */
export function TelegramBotsPanel() {
  const { t } = useTranslation(["settings", "common"]);
  const query = useTelegramBots();
  const connect = useConnectTelegramBot();
  const remove = useDeleteTelegramBot();
  const [connecting, setConnecting] = React.useState(false);
  const [removing, setRemoving] = React.useState<TelegramBot | null>(null);
  const form = useForm<ConnectValues>({ resolver: zodResolver(connectSchema(t)), defaultValues: { token: "", channel: "", welcomeText: "" } });
  const configured = query.data?.configured ?? true;

  function handleConnect(values: ConnectValues) {
    connect.mutate(values, {
      onSuccess: (bot) => {
        setConnecting(false);
        form.reset();
        toast(t("telegram.connected"), { description: "@" + bot.username });
      },
      onError: (err) => {
        if (err instanceof ApiError && err.fields) {
          for (const [field, message] of Object.entries(err.fields)) {
            if (field === "token" || field === "channel" || field === "welcomeText") form.setError(field, { message });
          }
          return;
        }
        toast.error(t("telegram.connectError"), { description: err.message });
      },
    });
  }

  function handleRemove() {
    if (!removing) return;
    remove.mutate(removing.id, {
      onSuccess: () => toast(t("telegram.removed"), { description: "@" + removing.username }),
      onError: (err) => toast.error(t("telegram.removeError"), { description: err.message }),
      onSettled: () => setRemoving(null),
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("telegram.title")}</CardTitle>
        <CardDescription>{t("telegram.description")}</CardDescription>
        <CardAction>
          <Button size="sm" onClick={() => setConnecting(true)} disabled={!configured}>
            <PlusIcon className="size-4" /> {t("telegram.connect")}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col divide-y divide-border p-0">
        {!configured && <p className="px-4 py-3 text-sm text-warning">{t("telegram.notConfigured")}</p>}
        {query.isPending && (
          <div className="px-4 py-6">
            <LoadingState label={t("telegram.loading")} />
          </div>
        )}
        {query.isError && (
          <div className="px-4 py-6">
            <ErrorState title={t("telegram.loadError")} description={query.error.message} onRetry={() => query.refetch()} />
          </div>
        )}
        {query.isSuccess && query.data.bots.length === 0 && <p className="px-4 py-6 text-sm text-muted-foreground">{t("telegram.empty")}</p>}
        {query.data?.bots.map((bot) => (
          <div key={bot.id} className="flex items-start gap-3 px-4 py-3">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <span className="text-sm font-medium">@{bot.username}</span>
              <span className="text-xs text-muted-foreground">
                {bot.channel ? t("telegram.channel", { title: bot.channel.title }) : t("telegram.noChannel")}
              </span>
              <div className="flex min-w-0 items-center gap-1">
                <Mono className="truncate text-xs">{bot.deepLink}</Mono>
                <IconButton
                  aria-label={t("telegram.copyDeepLink")}
                  size="icon-sm"
                  variant="ghost"
                  onClick={() => copy(bot.deepLink, t("telegram.copied"))}
                >
                  <CopyIcon className="size-3.5" />
                </IconButton>
              </div>
              {bot.clickInviteLink && (
                <div className="flex min-w-0 flex-col gap-0.5">
                  <span className="text-xs text-muted-foreground">{t("telegram.clickInviteLink")}</span>
                  <div className="flex min-w-0 items-center gap-1">
                    <Mono className="truncate text-xs">{bot.clickInviteLink}</Mono>
                    <IconButton
                      aria-label={t("telegram.copyClickInviteLink")}
                      size="icon-sm"
                      variant="ghost"
                      onClick={() => copy(bot.clickInviteLink!, t("telegram.copied"))}
                    >
                      <CopyIcon className="size-3.5" />
                    </IconButton>
                  </div>
                  <span className={bot.freeInviteLinks === 0 ? "text-xs text-warning" : "text-xs text-muted-foreground"}>
                    <Mono className="tabular-nums">{bot.freeInviteLinks}</Mono>{" "}
                    {bot.freeInviteLinks === 0 ? t("telegram.inviteLinksEmpty") : t("telegram.inviteLinksFree")}
                  </span>
                </div>
              )}
            </div>
            <IconButton aria-label={t("telegram.removeAria", { name: bot.username })} size="icon-sm" onClick={() => setRemoving(bot)}>
              <Trash2Icon className="size-3.5" />
            </IconButton>
          </div>
        ))}
        {query.isSuccess && query.data.bots.length > 0 && (
          <div className="flex flex-col gap-1 px-4 py-3 text-xs text-muted-foreground">
            <p>{t("telegram.deepLinkHint")}</p>
            {query.data.bots.some((b) => b.clickInviteLink) && <p>{t("telegram.clickInviteLinkHint")}</p>}
          </div>
        )}
      </CardContent>

      <Dialog open={connecting} onOpenChange={(o) => !connect.isPending && setConnecting(o)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("telegram.connectTitle")}</DialogTitle>
            <DialogDescription>{t("telegram.connectDescription")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(handleConnect)} className="flex flex-col gap-4">
            <div className="grid gap-1.5">
              <Label htmlFor="tg-token">{t("telegram.token")}</Label>
              <Input id="tg-token" type="password" autoComplete="off" className="font-mono text-xs" {...form.register("token")} aria-invalid={!!form.formState.errors.token} />
              {form.formState.errors.token ? (
                <p className="text-xs text-danger">{form.formState.errors.token.message}</p>
              ) : (
                <p className="text-xs text-muted-foreground">{t("telegram.tokenHint")}</p>
              )}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="tg-channel">{t("telegram.channelLabel")}</Label>
              <Input id="tg-channel" placeholder="@channelname" className="font-mono text-xs" {...form.register("channel")} aria-invalid={!!form.formState.errors.channel} />
              {form.formState.errors.channel ? (
                <p className="text-xs text-danger">{form.formState.errors.channel.message}</p>
              ) : (
                <p className="text-xs text-muted-foreground">{t("telegram.channelHint")}</p>
              )}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="tg-welcome">{t("telegram.welcome")}</Label>
              <Textarea id="tg-welcome" rows={3} maxLength={2000} {...form.register("welcomeText")} aria-invalid={!!form.formState.errors.welcomeText} />
              {form.formState.errors.welcomeText ? (
                <p className="text-xs text-danger">{form.formState.errors.welcomeText.message}</p>
              ) : (
                <p className="text-xs text-muted-foreground">{t("telegram.welcomeHint")}</p>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setConnecting(false)} disabled={connect.isPending}>
                {t("common:actions.cancel")}
              </Button>
              <Button type="submit" disabled={connect.isPending}>
                {t("telegram.connect")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={!!removing} onOpenChange={(o) => !o && !remove.isPending && setRemoving(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("telegram.removeTitle", { name: removing?.username })}</DialogTitle>
            <DialogDescription>{t("telegram.removeDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRemoving(null)} disabled={remove.isPending}>
              {t("common:actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleRemove} disabled={remove.isPending}>
              {t("telegram.remove")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

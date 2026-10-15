"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { CopyIcon, KeyRoundIcon, PencilIcon, PlusIcon, Trash2Icon, XIcon } from "lucide-react";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Mono } from "@/components/ui/typography";
import { useAddTelegramChannel, useRemoveTelegramChannel, useUpdateTelegramBot } from "@/hooks/use-telegram";
import { ApiError } from "@/lib/api/client";
import { MAX_TELEGRAM_CHANNELS, type TelegramBot, type TelegramChannel } from "@/lib/api/telegram";

/** telegram.channelPattern: "@channelname" or the channel's id. */
export const TELEGRAM_CHANNEL_PATTERN = /^(@[A-Za-z][A-Za-z0-9_]{4,31}|-100\d{5,15})$/;
/** telegram.MaxWelcomeText. */
export const TELEGRAM_MAX_WELCOME = 2000;

function channelSchema(t: TFunction<"settings">) {
  return z.object({ channel: z.string().trim().regex(TELEGRAM_CHANNEL_PATTERN, t("telegram.errors.channel")) });
}

/** telegram.tokenPattern. */
export const TELEGRAM_TOKEN_PATTERN = /^\d{5,15}:[A-Za-z0-9_-]{30,50}$/;

function tokenSchema(t: TFunction<"settings">) {
  return z.object({ token: z.string().trim().regex(TELEGRAM_TOKEN_PATTERN, t("telegram.errors.token")) });
}

function welcomeSchema(t: TFunction<"settings">) {
  return z.object({ welcomeText: z.string().trim().max(TELEGRAM_MAX_WELCOME, t("telegram.errors.welcomeText")) });
}

export function copyText(text: string, done: string) {
  navigator.clipboard.writeText(text).then(
    () => toast(done, { description: text }),
    () => toast.error(text),
  );
}

function CopyLine({ value, label, copied }: { value: string; label: string; copied: string }) {
  return (
    <div className="flex min-w-0 items-center gap-1">
      <Mono className="truncate text-xs">{value}</Mono>
      <IconButton aria-label={label} size="icon-sm" variant="ghost" onClick={() => copyText(value, copied)}>
        <CopyIcon className="size-3.5" />
      </IconButton>
    </div>
  );
}

/** One connected bot: its link, welcome message (editable), token
 * (replaceable by a new one of the same bot) and channels (added and removed
 * here; each with its per-click link, pool and joins). joins: TG_JOIN per
 * channel id over the last 30 days — undefined while unknown. */
export function TelegramBotRow({ bot, onRemove, joins }: { bot: TelegramBot; onRemove: () => void; joins?: Map<string, number> }) {
  const { t } = useTranslation(["settings", "common"]);
  const [editing, setEditing] = React.useState(false);
  const [changingToken, setChangingToken] = React.useState(false);
  const [adding, setAdding] = React.useState(false);
  const [removing, setRemoving] = React.useState<TelegramChannel | null>(null);
  const update = useUpdateTelegramBot();
  const add = useAddTelegramChannel();
  const removeChannel = useRemoveTelegramChannel();
  const welcomeForm = useForm<{ welcomeText: string }>({ resolver: zodResolver(welcomeSchema(t)), defaultValues: { welcomeText: bot.welcomeText } });
  const channelForm = useForm<{ channel: string }>({ resolver: zodResolver(channelSchema(t)), defaultValues: { channel: "" } });
  const tokenForm = useForm<{ token: string }>({ resolver: zodResolver(tokenSchema(t)), defaultValues: { token: "" } });
  const changeToken = useUpdateTelegramBot();
  const full = bot.channels.length >= MAX_TELEGRAM_CHANNELS;

  function openEdit() {
    welcomeForm.reset({ welcomeText: bot.welcomeText });
    setEditing(true);
  }

  function handleEdit(values: { welcomeText: string }) {
    update.mutate(
      { id: bot.id, welcomeText: values.welcomeText },
      {
        onSuccess: () => {
          setEditing(false);
          toast(t("telegram.welcomeSaved"), { description: "@" + bot.username });
        },
        onError: (err) => {
          if (err instanceof ApiError && err.fields?.welcomeText) {
            welcomeForm.setError("welcomeText", { message: err.fields.welcomeText });
            return;
          }
          toast.error(t("telegram.welcomeSaveError"), { description: err.message });
        },
      },
    );
  }

  function openChangeToken() {
    tokenForm.reset({ token: "" });
    setChangingToken(true);
  }

  function handleChangeToken(values: { token: string }) {
    changeToken.mutate(
      { id: bot.id, token: values.token },
      {
        onSuccess: () => {
          setChangingToken(false);
          tokenForm.reset({ token: "" });
          toast(t("telegram.tokenChanged"), { description: "@" + bot.username });
        },
        onError: (err) => {
          if (err instanceof ApiError && err.fields?.token) {
            tokenForm.setError("token", { message: err.fields.token });
            return;
          }
          toast.error(t("telegram.tokenChangeError"), { description: err.message });
        },
      },
    );
  }

  function handleAdd(values: { channel: string }) {
    add.mutate(
      { botId: bot.id, channel: values.channel },
      {
        onSuccess: () => {
          setAdding(false);
          channelForm.reset();
          toast(t("telegram.channelAdded"), { description: values.channel });
        },
        onError: (err) => {
          if (err instanceof ApiError && err.fields?.channel) {
            channelForm.setError("channel", { message: err.fields.channel });
            return;
          }
          if (err instanceof ApiError && err.status === 409) {
            channelForm.setError("channel", { message: t("telegram.errors.channelTaken") });
            return;
          }
          toast.error(t("telegram.channelAddError"), { description: err.message });
        },
      },
    );
  }

  function handleRemoveChannel() {
    if (!removing) return;
    removeChannel.mutate(
      { botId: bot.id, channelId: removing.id },
      {
        onSuccess: () => toast(t("telegram.channelRemoved"), { description: removing.title }),
        onError: (err) => toast.error(t("telegram.channelRemoveError"), { description: err.message }),
        onSettled: () => setRemoving(null),
      },
    );
  }

  return (
    <div className="flex flex-col gap-2 px-4 py-3">
      <div className="flex items-start gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <span className="text-sm font-medium">@{bot.username}</span>
          <CopyLine value={bot.deepLink} label={t("telegram.copyDeepLink")} copied={t("telegram.copied")} />
          <p className="line-clamp-2 text-xs text-muted-foreground">
            {bot.welcomeText ? bot.welcomeText : <span className="italic">{t("telegram.noWelcome")}</span>}
          </p>
        </div>
        <IconButton aria-label={t("telegram.editWelcomeAria", { name: bot.username })} size="icon-sm" variant="ghost" onClick={openEdit}>
          <PencilIcon className="size-3.5" />
        </IconButton>
        <IconButton aria-label={t("telegram.changeTokenAria", { name: bot.username })} size="icon-sm" variant="ghost" onClick={openChangeToken}>
          <KeyRoundIcon className="size-3.5" />
        </IconButton>
        <IconButton aria-label={t("telegram.removeAria", { name: bot.username })} size="icon-sm" onClick={onRemove}>
          <Trash2Icon className="size-3.5" />
        </IconButton>
      </div>

      <div className="flex flex-col gap-1 rounded-md border border-border">
        <div className="flex items-center justify-between gap-2 px-3 py-1.5">
          <span className="text-xs font-medium text-muted-foreground">
            {t("telegram.channels")} <Mono className="tabular-nums">{bot.channels.length}/{MAX_TELEGRAM_CHANNELS}</Mono>
          </span>
          <Button size="sm" variant="ghost" onClick={() => setAdding(true)} disabled={full} title={full ? t("telegram.channelsFull") : undefined}>
            <PlusIcon className="size-3.5" /> {t("telegram.addChannel")}
          </Button>
        </div>
        {bot.channels.length === 0 && <p className="px-3 pb-2 text-xs text-muted-foreground">{t("telegram.noChannel")}</p>}
        {bot.channels.map((ch) => (
          <div key={ch.id} className="flex items-start gap-2 border-t border-border px-3 py-2">
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="text-xs font-medium">
                {ch.title || ch.inviteLink} <Mono className="text-muted-foreground">{ch.tgChatId}</Mono>
              </span>
              {joins && (
                <span className="text-xs text-muted-foreground">
                  {t("telegram.joins30d")} <Mono className="tabular-nums text-foreground">{joins.get(ch.id) ?? 0}</Mono>
                </span>
              )}
              <CopyLine value={ch.inviteLink} label={t("telegram.copyChannelLink")} copied={t("telegram.copied")} />
              {ch.clickInviteLink && (
                <>
                  <span className="text-xs text-muted-foreground">{t("telegram.clickInviteLink")}</span>
                  <CopyLine value={ch.clickInviteLink} label={t("telegram.copyClickInviteLink")} copied={t("telegram.copied")} />
                  <span className={ch.freeInviteLinks === 0 ? "text-xs text-warning" : "text-xs text-muted-foreground"}>
                    <Mono className="tabular-nums">{ch.freeInviteLinks}</Mono>{" "}
                    {ch.freeInviteLinks === 0 ? t("telegram.inviteLinksEmpty") : t("telegram.inviteLinksFree")}
                  </span>
                </>
              )}
            </div>
            <IconButton aria-label={t("telegram.removeChannelAria", { title: ch.title })} size="icon-sm" variant="ghost" onClick={() => setRemoving(ch)}>
              <XIcon className="size-3.5" />
            </IconButton>
          </div>
        ))}
      </div>

      <Dialog open={editing} onOpenChange={(o) => !update.isPending && setEditing(o)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("telegram.editWelcomeTitle", { name: bot.username })}</DialogTitle>
            <DialogDescription>{t("telegram.welcomeHint")}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-4" onSubmit={welcomeForm.handleSubmit(handleEdit)} noValidate>
            <div className="grid gap-1.5">
              <Label htmlFor={`tg-welcome-${bot.id}`}>{t("telegram.welcome")}</Label>
              <Textarea
                id={`tg-welcome-${bot.id}`}
                rows={4}
                maxLength={TELEGRAM_MAX_WELCOME}
                {...welcomeForm.register("welcomeText")}
                aria-invalid={!!welcomeForm.formState.errors.welcomeText}
              />
              {welcomeForm.formState.errors.welcomeText && (
                <p className="text-xs text-danger">{welcomeForm.formState.errors.welcomeText.message}</p>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setEditing(false)} disabled={update.isPending}>
                {t("common:actions.cancel")}
              </Button>
              <Button type="submit" disabled={update.isPending || !welcomeForm.formState.isDirty}>
                {t("common:actions.save")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={changingToken} onOpenChange={(o) => !changeToken.isPending && setChangingToken(o)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("telegram.changeTokenTitle", { name: bot.username })}</DialogTitle>
            <DialogDescription>{t("telegram.changeTokenDescription")}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-4" onSubmit={tokenForm.handleSubmit(handleChangeToken)} noValidate>
            <div className="grid gap-1.5">
              <Label htmlFor={`tg-token-${bot.id}`}>{t("telegram.newToken")}</Label>
              <Input
                id={`tg-token-${bot.id}`}
                type="password"
                autoComplete="off"
                className="font-mono text-xs"
                {...tokenForm.register("token")}
                aria-invalid={!!tokenForm.formState.errors.token}
              />
              {tokenForm.formState.errors.token ? (
                <p className="text-xs text-danger">{tokenForm.formState.errors.token.message}</p>
              ) : (
                <p className="text-xs text-muted-foreground">{t("telegram.newTokenHint")}</p>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setChangingToken(false)} disabled={changeToken.isPending}>
                {t("common:actions.cancel")}
              </Button>
              <Button type="submit" disabled={changeToken.isPending}>
                {t("telegram.changeToken")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={adding} onOpenChange={(o) => !add.isPending && setAdding(o)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("telegram.addChannelTitle", { name: bot.username })}</DialogTitle>
            <DialogDescription>{t("telegram.channelHint")}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-4" onSubmit={channelForm.handleSubmit(handleAdd)} noValidate>
            <div className="grid gap-1.5">
              <Label htmlFor={`tg-channel-${bot.id}`}>{t("telegram.channelField")}</Label>
              <Input
                id={`tg-channel-${bot.id}`}
                placeholder="@channelname"
                className="font-mono text-xs"
                {...channelForm.register("channel")}
                aria-invalid={!!channelForm.formState.errors.channel}
              />
              {channelForm.formState.errors.channel && <p className="text-xs text-danger">{channelForm.formState.errors.channel.message}</p>}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setAdding(false)} disabled={add.isPending}>
                {t("common:actions.cancel")}
              </Button>
              <Button type="submit" disabled={add.isPending}>
                {t("telegram.addChannel")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={!!removing} onOpenChange={(o) => !o && !removeChannel.isPending && setRemoving(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("telegram.removeChannelTitle", { title: removing?.title })}</DialogTitle>
            <DialogDescription>{t("telegram.removeChannelDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRemoving(null)} disabled={removeChannel.isPending}>
              {t("common:actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleRemoveChannel} disabled={removeChannel.isPending}>
              {t("telegram.removeChannel")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

import { apiFetch } from "@/lib/api/client";

/** Mirrors internal/telegram's Channel: a channel the bot administers;
 * joining any of a bot's channels is TG_JOIN (once per click). */
export type TelegramChannel = {
  id: string;
  /** Telegram's id of the channel (-100…). */
  tgChatId: number;
  title: string;
  /** The channel's shared link. */
  inviteLink: string;
  createdAt: string;
  /** On a configured deployment: destination URL sending each click to its
   * own invite link of this channel, so a join is TG_JOIN without a /start. */
  clickInviteLink?: string;
  /** Invite links left in the channel's pool (the worker keeps it topped up). */
  freeInviteLinks: number;
};

/** Mirrors internal/telegram's Bot. The token never comes back. */
export type TelegramBot = {
  id: string;
  username: string;
  welcomeText: string;
  /** Oldest first, at most MAX_TELEGRAM_CHANNELS. */
  channels: TelegramChannel[];
  createdAt: string;
  /** Destination URL to use: {click_id} makes each visit open the bot
   * with its own click (TG_START). */
  deepLink: string;
};

/** telegram.MaxChannels. */
export const MAX_TELEGRAM_CHANNELS = 10;

export type ConnectTelegramBotInput = {
  token: string;
  welcomeText: string;
  /** "@channelname" or "-100…", optional. */
  channel: string;
};

/** configured: the deployment can connect bots (TELEGRAM_WEBHOOK_BASE_URL). */
export function listTelegramBots(): Promise<{ bots: TelegramBot[]; configured: boolean }> {
  return apiFetch("/telegram-bots");
}

export function connectTelegramBot(input: ConnectTelegramBotInput): Promise<TelegramBot> {
  return apiFetch("/telegram-bots", { method: "POST", body: input });
}

export function deleteTelegramBot(id: string): Promise<void> {
  return apiFetch(`/telegram-bots/${id}`, { method: "DELETE" });
}

/** welcomeText, and/or token: a new token of the same bot (revoked and
 * reissued in @BotFather) — another bot's is a `token` field error. */
export function updateTelegramBot(id: string, input: { welcomeText?: string; token?: string }): Promise<TelegramBot> {
  return apiFetch(`/telegram-bots/${id}`, { method: "PATCH", body: input });
}

/** A channel as reports name it (the `telegram_channel` dimension's ids). */
export type TelegramChannelName = { id: string; title: string; botUsername: string };

/** GET /telegram-channels: readable with analytics.read, for report labels. */
export function listTelegramChannelNames(): Promise<{ channels: TelegramChannelName[] }> {
  return apiFetch("/telegram-channels");
}

/** channel: "@channelname" or "-100…"; Telegram checks the bot can invite. */
export function addTelegramChannel(botId: string, channel: string): Promise<TelegramBot> {
  return apiFetch(`/telegram-bots/${botId}/channels`, { method: "POST", body: { channel } });
}

export function removeTelegramChannel(botId: string, channelId: string): Promise<TelegramBot> {
  return apiFetch(`/telegram-bots/${botId}/channels/${channelId}`, { method: "DELETE" });
}

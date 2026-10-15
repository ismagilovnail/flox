import { apiFetch } from "@/lib/api/client";

/** Mirrors internal/telegram's Bot. The token never comes back. */
export type TelegramBot = {
  id: string;
  username: string;
  welcomeText: string;
  /** The channel the bot administers; joining it is TG_JOIN. */
  channel: { id: number; title: string; inviteLink: string } | null;
  createdAt: string;
  /** Destination URL to use: {click_id} makes each visit open the bot
   * with its own click (TG_START). */
  deepLink: string;
  /** With a channel: destination URL sending each click to its own invite
   * link of the channel, so a join is TG_JOIN without a /start. */
  clickInviteLink?: string;
  /** Invite links left in the pool (the worker keeps it topped up). */
  freeInviteLinks: number;
};

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

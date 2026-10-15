"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  addTelegramChannel,
  connectTelegramBot,
  deleteTelegramBot,
  listTelegramBots,
  listTelegramChannelNames,
  removeTelegramChannel,
  updateTelegramBot,
  type ConnectTelegramBotInput,
} from "@/lib/api/telegram";

const telegramKey = ["telegram-bots"] as const;

export function useTelegramBots() {
  return useQuery({ queryKey: telegramKey, queryFn: listTelegramBots });
}

export function useConnectTelegramBot() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ConnectTelegramBotInput) => connectTelegramBot(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: telegramKey }),
  });
}

export function useDeleteTelegramBot() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteTelegramBot(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: telegramKey }),
  });
}

export function useUpdateTelegramBot() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: { id: string; welcomeText?: string; token?: string }) => updateTelegramBot(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: telegramKey }),
  });
}

/** Channel names for report labels; `enabled` off where nothing shows them. */
export function useTelegramChannelNames(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ["telegram-channel-names"],
    queryFn: listTelegramChannelNames,
    enabled: options?.enabled,
    staleTime: 60_000,
  });
}

export function useAddTelegramChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ botId, channel }: { botId: string; channel: string }) => addTelegramChannel(botId, channel),
    onSuccess: () => qc.invalidateQueries({ queryKey: telegramKey }),
  });
}

export function useRemoveTelegramChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ botId, channelId }: { botId: string; channelId: string }) => removeTelegramChannel(botId, channelId),
    onSuccess: () => qc.invalidateQueries({ queryKey: telegramKey }),
  });
}

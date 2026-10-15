"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  addTelegramChannel,
  connectTelegramBot,
  deleteTelegramBot,
  listTelegramBots,
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
    mutationFn: ({ id, welcomeText }: { id: string; welcomeText: string }) => updateTelegramBot(id, { welcomeText }),
    onSuccess: () => qc.invalidateQueries({ queryKey: telegramKey }),
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

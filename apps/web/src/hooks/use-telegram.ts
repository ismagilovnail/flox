"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { connectTelegramBot, deleteTelegramBot, listTelegramBots, type ConnectTelegramBotInput } from "@/lib/api/telegram";

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

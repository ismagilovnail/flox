"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { getConversionTimeline, listConversions, type ListConversionsParams } from "@/lib/api/conversions";

const conversionsKey = (params: ListConversionsParams) => ["conversions", params] as const;
const conversionTimelineKey = (clickId: string) => ["conversions", clickId, "timeline"] as const;

export function useConversions(params: ListConversionsParams = {}) {
  return useQuery({ queryKey: conversionsKey(params), queryFn: () => listConversions(params), placeholderData: keepPreviousData });
}

export function useConversionTimeline(clickId: string) {
  return useQuery({
    queryKey: conversionTimelineKey(clickId),
    queryFn: () => getConversionTimeline(clickId),
    enabled: !!clickId,
    retry: false,
  });
}

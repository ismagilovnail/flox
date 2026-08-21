"use client";

import { useQuery } from "@tanstack/react-query";

import { listTrafficSources } from "@/lib/api/traffic-sources";

export function useTrafficSources() {
  return useQuery({ queryKey: ["traffic-sources"], queryFn: listTrafficSources });
}

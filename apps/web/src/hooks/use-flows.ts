"use client";

import { useQuery } from "@tanstack/react-query";

import { getFlows } from "@/lib/api/flows";

export function useFlows() {
  return useQuery({ queryKey: ["flows"], queryFn: getFlows });
}

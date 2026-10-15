"use client";

import { useQuery } from "@tanstack/react-query";

import { getOnboarding } from "@/lib/api/onboarding";

export function useOnboarding() {
  // Steps are done on other pages; refetching when the Overview regains
  // focus is what makes a ticked step show up.
  return useQuery({ queryKey: ["onboarding"], queryFn: getOnboarding, refetchOnWindowFocus: true });
}

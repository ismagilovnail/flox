"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { listCohorts, type CohortQuery } from "@/lib/api/ltv";

export function useCohorts(q: CohortQuery) {
  return useQuery({ queryKey: ["ltv-cohorts", q], queryFn: () => listCohorts(q), placeholderData: keepPreviousData });
}

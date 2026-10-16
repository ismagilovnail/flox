"use client";

import { useQuery } from "@tanstack/react-query";

import { getOpenApiDocument } from "@/lib/api/openapi";

export function useOpenApiDocument() {
  return useQuery({ queryKey: ["openapi"], queryFn: getOpenApiDocument, staleTime: 5 * 60_000 });
}

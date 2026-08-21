/**
 * Event Mappings (§29) — the real API layer for apps/internal/eventmapping,
 * the CRUD write path for the same event_mappings table
 * apps/internal/conversion.PostgresMapper already reads at ingest time
 * (Phase 23). Org-wide, not scoped to one network — the panel groups
 * mappings by network client-side, same as the old mock.
 */

import { apiFetch } from "@/lib/api/client";
import type { CpaStatus } from "@/lib/api/conversions";

export type EventMapping = {
  id: string;
  organizationId: string;
  networkId: string;
  networkStatus: string;
  floxStatus: CpaStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreateEventMappingInput = {
  networkId: string;
  networkStatus: string;
  floxStatus: CpaStatus;
};

export function listEventMappings(): Promise<{ eventMappings: EventMapping[] }> {
  return apiFetch("/event-mappings");
}

export function createEventMapping(input: CreateEventMappingInput): Promise<EventMapping> {
  return apiFetch("/event-mappings", { method: "POST", body: input });
}

export function deleteEventMapping(id: string): Promise<void> {
  return apiFetch(`/event-mappings/${id}`, { method: "DELETE" });
}

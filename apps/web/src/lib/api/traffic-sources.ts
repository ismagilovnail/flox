import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/trafficsource.TrafficSource — a deliberately
 * read-only slice (id/name/type/status) just enough for a picker. Full
 * traffic source CRUD isn't built yet; see docs/frontend-integration.md. */
export type TrafficSource = {
  id: string;
  name: string;
  type: string;
  status: string;
};

export function listTrafficSources(): Promise<{ trafficSources: TrafficSource[] }> {
  return apiFetch("/traffic-sources");
}

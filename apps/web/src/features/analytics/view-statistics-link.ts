/** Directory drill-in (§27.5): hands off a fully-formed report query via URL —
 * a navigation + pre-filter, not a client-side recomputation. AnalyticsView
 * reads these same param names on mount. */
export function viewStatisticsHref(dimension: "network" | "offer" | "source", value: string): string {
  const params = new URLSearchParams({ dim: dimension, val: value, tab: "line" });
  return `/analytics?${params.toString()}`;
}

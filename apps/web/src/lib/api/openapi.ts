import { apiFetch, apiUrl, API_VERSION_PREFIX } from "@/lib/api/client";
import type { OpenApiDocument } from "@/lib/openapi";

/** The public API reference — served without a session or key. */
export function getOpenApiDocument(): Promise<OpenApiDocument> {
  return apiFetch("/openapi.json");
}

/** Where the raw document can be downloaded. */
export function openApiDocumentUrl(format: "json" | "yaml"): string {
  return apiUrl(`${API_VERSION_PREFIX}/openapi.${format}`);
}

/** The document's server as an absolute URL — "/v1" when the API has no
 * public address configured, resolved against the API this app calls. */
export function serverUrl(doc: OpenApiDocument): string {
  return apiUrl(doc.servers?.[0]?.url ?? API_VERSION_PREFIX);
}

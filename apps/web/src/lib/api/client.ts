/**
 * The real frontend API boundary (Phase 27, §51). Every domain that has a
 * live Go backend now calls through here instead of a Zustand mock store —
 * `lib/api/routing.ts`'s own doc comment already promised this file's
 * shape: "Phase 27 only ever changes this file's body," now true for
 * campaigns/traffic-sources/analytics/ltv too.
 *
 * Tenant scoping: there is no session/auth yet (Phase 28) — the Go backend
 * derives organization_id from an X-Organization-Id header
 * (apps/internal/tenant), and this client sends it from
 * NEXT_PUBLIC_DEV_ORG_ID, an explicit, temporary, developer-set stand-in
 * for "whoever is logged in." This is the same interim mechanism
 * apps/internal/tenant's own doc comment names ("today: manual testing...
 * eventually: Phase 27's frontend integration") — Phase 28 replaces both
 * halves of this at once (the header lookup server-side, the env var
 * here), and nothing downstream of either changes.
 */

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const DEV_ORG_ID = process.env.NEXT_PUBLIC_DEV_ORG_ID ?? "";

export class ApiError extends Error {
  code: string;
  fields?: Record<string, string>;
  status: number;

  constructor(status: number, code: string, message: string, fields?: Record<string, string>) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

/** Thrown by every call below when NEXT_PUBLIC_DEV_ORG_ID isn't set, rather
 * than silently sending an empty header and letting the Go side's own
 * validation produce a confusing 422 far from where the real problem is. */
export class MissingDevOrgError extends Error {
  constructor() {
    super(
      "NEXT_PUBLIC_DEV_ORG_ID is not set. There is no auth yet (Phase 28) — " +
        "set it in .env.local to a real organization id to use the app against live data.",
    );
    this.name = "MissingDevOrgError";
  }
}

type RequestOptions = {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
  searchParams?: Record<string, string | undefined>;
};

export async function apiFetch<T>(path: string, options: RequestOptions = {}): Promise<T> {
  if (!DEV_ORG_ID) throw new MissingDevOrgError();

  const url = new URL(path, API_URL);
  if (options.searchParams) {
    for (const [key, value] of Object.entries(options.searchParams)) {
      if (value !== undefined) url.searchParams.set(key, value);
    }
  }

  const res = await fetch(url, {
    method: options.method ?? "GET",
    headers: {
      "X-Organization-Id": DEV_ORG_ID,
      ...(options.body !== undefined ? { "Content-Type": "application/json" } : {}),
    },
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  });

  if (res.status === 204) return undefined as T;

  const isJSON = res.headers.get("Content-Type")?.includes("application/json");
  const data = isJSON ? await res.json() : undefined;

  if (!res.ok) {
    const code = (data?.code as string) ?? "unknown";
    const message = (data?.message as string) ?? `Request failed with status ${res.status}`;
    throw new ApiError(res.status, code, message, data?.fields);
  }

  return data as T;
}

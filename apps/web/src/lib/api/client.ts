/**
 * The real frontend API boundary (Phase 27, §51). Every domain that has a
 * live Go backend calls through here instead of a Zustand mock store —
 * `lib/api/routing.ts`'s own doc comment already promised this file's
 * shape: "Phase 27 only ever changes this file's body."
 *
 * Tenant scoping (Phase 28B): apps/internal/tenant derives organization_id
 * from a session cookie now, not a client-supplied header — this client no
 * longer sends anything identifying "who's asking" itself at all.
 * `credentials: "include"` is what makes the browser attach that cookie on
 * every cross-origin call to apps/api (a different origin from apps/web in
 * dev, and possibly in production too) — without it the cookie set by
 * lib/api/auth.ts's login/signup/acceptInvite would never be sent back.
 * apps/internal/httpserver's CORS config has AllowCredentials: true and a
 * single explicit AllowedOrigins entry to make this legal (the fetch spec
 * forbids combining a wildcard origin with credentialed requests).
 *
 * A 401 here means "not signed in" — every caller either already expects
 * that (useMe, see hooks/use-auth.ts, treats it as a normal "logged out"
 * state, not an error) or lets it surface as an ApiError like any other
 * failure; there's no global interceptor redirecting to /login on 401,
 * since that would fire on a stale tab's background refetch even when the
 * user is mid-way through, say, filling out a form on another tab. Route
 * protection is proxy.ts's job (a UX redirect only — apps/api is the real
 * enforcement boundary regardless of what the browser does).
 */

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

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

type RequestOptions = {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
  searchParams?: Record<string, string | undefined>;
};

export async function apiFetch<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const url = new URL(path, API_URL);
  if (options.searchParams) {
    for (const [key, value] of Object.entries(options.searchParams)) {
      if (value !== undefined) url.searchParams.set(key, value);
    }
  }

  const res = await fetch(url, {
    method: options.method ?? "GET",
    credentials: "include",
    headers: {
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

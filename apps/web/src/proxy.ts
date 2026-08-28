import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

/**
 * Route protection (Phase 28B). This is a UX convenience only — it just
 * checks whether the session cookie is *present*, never whether it's
 * still valid, since doing that would mean an extra network round trip
 * to apps/api on every single navigation. The real enforcement boundary
 * is apps/api itself (every route requires a valid session; §52's RBAC
 * additionally gates /team/* by permission) — a visitor with a stale or
 * forged cookie who slips past this redirect still gets 401s from every
 * API call once they land on a page, exactly as if this file didn't
 * exist. See docs/auth.md.
 *
 * SESSION_COOKIE_NAME must match apps/internal/tenant.CookieName exactly
 * ("flox_session") — there is no shared constant between the Go and
 * TypeScript codebases to import here, so this is a manual sync point
 * (like NEXT_PUBLIC_API_URL matching API_URL already is).
 */
const SESSION_COOKIE_NAME = "flox_session";

const PUBLIC_PATHS = new Set(["/", "/login", "/signup", "/accept-invite", "/style-guide"]);

export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const hasSession = request.cookies.has(SESSION_COOKIE_NAME);
  const isPublic = PUBLIC_PATHS.has(pathname);

  if (!hasSession && !isPublic) {
    return NextResponse.redirect(new URL("/login", request.url));
  }

  // A signed-in visitor has no reason to see the login/signup forms again
  // — /accept-invite stays reachable regardless of an existing session
  // (accepting a second org's invite while already signed into a first
  // is a legitimate flow: it just replaces the cookie with a new one
  // scoped to the newly-joined org).
  if (hasSession && (pathname === "/login" || pathname === "/signup")) {
    return NextResponse.redirect(new URL("/overview", request.url));
  }

  return NextResponse.next();
}

export const config = {
  // api/health excluded alongside the existing static-asset exclusions:
  // it's a container/orchestrator liveness probe (§61, Phase 33's
  // apps/web/Dockerfile HEALTHCHECK), not a user-facing page, and must
  // answer 200 regardless of session state or it stops meaning "the
  // server is up."
  matcher: ["/((?!_next/static|_next/image|favicon.ico|api/health).*)"],
};

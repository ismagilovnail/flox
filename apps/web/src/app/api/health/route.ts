import { NextResponse } from "next/server";

// Container/orchestrator liveness probe only (§61, Phase 33's
// apps/web/Dockerfile HEALTHCHECK) — proves the Next.js server itself is
// up, nothing about apps/api or its dependencies. Every real data call
// still goes through src/lib/api to the actual backend (CLAUDE.md §32);
// this is the one intentional exception to that rule, matching
// apps/api/apps/tracker/apps/worker's own bare GET /health.
export function GET() {
  return NextResponse.json({ status: "ok" });
}

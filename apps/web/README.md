# apps/web

Next.js frontend — the FLOX control plane UI. Next.js (App Router) + React +
TypeScript + Tailwind CSS + shadcn/ui + Radix + TanStack Query/Table + React
Hook Form + Zod + Zustand + Lucide + date-fns + Apache ECharts.

## Development

```bash
npm install
npm run dev      # http://localhost:3000
npm run build
npm run lint
```

## Structure

```
src/
  app/         routes (App Router)
  components/  design-system / reusable UI primitives (packages/ui re-exports)
  features/    domain-specific code (campaigns, routing, analytics, ...)
  hooks/       shared React hooks
  lib/         utilities, src/lib/api (typed API client — no scattered fetch())
  stores/      Zustand stores
  types/       shared TypeScript types
  schemas/     Zod schemas
```

Design system entry point: `/style-guide` route — tokens, typography, color
system, and every reusable component in one place.

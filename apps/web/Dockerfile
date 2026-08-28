# Multi-stage build for apps/web, the Next.js frontend (§61, Phase 33).
#
# Build context is apps/web/ — unlike the three Go services, this app has
# no dependencies outside its own directory (its own package.json/
# node_modules, no npm workspace):
#
#   docker build -f apps/web/Dockerfile -t flox/web \
#     --build-arg NEXT_PUBLIC_API_URL=https://api.example.com apps/web

# ---------- dependencies ----------
FROM node:24-alpine AS deps

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

# ---------- build ----------
FROM node:24-alpine AS builder

WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

# NEXT_PUBLIC_API_URL is inlined into the client bundle at build time (how
# Next.js handles every NEXT_PUBLIC_* variable — src/lib/api/client.ts's
# own doc comment) — it must be the URL a visitor's *browser* can reach,
# never a container-internal hostname. Defaults to .env.example's own dev
# value so a plain `docker build` with no --build-arg still produces a
# working image against the documented local dev stack.
ARG NEXT_PUBLIC_API_URL=http://localhost:8080
ENV NEXT_PUBLIC_API_URL=${NEXT_PUBLIC_API_URL}
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# ---------- runtime ----------
FROM node:24-alpine AS web

WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0

RUN addgroup -g 10001 -S nodejs && adduser -u 10001 -S nextjs -G nodejs

# next.config.ts's output:"standalone" (added this phase) traces only the
# files the server actually needs; static assets and public/ are copied
# alongside separately, matching Next's own documented standalone layout.
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public

USER nextjs
EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=5 \
    CMD wget -qO- http://127.0.0.1:3000/api/health >/dev/null 2>&1 || exit 1

CMD ["node", "server.js"]

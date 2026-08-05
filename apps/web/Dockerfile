# ---- Builder stage ----
# syntax=docker/dockerfile:1.7
FROM node:22-alpine AS builder
WORKDIR /build

# Enable pnpm via corepack (Node 22 ships corepack).
RUN corepack enable

# Cache deps. Copy workspace manifests first.
COPY apps/web/package.json apps/web/pnpm-lock.yaml apps/web/pnpm-workspace.yaml ./

# Install with frozen lockfile (no workspace siblings needed here).
RUN pnpm install --frozen-lockfile

# Copy the web app source.
COPY apps/web/ ./

# Build standalone output (configured in next.config.ts).
# CI=true tells pnpm to auto-purge stale node_modules without a TTY prompt.
ENV NEXT_TELEMETRY_DISABLED=1
ENV CI=true
RUN pnpm run build

# ---- Runtime stage ----
FROM node:22-alpine AS runtime
WORKDIR /app

RUN apk add --no-cache wget && \
    addgroup -g 1001 -S nodejs && \
    adduser -S nextjs -u 1001

# Copy the standalone server + static assets from the builder.
# Next.js standalone places server.js at the standalone root; static
# assets must sit under .next/static relative to it.
COPY --from=builder /build/.next/standalone ./
COPY --from=builder /build/.next/static ./.next/static
COPY --from=builder /build/public ./public

ENV NODE_ENV=production
ENV PORT=3000
ENV HOSTNAME=0.0.0.0
EXPOSE 3000

HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=10 \
    CMD wget -qO- http://localhost:3000/ || exit 1

USER nextjs
CMD ["node", "server.js"]

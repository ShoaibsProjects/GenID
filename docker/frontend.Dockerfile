# syntax=docker/dockerfile:1
# ─── GenID Reimagined Frontend (Next.js static export) ────
# Multi-stage: build static export with pinned Node, serve via nginx
# (pinned, hardened, running as non-root).

FROM node:22-alpine AS builder

WORKDIR /app

# Cache dependencies (invalidated only when lockfile changes)
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

# Build static export
COPY frontend/ .
RUN npm run build

# ─── Runtime: nginx ───────────────────────────────────────
FROM nginx:1.27-alpine

COPY docker/nginx.conf /etc/nginx/nginx.conf

COPY --from=builder /app/out /usr/share/nginx/html

# Run as the unprivileged nginx user (pid/temp dirs already under /tmp)
RUN chown -R nginx:nginx /usr/share/nginx/html \
    && rm -rf /docker-entrypoint.d

USER nginx

ARG VERSION=dev
LABEL org.opencontainers.image.source="https://github.com/ShoaibsProjects/GenID" \
      org.opencontainers.image.title="genid-frontend" \
      org.opencontainers.image.description="GenID reimagined frontend" \
      org.opencontainers.image.version="${VERSION}"

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:3000 || exit 1

CMD ["nginx", "-g", "daemon off;"]
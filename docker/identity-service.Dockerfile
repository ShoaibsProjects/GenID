# syntax=docker/dockerfile:1
# ─── GenID Reimagined Identity Service (Go) ───────────────
# Multi-stage: build with pinned Go (CGO off, stripped), run as non-root
# on minimal Alpine. Generated protobuf code is committed under
# backend/pkg/proto, so no protoc toolchain is needed in the build.

FROM golang:1.26.6-alpine AS builder

WORKDIR /app

# Cache dependencies (invalidated only when go.mod/go.sum change)
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Build
COPY backend/ .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /app/identity-service ./cmd/identity-service

# ─── Runtime ──────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && apk upgrade --no-cache \
    && adduser -D -u 1001 genid

USER genid
WORKDIR /app

COPY --from=builder /app/identity-service .

ARG VERSION=dev
LABEL org.opencontainers.image.source="https://github.com/ShoaibsProjects/GenID" \
      org.opencontainers.image.title="genid-identity-service" \
      org.opencontainers.image.description="GenID reimagined identity service" \
      org.opencontainers.image.version="${VERSION}"

HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/health || exit 1

EXPOSE 8080 8081 9090

ENTRYPOINT ["/app/identity-service"]
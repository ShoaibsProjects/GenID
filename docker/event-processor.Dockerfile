# syntax=docker/dockerfile:1
# ─── GenID Event Processor (Go) ────────────────────────────
# Multi-stage: build with pinned Go (CGO off, stripped), run as non-root
# on minimal Alpine.

FROM golang:1.26.6-alpine AS builder

WORKDIR /app

# Cache dependencies (invalidated only when go.mod/go.sum change)
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Build
COPY backend/ .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /app/event-processor ./cmd/event-processor

# ─── Runtime ──────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && apk upgrade --no-cache \
    && adduser -D -u 1001 genid

USER genid
WORKDIR /app

COPY --from=builder /app/event-processor .

ARG VERSION=dev
LABEL org.opencontainers.image.source="https://github.com/ShoaibsProjects/GenID" \
      org.opencontainers.image.title="genid-event-processor" \
      org.opencontainers.image.description="GenID event processor" \
      org.opencontainers.image.version="${VERSION}"

HEALTHCHECK --interval=15s --timeout=5s --retries=5 \
    CMD pgrep event-processor || exit 1

ENTRYPOINT ["/app/event-processor"]
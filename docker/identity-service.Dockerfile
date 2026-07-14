# ─── ObserveID Identity Service (Go) ──────────────────────
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev protoc

WORKDIR /app

# Cache dependencies
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Generate protobuf code
COPY proto ./proto
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest && \
    protoc \
        --proto_path=proto \
        --go_out=. --go_opt=paths=source_relative \
        --go-grpc_out=. --go-grpc_opt=paths=source_relative \
        proto/event/v1/*.proto proto/model/v1/*.proto

# Build
COPY backend/ .
RUN CGO_ENABLED=0 go build -o /app/identity-service ./cmd/identity-service

# Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata

RUN adduser -D -u 1001 observeid
USER observeid

WORKDIR /app

COPY --from=builder /app/identity-service .

HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD ["/app/identity-service", "health"]

EXPOSE 8080 8081 9090

ENTRYPOINT ["/app/identity-service"]

# =============================================================================
# Samvada Platform — Application & Billing Service (ABS) Dockerfile
# =============================================================================

# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /workspace

RUN apk add --no-cache git ca-certificates tzdata

COPY proto ./proto
COPY abs ./abs

WORKDIR /workspace/abs

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/bin/abs-server \
    ./cmd/abs-server/main.go

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/bin/abs-worker \
    ./cmd/abs-worker/main.go

# Production runtime stage
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/bin/abs-server /app/abs-server
COPY --from=builder /app/bin/abs-worker /app/abs-worker
COPY --from=builder /workspace/abs/configs /app/configs
COPY --from=builder /workspace/abs/migrations /app/migrations

EXPOSE 8082 9082

ENTRYPOINT ["/app/abs-server"]

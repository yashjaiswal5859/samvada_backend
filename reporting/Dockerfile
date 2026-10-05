# =============================================================================
# Samvada Platform — Reporting Engine (RE) Dockerfile
# =============================================================================

# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /workspace

RUN apk add --no-cache git ca-certificates tzdata

COPY proto ./proto
COPY reporting ./reporting

WORKDIR /workspace/reporting

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/bin/reporting-server \
    ./cmd/reporting-server/main.go

# Production runtime stage
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/bin/reporting-server /app/reporting-server
COPY --from=builder /workspace/reporting/configs /app/configs

EXPOSE 8083 9083

ENTRYPOINT ["/app/reporting-server"]

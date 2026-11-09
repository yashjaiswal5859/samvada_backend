FROM golang:1.26-alpine AS builder

WORKDIR /workspace

RUN apk add --no-cache git ca-certificates

COPY auth ./auth

WORKDIR /workspace/auth

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/auth-service cmd/auth-service/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/migrate cmd/migrate/main.go

FROM gcr.io/distroless/static-debian12

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/auth-service .
COPY --from=builder /app/migrate .
COPY --from=builder /workspace/auth/migrations ./migrations

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/app/auth-service"]

# =============================================================================
# Samvada Platform — Speech-to-Speech (S2S) Engine Multi-Stage Containerfile
# =============================================================================

FROM golang:1.26-alpine AS builder

WORKDIR /workspace

RUN apk add --no-cache git ca-certificates tzdata

COPY proto ./proto
COPY approach_3_speech_to_speech/voice-agent-go ./approach_3_speech_to_speech/voice-agent-go

WORKDIR /workspace/approach_3_speech_to_speech/voice-agent-go

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/s2s-server cmd/server/main.go

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/s2s-server .

EXPOSE 8084

ENTRYPOINT ["/app/s2s-server"]

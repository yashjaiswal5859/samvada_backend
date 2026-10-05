# 🚀 Approach 3: Speech-to-Speech Voice Agent (Go + Gin + Worker Pool)

A streamlined, production-ready implementation of the AI Voice Agent platform using the **Gin Web Framework**, asynchronous **Worker Pool**, and **`POST /v1/calls/outbound` API**, incorporating the low-latency techniques from Approach 2.

---

## 🏗️ Architecture & Component Overview

```
                      ┌──────────────────────────────────────────────┐
                      │    Telephony Provider (Plivo/Twilio)         │
                      └──────────────────────┬───────────────────────┘
                                             │ WebSocket / Webhook
                                             ▼
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                        Go Server with Gin Framework (:8000)                             │
│                                                                                        │
│  ┌─────────────────────────┐   ┌──────────────────────────┐   ┌─────────────────────┐ │
│  │ POST /v1/calls/outbound │   │ GET /v1/telephony/answer │   │ GET /v1/stream/plivo│ │
│  │ (Outbound API Endpoint) │   │ (XML Answer Webhook)     │   │ (WebSocket Stream)  │ │
│  └────────────┬────────────┘   └──────────────────────────┘   └──────────┬──────────┘ │
│               │                                                          │            │
│               ▼                                                          ▼            │
│  ┌─────────────────────────┐                                  ┌─────────────────────┐ │
│  │  Background Call Worker │                                  │  MediaPacer Engine  │ │
│  │  (Channel Job Queue)    │                                  │  (20ms RTP Clock)   │ │
│  └────────────┬────────────┘                                  └─────────────────────┘ │
│               │                                                                       │
│               ▼                                                                       │
│  ┌─────────────────────────┐                                                          │
│  │   Pre-Synth Service     │ ──► [0ms Initial Audio Playout Cache]                    │
│  └─────────────────────────┘                                                          │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 📂 Codebase Directory Structure

```
approach_3_speech_to_speech/
└── voice-agent-go/
    ├── main.go                     # Gin Engine setup, route registration & server bootstrap
    ├── go.mod                      # Go module definition
    ├── .env                        # Environment variables
    ├── config/
    │   └── config.go               # Application configuration loader
    ├── models/
    │   └── call.go                 # Request, Response, Job, and Session structures
    ├── handlers/
    │   └── call_handler.go         # Gin HTTP API & WebSocket handlers (POST /v1/calls/outbound)
    ├── worker/
    │   └── call_worker.go          # Asynchronous worker pool for background call dispatching
    └── services/
        ├── presynth.go             # Approach 2: 0ms Turn 1 audio greeting pre-synthesis
        ├── pacer.go                # Approach 2: 20ms RTP frame pacer for G.711 µ-law audio
        ├── filler.go               # Approach 2: Instant acoustic latency-masking fillers
        └── rag.go                  # Approach 2: In-process ONNX vector RAG search (<5ms)
```

---

## ⚡ Key Low-Latency Features (Approach 2 Optimizations)

1. **Pre-Synthesis Engine (`presynth.go`)**: Pre-renders Turn 1 greeting audio into cache upon receiving `POST /v1/calls/outbound` before dialing, enabling **0ms initial playout delay**.
2. **MediaPacer (`pacer.go`)**: 20ms precision RTP ticker delivering 160-byte G.711 µ-law frames over WebSockets.
3. **Filler Service (`filler.go`)**: Pre-cached fillers ("Got it!", "Checking that") played instantly to mask LLM thinking latency.
4. **In-Process RAG (`rag.go`)**: Vector search executed directly within Go process memory (<5ms latency).

---

## 🚀 Quick Start Guide

### 1. Build and Run the Server
```bash
cd voice-agent-go
go build -o server.exe main.go
./server.exe
```

### 2. Initiate an Outbound Call
```bash
curl -X POST http://localhost:8000/v1/calls/outbound \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "+916261306890",
    "customer_name": "Yash",
    "company": "HDFC"
  }'
```

### 3. Check Health Endpoint
```bash
curl http://localhost:8000/health
```

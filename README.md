# Samvada Backend Monorepo

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Buf](https://img.shields.io/badge/Buf-v2-blue)](https://buf.build/)
[![Docker Compose](https://img.shields.io/badge/Docker_Compose-All--in--One-blue)](docker-compose.all.yml)

> **High-Performance Distributed Voice AI Backend**  
> Built strictly adhering to Clean Hexagonal Architecture, structured concurrency, and enterprise tenant isolation.

---

## 🏗️ Architecture & Service Topology

```
                       ┌───────────────────────────────┐
                       │     NGINX Gateway (:8080)     │
                       └─┬─────────────┬─────────────┬─┘
                         │             │             │
        ┌────────────────┘             │             └────────────────┐
        │ :8081                        │ :8082                        │ :8083 / :8084
        ▼                              ▼                              ▼
┌──────────────────┐          ┌──────────────────┐          ┌──────────────────┐
│   auth-service   │          │    abs-server    │          │    s2s-server    │
│  (Ory Ecosystem) │          │  (Core Business) │          │  (Voice Engine)  │
└────────┬─────────┘          └────────┬─────────┘          └────────┬─────────┘
         │                             │                             │
         │                             │ Produce Events              │ Audio Streaming
         │                             ▼                             ▼
         │                    ┌──────────────────────────────────────────────┐
         │                    │            Apache Kafka (KRaft)              │
         │                    └──────────────┬───────────────────────────────┘
         │                                   │
         │                ┌──────────────────┴──────────────────┐
         │                ▼                                     ▼
         │      ┌──────────────────┐                  ┌──────────────────┐
         │      │    abs-worker    │                  │ reporting-server │
         │      │ (Billing/Queues) │                  │  (ClickHouse 24) │
         │      └──────────────────┘                  └──────────────────┘
         ▼
┌─────────────────────────────────┐
│     PostgreSQL 16 (pgvector)    │
│   (samvada_auth + samvada_abs)  │
└─────────────────────────────────┘
```

---

## 📦 Services in This Monorepo

| Service | Path | Tech Stack | Description |
|---|---|---|---|
| **ABS** | [`abs/`](abs/) | Go 1.22, Gin, pgvector | Core application, agent studio, campaign dispatch, billing ledger |
| **Auth** | [`auth/`](auth/) | Go 1.22, Ory (Kratos/Hydra/Keto) | Passwordless authentication, session rotation, ReBAC permissions |
| **Reporting** | [`reporting/`](reporting/) | Go 1.22, ClickHouse | Real-time voice analytics, call quality metrics, aggregated billing |
| **S2S Engine** | [`approach_3_speech_to_speech/`](approach_3_speech_to_speech/) | Go, WebSocket, Fish-Speech | Speech-to-speech engine targeting sub-300ms audio turnaround |
| **Docling Worker** | [`docling-worker/`](docling-worker/) | Python 3.11, PyTorch | Unstructured document ingestion & vector embeddings for Agent RAG |
| **Proto Contracts** | [`proto/`](proto/) | Protobuf, Buf v2 | Shared gRPC schemas and canonical platform error package |
| **Infrastructure** | [`infra/`](infra/) | Docker, Helm, Prom/Grafana | K8s manifests, NGINX gateway, and observability configs |
| **Tests** | [`tests/`](tests/) | Go, K6 | Cross-service E2E test runner and load testing suites |

---

## 📐 Clean Architecture Pattern

All Go services in this repository follow the standardized Hexagonal/Clean layout:

```
service/
├── cmd/               # Executable entry points (main.go)
├── configs/           # Configuration parsing & YAML specs
├── internal/
│   ├── api/           # HTTP Handlers, DTO validation, Gin routes, Middleware
│   ├── application/   # Domain use cases and orchestrators (pure business logic)
│   ├── domain/        # Core entities, value objects, and domain error interfaces
│   ├── grpc/          # Protobuf gRPC server and client adapters
│   └── infrastructure/# Adapters: PostgreSQL repositories, Redis caching, Kafka
├── migrations/        # Sequenced database migrations (up/down SQL pairs)
└── docs/              # OpenAPI specs, Swagger UI, architectural docs
```

---

## ⚡ Deployment & Running

### Option 1: Full-Stack Docker Deployment
Starts all infrastructure, databases, and microservices behind the unified NGINX Gateway:
```bash
docker compose -f docker-compose.all.yml up --build -d
```

### Option 2: Infrastructure Only (For Local Go Development)
Spins up PostgreSQL, Redis, Kafka, ClickHouse, Mailpit, and Ory stack:
```bash
docker compose -f docker-compose.infra.yml up -d
```

---

## 🧪 Testing

```bash
# 1. Run unit & integration tests for ABS
cd abs && go test -v -race ./...

# 2. Run unit & integration tests for Auth
cd ../auth && go test -v -race ./...

# 3. Run unit tests for Reporting
cd ../reporting && go test -v ./...

# 4. Execute the 46-Step Cross-Service Integration Suite
cd .. && go run tests/e2e/scratch_e2e_runner.go
```

---

## 📖 Key Documentation Links
- **[High-Level Design (HLD)](SAMVADA_BACKEND_HLD.md)**: 1,890-line comprehensive technical specification.
- **[Infrastructure Operations Guide](INFRASTRUCTURE.md)**: Complete guide to database schemas, ports, and clusters.
- **[Architecture & Testing Guide](ARCHITECTURE_AND_TESTING_GUIDE.md)**: Worker decoupling and testing matrix.
- **[Master Implementation Plan](SAMVADA_MASTER_TODO.md)**: Current completion roadmap and milestone status.


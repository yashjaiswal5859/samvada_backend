# Samvada ABS — Application & Billing Service

> **Microservice:** Application & Billing Service (ABS)  
> **Language:** Go 1.26+  
> **HTTP Port:** `:8082`  
> **gRPC Port:** `:9082`  
> **Primary Stores:** PostgreSQL 16 (`samvada_abs`), Redis 7 (DB 1), Apache Kafka (`localhost:9092`)

---

## 1. Responsibilities

The **Application & Billing Service (ABS)** is the central management and commerce engine of Samvada:
- **Tenant Management:** Multi-tenant Organizations, isolated Workspaces, and RBAC memberships.
- **Voice Agent Studio:** Agent configuration, system prompts, LLM/TTS voice parameters, and immutable versions.
- **Knowledge Base (RAG):** Document chunking metadata, vector embeddings associations, and retrieval configs.
- **Campaigns & Contacts:** Outbound campaign scheduling, audience segmentation, and contact management.
- **Telephony & DIDs:** Inbound SIP DID mapping to agents and carrier trunk bindings.
- **Commercial & Billing Engine:** Promotional signup credits ($5.00 PostgreSQL wallet), usage tier enforcement, Stripe & Razorpay dual-gateway checkout, and invoice generation.
- **Inter-Service Communication:** Serves `AgentRuntimeService` via gRPC to S2S, queries `UsageReportingService` via gRPC from RE, and publishes domain lifecycle events to Kafka.

---

## 2. Clean Architecture Directory Structure

```
backend/abs/
├── cmd/
│   └── abs-server/
│       └── main.go              # Service bootstrap, HTTP (:8082) & gRPC (:9082) listeners
├── configs/
│   ├── config.go                # Configuration parser with env overrides
│   └── config.yaml              # Local development configuration
├── internal/
│   ├── api/
│   │   ├── dto/                 # Request/Response DTO structs with validation tags
│   │   ├── handlers/            # HTTP controller handlers
│   │   ├── middleware/          # Auth JWT, workspace resolution, CORS, correlation
│   │   └── routes/              # Route group registrations
│   ├── application/             # Use-case business logic services
│   ├── domain/                  # Pure entities, domain errors, and repository interfaces
│   ├── infrastructure/
│   │   ├── postgres/            # Database connection pool & GORM repository implementations
│   │   ├── redis/               # Session cache, ticket nonce store, and distributed locks
│   │   ├── kafka/               # Sarama producer & event consumers
│   │   └── storage/             # S3 / Local filesystem adapter
│   ├── grpc/                    # gRPC server implementation for AgentRuntimeService
│   └── workers/                 # Background workers (campaign runner, webhook dispatcher)
├── migrations/                  # Versioned SQL migration files (.up.sql / .down.sql)
├── pkg/
│   └── logger/                  # Enterprise Zap structured logger with correlation IDs
├── go.mod                       # Module definition (github.com/samvada/abs)
└── README.md                    # Service documentation
```

---

## 3. Running Locally

### Prerequisites
Ensure the shared infrastructure is running:
```powershell
cd d:\Voice AI\backend
docker compose -f docker-compose.infra.yml up -d
```

### Run ABS Server
```powershell
cd d:\Voice AI\backend\abs
go run ./cmd/abs-server
```

### Health Endpoints
- `GET http://localhost:8082/health` — Service liveness probe
- `GET http://localhost:8082/ready` — Service readiness probe
- `GET http://localhost:8082/api/v1/ping` — API connectivity test

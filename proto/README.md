# Samvada Platform — Shared Protobuf Contracts & Core Errors

> **Package:** `github.com/samvada/proto`  
> **Tooling:** Buf CLI (`bufbuild/buf:latest`) & Protoc Go plugins  
> **Target Services:** ABS, S2S Engine, Reporting Engine, Auth Service

---

## 1. Overview

This directory contains the protocol buffer contracts and shared error domain models used for cross-microservice gRPC communication across the Samvada platform:

```
backend/proto/
├── abs/v1/
│   └── agent.proto           # AgentRuntimeService (ABS -> S2S)
├── reporting/v1/
│   └── usage.proto           # UsageReportingService (RE -> ABS)
├── gen/go/                   # Auto-generated Go code (committed to git)
│   ├── abs/v1/
│   │   ├── agent.pb.go
│   │   └── agent_grpc.pb.go
│   └── reporting/v1/
│       ├── usage.pb.go
│       └── usage_grpc.pb.go
├── pkg/errors/
│   └── errors.go             # Canonical enterprise error codes & gRPC/HTTP status mappers
├── buf.yaml                  # Buf v2 module configuration
├── buf.gen.yaml              # Buf code generation template
└── go.mod                    # Standalone Go module: github.com/samvada/proto
```

---

## 2. Service Contracts

### 2.1 `AgentRuntimeService` (`abs/v1/agent.proto`)
- **Server:** ABS (Application & Billing Service) on port `:9082`
- **Client:** S2S Engine (Speech-to-Speech) on port `:9084`
- **RPCs:**
  - `GetAgentRuntimeConfig(agent_id, workspace_id)`: Loads agent instructions, voice ID, language, model, knowledge bases, and tool schemas.
  - `CheckCallAuthorization(workspace_id, phone_number, direction)`: Evaluates monthly workspace minute limits and active wallet balance before accepting an inbound SIP/WebSocket call or initiating outbound dialing.

### 2.2 `UsageReportingService` (`reporting/v1/usage.proto`)
- **Server:** Reporting Engine (ClickHouse OLAP) on port `:9083`
- **Client:** ABS (Application & Billing Service) on port `:9082`
- **RPCs:**
  - `GetUsageSummary(org_id, period_start, period_end)`: Provides consolidated billable telephony seconds, STT/LLM/TTS costs, and total USD spend for invoice generation.
  - `GetCurrentMonthUsage(org_id, workspace_id)`: Returns current billing cycle progress metrics for UI usage bars and real-time dashboard spend widgets.

---

## 3. Code Generation

Code generation uses Buf inside a containerized runner (zero local dependencies required):

```powershell
# From backend/proto:
docker run --rm -v "${PWD}:/workspace" -w /workspace bufbuild/buf:latest generate
```

To update Go dependencies:
```powershell
go mod tidy
```

---

## 4. Shared Error Standards (`pkg/errors`)

All services use `github.com/samvada/proto/pkg/errors` to maintain unified error handling across REST and gRPC boundaries:

```go
import "github.com/samvada/proto/pkg/errors"

// In HTTP handlers:
c.JSON(http.StatusNotFound, errors.ErrNotFound("agent", agentID))

// In gRPC server handlers:
return nil, errors.ErrQuotaExceeded("Organization trial minutes depleted").ToGRPCStatus()

// In gRPC client callers:
appErr := errors.FromGRPCError(err)
```

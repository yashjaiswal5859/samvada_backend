# Samvada Application & Billing Service (ABS)

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![pgvector](https://img.shields.io/badge/PostgreSQL-pgvector-336791?style=flat&logo=postgresql)](https://github.com/pgvector/pgvector)
[![Port](https://img.shields.io/badge/Port-8082-orange)](#)

> **Core Multi-Tenant Application, Telephony Ledger & Campaign Orchestrator**  
> Manages organizations, workspaces, voice agent configurations, RAG vector embeddings, campaign dialer schedules, and wallet credit deductions.

---

## 🏛️ Clean Architecture & Decoupled Workers

ABS strictly adheres to Clean Hexagonal Architecture and splits execution into two binaries:
1. **`abs-server` (:8082 HTTP / :9082 gRPC):** Stateless Gin REST API and gRPC server for tenant CRUD, agent builder, and runtime auth tickets. It acts as a pure event producer to Kafka.
2. **`abs-worker` (Background Worker):** Asynchronous event consumer handling:
   - Wallet credit deductions upon call completion (`s2s.call.completed`)
   - Campaign contact dialing dispatch (`abs.campaign.dispatch`)
   - Customer webhook deliveries with retry backoff (`abs.webhook.event`)
   - Onboarding tenant provisioning (`auth.onboarding.completed`)

---

## 📨 Kafka Event Catalog

| Event Topic | Direction | Producer | Consumer | Payload / Purpose |
|---|:---:|---|---|---|
| `s2s.call.completed` | Consumed | S2S Voice Engine | `abs-worker` | Call duration & billable seconds; deducts balance from organization wallet |
| `abs.campaign.dispatch` | Produced/Consumed | `abs-server` | `abs-worker` | Triggers parallel outbound dialer jobs for audience contacts |
| `abs.webhook.event` | Produced/Consumed | `abs-server` | `abs-worker` | Dispatches HMAC-SHA256 signed event payloads to customer webhooks |
| `auth.onboarding.completed` | Consumed | Auth Service | `abs-worker` | Auto-provisions default workspace, initial demo credits, and trial agent |
| `kb.doc.ingest` | Produced | `abs-server` | `docling-worker` | Ingestion notification for uploaded PDF/DOCX files |

---

## 🗄️ Database Schemas & Migrations (14 Versions)

Located in [`migrations/`](migrations/):
1. `000001_create_organizations_and_workspaces`: Multi-tenant hierarchy
2. `000002_create_agents`: Voice agent parameters, prompts, and telephony bindings
3. `000003_create_knowledge_bases`: RAG document collections & `pgvector` chunks
4. `000004_create_campaigns`: Outbound campaign dialer definitions and audiences
5. `000005_create_telephony_numbers`: Phone number inventory and routing
6. `000006_create_billing_and_wallets`: Credit balances, ledger transactions, and invoices
7. `000007_create_webhooks`: Webhook endpoints and event subscriptions
8. `000008_create_audit_logs`: SOC2-compliant administrative action trails
9. `000009_create_performance_indexes`: Composite B-tree indexes for low-latency queries
10. `000010_create_catalog_models`: Dynamic STT, LLM, and TTS provider inventory
11. `000011_add_catalog_category`: Model categorization flags
12. `000012_create_onboarding_table`: Operator onboarding step tracker
13. `000013_add_phone_catalog_type`: Toll-free vs local number classification
14. `000014_create_agent_versions`: Immutable versioning snapshots for rollback safety

---

## 📡 API Documentation
- **Interactive Swagger UI:** Open `docs/swagger-ui.html`.
- **OpenAPI 3.0 Specification:** See `docs/openapi.yaml` (1,352 lines).

### Key Endpoints (`:8082`)
| Endpoint | Method | Description |
|---|---|---|
| `/api/v1/workspaces` | `GET`, `POST` | Workspace listing and tenant creation |
| `/api/v1/agents` | `GET`, `POST` | List and create voice agents |
| `/api/v1/agents/:id/test-token` | `POST` | Issues signed ephemeral ticket for WebSocket testing |
| `/api/v1/agents/:id/publish` | `POST` | Creates immutable snapshot version of agent |
| `/api/v1/campaigns` | `GET`, `POST` | Outbound campaign creation and schedule start |
| `/api/v1/kb/:id/documents` | `POST` | Presigned URL generation for document upload |
| `/api/v1/billing/wallet` | `GET`, `POST` | Wallet balance inquiry and manual top-up |
| `/healthz` | `GET` | Liveness and readiness probe |

---

## 🧪 Testing

```bash
# Run unit tests
go test -v ./...

# Run tests with race detection
go test -v -race ./...
```

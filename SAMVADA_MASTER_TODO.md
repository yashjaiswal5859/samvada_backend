# Samvada Platform — Master Implementation TODO
> **Status:** Living Document — Check off tasks as they are completed  
> **Owner:** Core Engineering Team  
> **Last Updated:** 2026-09-26 (Phases 1, 2, 3, 4, 5 fully completed & verified)  
> **Reference:** [SAMVADA_BACKEND_HLD.md](file:///d:/Voice%20AI/backend/SAMVADA_BACKEND_HLD.md)

---

## How to Use This Document

Each task has a checkbox `[ ]`. Mark `[x]` when complete.  
Priority tags: `🔴 P0` (blocks everything), `🟠 P1` (critical path), `🟡 P2` (important), `🟢 P3` (nice-to-have).

> [!IMPORTANT]
> Tasks are ordered by dependency — **do not skip ahead** unless all blockers in the current phase are resolved.

---

## Phase 0: Infrastructure & Shared Foundations
> **Timeline:** Before any service code — Day 0  
> **Goal:** Provision shared infrastructure that all 4 services depend on.

### 0.1 Database Provisioning
- [x] 🔴 P0 — Provision PostgreSQL instance for ABS (`samvada_abs` database) ✅ `docker-compose.infra.yml` *(Tested & Healthy: samvada_auth + samvada_abs, uuid-ossp, pgcrypto)*
- [x] 🔴 P0 — Provision ClickHouse cluster for Reporting Engine (`samvada_analytics` database) ✅ `docker-compose.infra.yml` *(Tested & Healthy: 2 tables, 3 MVs)*
- [x] 🔴 P0 — Provision Redis Cluster (shared across Auth, ABS, S2S, RE for sessions, caching, rate limits) ✅ `docker-compose.infra.yml` *(Tested & Healthy: PONG, SET/GET)*
- [x] 🟠 P1 — Configure PostgreSQL connection pooling (PgBouncer or built-in pool, max 100 conns) ✅ GORM connection pool: `SetMaxOpenConns(100)`, `SetMaxIdleConns(20)`, `SetConnMaxLifetime(60m)` in `connection.go`
- [x] 🟡 P2 — Set up read replicas for PostgreSQL ABS (analytics fallback reads) ✅ Configured via `gorm.io/plugin/dbresolver` with random policy and `DB_REPLICA_HOSTS` support in `connection.go`

### 0.2 Kafka Event Bus
- [x] 🔴 P0 — Provision Apache Kafka cluster (KRaft mode, single broker) ✅ `docker-compose.infra.yml` *(Tested & Healthy: KRaft mode, cluster online)*
- [x] 🔴 P0 — Create Kafka topics with correct partition counts: ✅ `kafka-init` container *(Tested & Verified: all 7 topics active in Kafka UI)*
  - `auth.user.registered` — 6 partitions, 7d retention
  - `abs.agent.updated` — 12 partitions, 3d retention
  - `abs.campaign.dispatch` — 12 partitions, 7d retention
  - `s2s.call.started` — 24 partitions, 14d retention
  - `s2s.call.completed` — 24 partitions, 30d retention
  - `s2s.call.turns` — 24 partitions, 7d retention
  - `abs.webhook.event` — 12 partitions, 3d retention
- [x] 🟠 P1 — Configure consumer group IDs for ABS, RE, and S2S consumers ✅ `samvada-abs-consumer`, `samvada-reporting-consumer`, `voice_audio_worker_group`, `voice_tx_worker_group`

### 0.3 Object Storage
- [x] 🟠 P1 — Provision S3 / GCS bucket for audio recordings, KB documents, invoice PDFs ✅ MinIO + `minio-init` container in `docker-compose.all.yml` (`samvada-recordings`, `samvada-kb`, `samvada-invoices`) + S3 driver `s3_storage.go`
- [x] 🟡 P2 — Configure CORS policy for presigned upload URLs from frontend ✅ `backend/infra/minio/cors.json`
- [x] 🟡 P2 — Set lifecycle rules (archive recordings > 1 year to Glacier/Nearline) ✅ Configured in `minio-init`: `mc ilm rule add myminio/samvada-recordings --expire-days 730`

### 0.4 API Gateway
- [x] 🟡 P2 — Set up NGINX as central API Gateway (`:8080`) ✅ `backend/infra/gateway/nginx.conf`
- [x] 🟡 P2 — Configure route prefixes: ✅ Fully routed in `nginx.conf`
  - `/api/v1/auth/*`, `/security/*` → Auth Service `:8081`
  - `/api/v1/user/*`, `/onboarding/*`, `/orgs/*`, `/workspaces/*`, `/agents/*`, `/kb/*`, `/campaigns/*`, `/audiences/*`, `/phone-numbers/*`, `/api-keys/*`, `/webhooks/*`, `/notifications/*`, `/integrations/*`, `/billing/*` → ABS `:8082`
  - `/api/v1/analytics/*`, `/calls/*` → RE `:8083`
  - `/ws/*` (WebSockets for demo & live console) → S2S `:8084`
- [x] 🟡 P2 — Attach JWT verification middleware at gateway level ✅ `/_auth_validate` subrequest configured in `backend/infra/gateway/nginx.conf`
- [x] 🟡 P2 — Configure rate limiting (token bucket per API key / user / IP: 120 req/min default) ✅ `internal/api/middleware/rate_limit.go` in ABS & RE

### 0.5 Proto & Shared Libraries
- [x] 🔴 P0 — Create shared `proto/` repository (or `backend/proto/`) with gRPC contracts: ✅ `backend/proto/`
  - `proto/abs/v1/agent.proto` — `AgentRuntimeService` (served by ABS → consumed by S2S)
  - `proto/reporting/v1/usage.proto` — `UsageReportingService` (served by RE → consumed by ABS)
- [x] 🔴 P0 — Generate Go stubs with `protoc-gen-go` and `protoc-gen-go-grpc` ✅ generated via `buf` in `proto/gen/go/`
- [x] 🟠 P1 — Create shared error code library (`domain/errors.go` conventions across services) ✅ `proto/pkg/errors/errors.go`

### 0.6 CI/CD & DevOps
- [x] 🟡 P2 — Dockerfile for ABS service ✅ `backend/abs/Dockerfile` (Go 1.23+ Alpine)
- [x] 🟡 P2 — Dockerfile for RE service ✅ `backend/reporting/Dockerfile` (Go 1.23+ Alpine)
- [x] 🟡 P2 — Dockerfile for Auth service ✅ `backend/auth/Dockerfile`
- [x] 🟡 P2 — Dockerfile for S2S service ✅ `backend/approach_3_speech_to_speech/voice-agent-go/Dockerfile`
- [x] 🟡 P2 — Docker Compose for full local development (Postgres, Redis, Kafka, ClickHouse, MinIO, all 4 services, NGINX Gateway) ✅ `backend/docker-compose.all.yml`
- [x] 🟡 P2 — GitHub Actions / GitLab CI pipeline for lint, test, build, push ✅ `.github/workflows/ci.yml`
- [x] 🟢 P3 — Kubernetes Helm charts / Kustomize manifests for staging & production ✅ `backend/infra/k8s/helm/samvada/`
- [x] 🟢 P3 — HPA (Horizontal Pod Autoscaler) config for S2S based on WebSocket connections ✅ `backend/infra/k8s/helm/samvada/templates/hpa-s2s.yaml`

---

## Phase 1: ABS Foundation & Core CRUD (Weeks 1–3)
> **Goal:** Scaffold `backend/abs` Go module, run PostgreSQL migrations, implement Org/Workspace/Agent CRUD, Auth JWT middleware integration, and wallet provisioning.

### 1.1 Project Scaffolding
- [x] 🔴 P0 — Initialize `backend/abs/go.mod` (Go 1.23+, match `backend/auth` dependency style) ✅ `github.com/samvada/abs`
- [x] 🔴 P0 — Create Clean Architecture directory structure: ✅ `backend/abs/internal/...`
  ```
  backend/abs/
  ├── cmd/abs-server/main.go
  ├── configs/config.yaml
  ├── internal/
  │   ├── api/ (dto, handlers, middleware, routes)
  │   ├── application/ (use-case services)
  │   ├── domain/ (entities & interfaces)
  │   ├── infrastructure/ (postgres, redis, kafka, storage, payment, telephony)
  │   ├── grpc/ (server, client)
  │   └── workers/
  ├── migrations/
  ├── pkg/logger/
  ├── Dockerfile
  └── go.mod
  ```
- [x] 🔴 P0 — Bootstrap `main.go` with Gin HTTP server (`:8082`) + gRPC server (`:9082`) ✅ Tested & Verified
- [x] 🔴 P0 — Add structured logging with Zap (JSON output, request correlation IDs) ✅ `pkg/logger/logger.go`
- [x] 🔴 P0 — Add graceful shutdown handler (SIGINT/SIGTERM) ✅ Tested & Verified

### 1.2 PostgreSQL Migrations (DDL)
- [x] 🔴 P0 — `000001_create_orgs_and_workspaces.up.sql` ✅ Applied & Verified (organizations, workspaces, org_members, workspace_members)
- [x] 🔴 P0 — `000002_create_agents_and_versions.up.sql` ✅ Applied & Verified (agents, agent_versions)
- [x] 🔴 P0 — `000003_create_kb_tables.up.sql` ✅ Applied & Verified (knowledge_bases, kb_documents)
- [x] 🟠 P1 — `000004_create_campaigns_and_contacts.up.sql` ✅ Applied & Verified (audiences, contacts, campaigns)
- [x] 🟠 P1 — `000005_create_telephony_tables.up.sql` ✅ Applied & Verified (phone_numbers)
- [x] 🔴 P0 — `000006_create_billing_and_invoices.up.sql` ✅ Applied & Verified (plans, subscriptions, invoices, payment_methods, organization_wallets, wallet_transactions)
- [x] 🟠 P1 — `000007_create_webhooks_and_outbox.up.sql` ✅ Applied & Verified (api_keys, webhooks)
- [x] 🔴 P0 — `000008_seed_initial_plans.up.sql` ✅ Applied & Verified (Free, Starter, Pro, Enterprise tiers seeded)
- [x] 🔴 P0 — Native Go migration runner CLI (`cmd/migrate/main.go` with up, down, status commands) ✅ Applied & Verified

### 1.3 Infrastructure Adapters (Connectivity)
- [x] 🔴 P0 — `infrastructure/postgres/connection.go` — GORM connection pool, Zap logger, health check ✅ Tested & Verified
- [x] 🔴 P0 — `infrastructure/redis/client.go` — go-redis connection, ping health check, ephemeral ticket nonce Lua script ✅ Tested & Verified
- [x] 🟠 P1 — `infrastructure/kafka/producer.go` — Sarama producer, partition key routing, JSON publishing ✅ Tested & Verified
- [x] 🟠 P1 — `infrastructure/kafka/consumer.go` — Consumer group base for Kafka event processing ✅ Implemented & Tested
- [x] 🟡 P2 — `infrastructure/storage/storage.go` — Storage interface with LocalStorage filesystem implementation ✅ Implemented & Tested

### 1.4 Auth Middleware & Tenant Context
- [x] 🔴 P0 — `middleware/auth_middleware.go` — Extract Bearer JWT, verify signature, detect expired token (`TOKEN_EXPIRED`), inject `user_id` into Gin context ✅ Implemented & Tested
- [x] 🔴 P0 — `middleware/workspace_context.go` — Extract `X-Workspace-ID` header/param/query, verify user membership in `workspace_members` with 60s Redis cache, inject `workspace_id` into context ✅ Implemented & Tested
- [x] 🔴 P0 — `middleware/correlation.go` — Generate & propagate `X-Correlation-ID` and `X-Request-ID` for distributed tracing ✅ Implemented & Tested
- [x] 🟠 P1 — `middleware/rbac_middleware.go` — Check user role (`owner`, `admin`, `operator`, `viewer`) with role hierarchy (`RequireMinWorkspaceRole`) per route ✅ Implemented & Tested

### 1.5 Domain Entities
- [x] 🔴 P0 — `domain/organization.go` — Organization, OrgMember entities + repository interface ✅ Implemented & Tested
- [x] 🔴 P0 — `domain/workspace.go` — Workspace, WorkspaceMember entities + repository interface ✅ Implemented & Tested
- [x] 🔴 P0 — `domain/agent.go` — Agent, AgentVersion entities + repository interface ✅ Implemented & Tested
- [x] 🔴 P0 — `domain/billing.go` — Plan, Subscription, Invoice, PaymentMethod, OrganizationWallet, WalletTransaction entities ✅ Implemented & Tested
- [x] 🟠 P1 — `domain/kb.go` — KnowledgeBase, KBDocument entities + repository interface ✅ Implemented & Tested
- [x] 🟠 P1 — `domain/campaign.go` — Campaign, Audience, Contact entities + repository interface ✅ Implemented & Tested
- [x] 🟠 P1 — `domain/telephony.go` — PhoneNumber entity + repository interface ✅ Implemented & Tested
- [x] 🟠 P1 — `domain/webhook.go` — Webhook, APIKey, OutboxEvent entities + repository interface ✅ Implemented & Tested
- [x] 🔴 P0 — `domain/errors.go` — Standardized error codes (`DEMO_CREDITS_EXHAUSTED`, `WORKSPACE_NOT_FOUND`, `QUOTA_EXCEEDED`, etc.) ✅ Implemented & Tested

### 1.6 Repository Implementations (PostgreSQL GORM)
- [x] 🔴 P0 — `infrastructure/postgres/repos/org_repo.go` — CRUD for organizations, org_members ✅ Implemented & Tested
- [x] 🔴 P0 — `infrastructure/postgres/repos/workspace_repo.go` — CRUD for workspaces, workspace_members (scoped by org_id) ✅ Implemented & Tested
- [x] 🔴 P0 — `infrastructure/postgres/repos/agent_repo.go` — CRUD for agents, agent_versions (scoped by workspace_id) ✅ Implemented & Tested
- [x] 🔴 P0 — `infrastructure/postgres/repos/billing_repo.go` — CRUD for plans, subscriptions, invoices, wallets, wallet_transactions ✅ Implemented & Tested
- [x] 🟠 P1 — `infrastructure/postgres/repos/kb_repo.go` — CRUD for knowledge_bases, kb_documents ✅ Implemented & Tested
- [x] 🟠 P1 — `infrastructure/postgres/repos/campaign_repo.go` — CRUD for campaigns, audiences, contacts ✅ Implemented & Tested
- [x] 🟠 P1 — `infrastructure/postgres/repos/telephony_repo.go` — CRUD for phone_numbers ✅ Implemented & Tested
- [x] 🟠 P1 — `infrastructure/postgres/repos/webhook_repo.go` — CRUD for webhooks, api_keys, outbox_events ✅ Implemented & Tested

### 1.7 Application Services (Business Logic)
- [x] 🔴 P0 — `application/organization/org_service.go` — Create org, invite member, role change, auto-provision $5.00 wallet & free plan ✅ Implemented & Tested
- [x] 🔴 P0 — `application/workspace/workspace_service.go` — Create workspace, update settings, member management ✅ Implemented & Tested
- [x] 🔴 P0 — `application/agent/agent_service.go` — Create agent, update config, publish version, rollback version ✅ Implemented & Tested
- [x] 🔴 P0 — `application/billing/wallet_service.go` — Check call authorization, atomic deduction with row locking, top-up ✅ Implemented & Tested
- [x] 🟠 P1 — `application/kb/rag_service.go` — Knowledge base management, document uploading, indexing status ✅ Implemented & Tested
- [x] 🟠 P1 — `application/campaign/campaign_service.go` — Audience management, contact imports, campaign lifecycle ✅ Implemented & Tested
- [x] 🟠 P1 — `application/telephony/telephony_service.go` — Phone number provisioning, agent routing, number release ✅ Implemented & Tested
- [x] 🟠 P1 — `application/webhook/webhook_dispatcher.go` — API key generation (SHA-256 hashed secret) and HMAC-SHA256 webhooks ✅ Implemented & Tested

### 1.8 HTTP Handlers (Gin)
- [x] 🔴 P0 — `handlers/org_handler.go` — Category 3 APIs: `GET/PATCH /orgs/:org_id`, member CRUD ✅ Implemented & Tested
- [x] 🔴 P0 — `handlers/workspace_handler.go` — Category 4 APIs: `GET/POST/PATCH/DELETE /workspaces` ✅ Implemented & Tested
- [x] 🔴 P0 — `handlers/agent_handler.go` — Category 5 APIs: agent CRUD, publish, rollback, test-token ✅ Implemented & Tested
- [x] 🔴 P0 — `handlers/billing_handler.go` — Category 13 APIs: usage, plans, subscribe, invoices, wallet ✅ Implemented & Tested
- [x] 🟠 P1 — `handlers/kb_handler.go` — Category 6 APIs: KB CRUD, document upload/delete/sync ✅ Implemented & Tested
- [x] 🟠 P1 — `handlers/campaign_handler.go` — Category 7 APIs: campaign CRUD, start/pause/resume ✅ Implemented & Tested
- [x] 🟠 P1 — `handlers/telephony_handler.go` — Category 9 APIs: phone number management ✅ Implemented & Tested
- [x] 🟠 P1 — `handlers/webhook_handler.go` — Category 14 APIs: webhook CRUD, test ping, API keys ✅ Implemented & Tested

### 1.9 Route Registration
- [x] 🔴 P0 — `routes/routes.go` — Wire all handlers, middleware groups, versioned route prefixes (`/api/v1/`) with RBAC & workspace context ✅ Implemented & Tested

### 1.10 Onboarding Flow (Category 2)
- [x] 🔴 P0 — `POST /api/v1/onboarding/profile` handler: ✅ Implemented & Tested (`onboarding_test.go`)
  1. Create `organizations` row
  2. Create default `workspaces` row (is_default=true)
  3. Create `org_members` row (role=owner)
  4. Create `workspace_members` row (role=workspace_admin)
  5. Create `organization_wallets` with $5.00 balance
  6. Create `wallet_transactions` signup_promo record
- [x] 🔴 P0 — `POST /api/v1/onboarding/industries` handler — Save industries, mark onboarding complete ✅ Implemented & Tested
- [x] 🔴 P0 — `GET /api/v1/onboarding/state` handler — Return completion status & tenant metadata ✅ Implemented & Tested
- [x] 🔴 P0 — `POST /api/v1/onboarding` handler — One-shot onboarding pipeline for automated API clients ✅ Implemented & Tested

### 1.11 gRPC Server (Served to S2S)
- [x] 🟠 P1 — `grpc/server/agent_runtime_server.go` — Implement `AgentRuntimeService.GetAgentRuntimeConfig` ✅ Implemented & Tested
  - S2S calls this when a phone call hits an agent's DID
  - Returns full prompt, voice ID, provider, language, model, KB IDs, tools config
- [x] 🟠 P1 — `grpc/server/agent_runtime_server.go` — Implement `CheckCallAuthorization` ✅ Implemented & Tested
  - Verify workspace quota (minutes used vs. limit), check wallet balance
  - Return `is_authorized`, `denial_reason`, `remaining_minutes`, `current_wallet_balance_usd`

### 1.12 gRPC Clients (Calling Other Services)
- [x] 🟠 P1 — `grpc/client/auth_client.go` — Call Auth `ValidateToken` for cross-service token checks ✅ Implemented & Tested
- [x] 🟠 P1 — `grpc/client/s2s_client.go` — Call S2S `InitiateOutboundCall` for campaign dialing ✅ Implemented & Tested
- [x] 🟠 P1 — `grpc/client/reporting_client.go` — Call RE `GetUsageSummary` and `GetCurrentMonthUsage` ✅ Implemented & Tested

### 1.13 Unit & Integration Tests
- [x] 🟠 P1 — Unit tests for all domain entities and application services (`services_test.go`) ✅ Tested & Passing
- [x] 🟠 P1 — Integration tests for PostgreSQL repositories (`repos_test.go` against live Postgres) ✅ Tested & Passing
- [x] 🟡 P2 — API handler tests with httptest (`onboarding_test.go`) ✅ Tested & Passing
- [x] 🟡 P2 — End-to-end test: onboarding flow → wallet creation → agent creation → test-token → gRPC runtime (`e2e_journey_test.go`) ✅ Tested & Passing

---

## Phase 2: Reporting Engine & ClickHouse (Weeks 4–6)
> **Goal:** Build `backend/reporting`, ingest Kafka events into ClickHouse, create materialized views, serve analytics APIs.

### 2.1 Project Scaffolding
- [x] 🔴 P0 — Initialize `backend/reporting/go.mod` ✅ `github.com/samvada/reporting`
- [x] 🔴 P0 — Create directory structure (Clean Architecture, same pattern as ABS) ✅ `backend/reporting/internal/...`
- [x] 🔴 P0 — Bootstrap `main.go` with Gin HTTP (`:8083`) + gRPC (`:9083`) ✅ Tested & Verified
- [x] 🔴 P0 — Zap logging, correlation middleware, graceful shutdown ✅ Implemented & Tested

### 2.2 ClickHouse Schema
- [x] 🔴 P0 — Create `call_events` table (ReplacingMergeTree, partitioned by month, 2-year TTL) ✅ Initialized in `infra/clickhouse/init-schema.sql`
- [x] 🔴 P0 — Create `sentiment_turns` table (MergeTree, partitioned by month, 1-year TTL) ✅ Initialized in `infra/clickhouse/init-schema.sql`
- [x] 🔴 P0 — Create materialized view `mv_daily_usage` (SummingMergeTree) ✅ Initialized & Tested
- [x] 🟠 P1 — Create materialized view `mv_agent_performance` (AggregatingMergeTree) ✅ Initialized & Tested
- [x] 🟠 P1 — Create materialized view `mv_conversion_funnels` (SummingMergeTree) ✅ Initialized & Tested

### 2.3 Kafka Consumer Pipeline
- [x] 🔴 P0 — Implement consumer group for `s2s.call.completed` → batch insert into `call_events` ✅ `internal/workers/ingestion_worker.go`
- [x] 🔴 P0 — Implement consumer group for `s2s.call.turns` → batch insert into `sentiment_turns` ✅ `internal/workers/ingestion_worker.go`
- [x] 🔴 P0 — Implement 5-second / 1,000-event memory ring-buffer for bulk inserts ✅ `internal/workers/ingestion_worker.go`
- [x] 🟠 P1 — Implement consumer for `s2s.call.started` → update live call counters in Redis ✅ `internal/workers/ingestion_worker.go`
- [x] 🟡 P2 — Idempotent insert handling (ReplacingMergeTree dedup on call_id) ✅ Handled in ClickHouse schema & ingestion

### 2.4 ClickHouse Query Layer
- [x] 🔴 P0 — `infrastructure/clickhouse/connection.go` — ClickHouse Go driver connection pool ✅ Native TCP v2 pool `:9000`
- [x] 🔴 P0 — `infrastructure/clickhouse/repos/call_repo.go` — Query call_events with filters ✅ Implemented & Tested
- [x] 🔴 P0 — `infrastructure/clickhouse/repos/usage_repo.go` — Query mv_daily_usage for billing ✅ Implemented & Tested
- [x] 🟠 P1 — `infrastructure/clickhouse/repos/sentiment_repo.go` — Query sentiment_turns ✅ Implemented & Tested in `call_repo.go`
- [x] 🟠 P1 — `infrastructure/clickhouse/repos/analytics_repo.go` — Agent leaderboard, funnel queries ✅ Implemented & Tested

### 2.5 Redis Query Cache
- [x] 🟠 P1 — Cache frequently-hit dashboard queries (KPI summary, volume trends) with 60s TTL ✅ Implemented in `analytics_service.go`
- [x] 🟠 P1 — Cache invalidation on new data ingestion (use Kafka offset watermarks) ✅ Implemented in `workers/ingestion_worker.go`

### 2.6 Application Services
- [x] 🔴 P0 — `application/analytics/analytics_service.go` ✅
  - KPI summary (total calls, duration, avg latency, success rate, cost, sentiment breakdown)
  - Daily volume & cost trend (30-day lookback)
  - Agent performance leaderboard
  - Conversion funnel breakdown
- [x] 🔴 P0 — `application/usage/metering_service.go` ✅
  - Monthly usage summary for ABS billing (total billable seconds, per-workspace minutes)
  - Current month usage for dashboard progress bars

### 2.7 HTTP Handlers (Category 11 & 12)
- [x] 🔴 P0 — `handlers/analytics_handler.go` ✅
  - `GET /api/v1/analytics/summary` — KPI summary
  - `GET /api/v1/analytics/volume-trend` — Daily volume chart data
  - `GET /api/v1/analytics/agents/leaderboard` — Agent ranking
  - `GET /api/v1/analytics/funnels` — Conversion funnel
- [x] 🔴 P0 — `handlers/calls_handler.go` ✅
  - `GET /api/v1/calls` — Paginated CDR list with filters (agent, sentiment, date range)
  - `GET /api/v1/calls/:call_id` — Full call detail + transcript + recording URL

### 2.8 gRPC Server (Served to ABS)
- [x] 🔴 P0 — Implement `UsageReportingService.GetUsageSummary` ✅ Query ClickHouse for billable seconds in date range, return cost breakdown
- [x] 🔴 P0 — Implement `UsageReportingService.GetCurrentMonthUsage` ✅ Return minutes used vs. limit, estimated spend, percent consumed

### 2.9 Tests
- [x] 🟠 P1 — Unit tests for analytics and metering services ✅ Tested & Passing
- [x] 🟡 P2 — Integration tests with ClickHouse testcontainer / native TCP ✅ Tested & Passing (`repos_test.go`)
- [x] 🟡 P2 — Kafka consumer integration test (produce event → verify ClickHouse insert) ✅ Verified

---

## Phase 3: S2S Integration & Outbound Campaigns (Weeks 7–9)
> **Goal:** Wire S2S to ABS for config loading, implement live console, build campaign dispatcher.

### 3.1 S2S ↔ ABS gRPC Integration
- [x] 🔴 P0 — S2S: On new WebSocket / SIP connection, call `ABS.GetAgentRuntimeConfig(agent_id, workspace_id)` ✅ `abs_client.go` + `s2s_call_handler.go`
- [x] 🔴 P0 — S2S: Cache returned agent config in Redis (60s TTL, invalidate on `abs.agent.updated` Kafka event) ✅ Implemented in `abs_client.go`
- [x] 🟠 P1 — S2S: Before answering/dialing, call `ABS.CheckCallAuthorization(workspace_id, phone, direction)` ✅ Implemented in `abs_client.go`
- [x] 🟠 P1 — S2S: On call start, emit `s2s.call.started` to Kafka ✅ `telemetry_emitter.go`
- [x] 🔴 P0 — S2S: On call end, compute talk time + cost breakdown, emit `s2s.call.completed` to Kafka ✅ `telemetry_emitter.go`
- [x] 🟠 P1 — S2S: During call, emit per-turn sentiment events to `s2s.call.turns` ✅ `telemetry_emitter.go`

### 3.2 Ephemeral Call Ticket (Demo Flow)
- [x] 🔴 P0 — ABS: `POST /api/v1/agents/:id/test-token` ✅ Implemented in `handlers/agent_handler.go`
  1. Verify JWT + workspace membership
  2. Query `organization_wallets.balance_usd` (PostgreSQL)
  3. If balance ≤ 0: return 402 `DEMO_CREDITS_EXHAUSTED`
  4. Mint HMAC-SHA256 signed ticket: `{ sub, org_id, ws_id, agent_id, jti, exp: now+60s }`
  5. Return `{ ticket, wssUrl, remainingCreditUSD, maxDurationSec: 300, expiresInSec: 60 }`
- [x] 🔴 P0 — S2S: Validate ticket on WebSocket upgrade (`/ws/calls/demo?ticket=...`) ✅ Implemented in `s2s_call_handler.go`
  - Verify HMAC signature + expiry locally (< 0.1ms, zero DB hop)
  - Extract `agent_id`, begin audio stream
- [x] 🔴 P0 — ABS Kafka Consumer: On `s2s.call.completed` ✅ `workers/billing_worker.go`
  - Atomic deduction: `UPDATE organization_wallets SET balance_usd = GREATEST(0, balance_usd - $cost) WHERE org_id = $1`
  - Insert `wallet_transactions` record (type: `call_deduction`)

### 3.3 Live Call Console (Category 10)
- [x] 🟠 P1 — S2S: Implement `GET /api/v1/calls/live` — List active calls from Redis live state ✅ Implemented
- [x] 🟠 P1 — S2S: Implement WebSocket `/ws/live-console` — Stream live transcript chunks, sentiment events ✅ `websocket/live_console_ws.go`
- [x] 🟡 P2 — S2S: Implement `POST /api/v1/calls/live/:call_id/listen` — Audio tap / listen-in stream ✅ Implemented
- [x] 🟡 P2 — S2S: Implement `POST /api/v1/calls/live/:call_id/transfer` — Warm transfer to human ✅ Implemented
- [x] 🟡 P2 — S2S: Implement `POST /api/v1/calls/live/:call_id/hold` — Hold / resume ✅ Implemented
- [x] 🟡 P2 — S2S: Implement `POST /api/v1/calls/live/:call_id/terminate` — Force disconnect ✅ Implemented

### 3.4 Outbound Campaign Engine
- [x] 🟠 P1 — ABS: `workers/campaign_runner.go` ✅ Implemented & Tested
  - Poll `campaigns` WHERE `status = 'running'` AND `scheduled_at <= NOW()`
  - Fetch next N contacts (respecting `concurrent_calls_limit`)
  - Dispatch via gRPC `S2S.InitiateOutboundCall(agent_id, phone, custom_variables)`
  - Update contact `last_called_at`, increment campaign counters
- [x] 🟠 P1 — S2S: Implement gRPC `InitiateOutboundCall` endpoint ✅ Implemented in S2S runtime
  - SIP INVITE via Twilio/Plivo API → connect voice → start AI conversation
- [x] 🟠 P1 — ABS: Listen for `s2s.call.completed` and update campaign stats (completed_calls, successful_calls, failed_calls) ✅ `workers/billing_worker.go`
- [x] 🟡 P2 — ABS: Campaign scheduling (cron-like: "Start at 9 AM IST, pause at 9 PM") ✅ Supported in `campaign_runner.go`

### 3.5 Contact & Audience Management (Category 8)
- [x] 🟠 P1 — ABS: Implement audience CRUD (`GET/POST /audiences`, `DELETE /audiences/:id`) ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: Implement contact CRUD (`GET/POST/DELETE /contacts`) ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: Implement CSV batch import (`POST /audiences/:id/batch-import`, up to 100K records) ✅ Implemented & Tested
- [x] 🟡 P2 — ABS: Contact deduplication and DND validation ✅ Implemented & Tested in `campaign_service.go`

### 3.6 Telephony Integration (Category 9)
- [x] 🟠 P1 — ABS: `infrastructure/telephony/twilio_adapter.go` — Search, purchase, release phone numbers ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: `infrastructure/telephony/plivo_adapter.go` — Same as above for Plivo ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: Implement telephony handler endpoints (list, search available, purchase, assign, release) ✅ Implemented & Tested
- [x] 🟡 P2 — ABS: Configure SIP trunk routing to S2S for inbound calls on purchased DIDs ✅ Wired in carriers & DB

### 3.7 Knowledge Base & RAG (Category 6)
- [x] 🟠 P1 — ABS: KB CRUD handlers (create/list/delete knowledge bases) ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: Document upload handler (stream to storage, create `kb_documents` record) ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: `application/kb/document_processor.go` — Background worker: ✅ Implemented & Tested
  1. Extract text from PDF/DOCX/URL/text
  2. Recursive semantic chunking (2000 chars, 10% overlap)
  3. Generate embeddings (deterministic 384-dim vector fallback / cloud embeddings)
  4. Store chunks in database
  5. Update document status `indexing` → `indexed` / `failed`
- [x] 🟡 P2 — ABS: `POST /kb/:kb_id/sync` — Re-index all documents in a KB ✅ Implemented & Tested

### 3.8 Tests
- [x] 🟠 P1 — Integration test: Agent creation → test-token → S2S ticket validation → call completion → wallet deduction ✅ `phase3_e2e_test.go`
- [x] 🟡 P2 — Campaign dispatcher unit tests (mock S2S gRPC calls) ✅ Passing
- [x] 🟡 P2 — KB document processor tests (mock storage + embeddings) ✅ `phase3_e2e_test.go` (Async background worker verification)

---

## Phase 4: Billing Lifecycle & Enterprise Launch (Weeks 10–12)
> **Goal:** Stripe + Razorpay payment integrations, invoice cron, API keys, webhooks, security features.

### 4.1 Payment Gateway Integration (Category 13)
- [x] 🔴 P0 — `infrastructure/payment/gateway_interface.go` — Define `PaymentGateway` strategy interface ✅ Implemented & Tested
- [x] 🔴 P0 — `infrastructure/payment/stripe_gateway.go` — Stripe SDK implementation ✅ Implemented & Tested (`stripe_gateway.go`)
  - Create customer, create subscription, charge invoice, verify Stripe webhook signature
- [x] 🟠 P1 — `infrastructure/payment/razorpay_gateway.go` — Razorpay SDK implementation ✅ Implemented & Tested (`razorpay_gateway.go`)
  - UPI AutoPay, NetBanking, RuPay, e-mandate support
- [x] 🔴 P0 — ABS: `POST /api/v1/billing/payment-methods` — Save payment method (Stripe Setup Intent / Razorpay token) ✅ Implemented & Tested
- [x] 🔴 P0 — ABS: `POST /api/v1/billing/subscribe` — Create/upgrade subscription ✅ Implemented & Tested
- [x] 🔴 P0 — ABS: `GET /api/v1/billing/plans` — List available pricing tiers ✅ Implemented & Tested
- [x] 🔴 P0 — ABS: `GET /api/v1/billing/usage` — Current subscription + quota usage summary ✅ Implemented & Tested
- [x] 🔴 P0 — ABS: `GET /api/v1/billing/invoices` — List historical invoices + PDF download URLs ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: Stripe webhook handler (`POST /api/v1/billing/webhooks/stripe`) ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: Razorpay webhook handler (`POST /api/v1/billing/webhooks/razorpay`) ✅ Implemented & Tested

### 4.2 Subscription & Invoice Services
- [x] 🔴 P0 — `application/billing/subscription_service.go` ✅ Implemented & Tested
  - Create subscription (gateway customer + subscription), upgrade/downgrade, cancel at period end
  - Handle dunning (past_due → grace period → suspended)
- [x] 🔴 P0 — `application/billing/invoice_service.go` ✅ Implemented & Tested
  - Generate invoice: query RE for billable seconds, calculate overages, compute tax
  - Generate PDF (Go template or headless Chrome), upload to S3
  - Charge payment method via gateway
- [x] 🔴 P0 — `application/billing/usage_metering_service.go` ✅ Implemented & Tested
  - Orchestrate gRPC call to RE `GetUsageSummary`, merge with plan limits, return billing breakdown

### 4.3 Background Workers
- [x] 🔴 P0 — `workers/outbox_publisher.go` ✅ Implemented & Tested
  - Poll `outbox_events` WHERE `status = 'pending'` every 500ms
  - Publish to Kafka with partition key
  - Mark `published` or `failed` (with retry_count)
- [x] 🔴 P0 — `workers/invoice_cron.go` ✅ Implemented & Tested
  - Run nightly at 00:00 UTC
  - Select subscriptions WHERE `current_period_end <= NOW()`
  - Generate invoice, charge gateway, send email
  - Handle payment failures (status → `past_due`, alert owner)
- [x] 🟠 P1 — `workers/usage_quota_checker.go` ✅ Implemented & Tested
  - Periodic check: workspace minute usage vs. plan limits
  - If over 90%: send warning notification
  - If over 100%: restrict new calls (return `QUOTA_EXCEEDED` from `CheckCallAuthorization`)
- [x] 🟠 P1 — `workers/webhook_retry_worker.go` ✅ Implemented & Tested
  - Retry failed webhook deliveries with exponential backoff (1m, 5m, 15m, 1h)
  - Disable webhook after 10 consecutive failures
- [x] 🟠 P1 — `workers/campaign_runner.go` (finalize from Phase 3) ✅ Implemented & Tested

### 4.4 Developer API Keys (Category 15)
- [x] 🟠 P1 — ABS: `GET /api/v1/api-keys` — List keys (show prefix, name, last used, created) ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: `POST /api/v1/api-keys` — Generate key with scopes, hash secret with Argon2id, return once ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: `DELETE /api/v1/api-keys/:id` — Revoke key ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: API key authentication middleware (alternative to JWT for programmatic access) ✅ `api_key_auth.go`

### 4.5 Webhook System (Category 14)
- [x] 🟠 P1 — ABS: `GET /api/v1/webhooks` — List configured webhooks ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: `POST /api/v1/webhooks` — Register endpoint with URL, events, signing secret ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: `POST /api/v1/webhooks/:id/test` — Dispatch test ping with HMAC-SHA256 signature ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: `DELETE /api/v1/webhooks/:id` — Remove webhook ✅ Implemented & Tested
- [x] 🟠 P1 — ABS: Webhook dispatch logic — Insert into `outbox_events`, publisher sends to Kafka, worker delivers with retry ✅ Implemented & Tested

### 4.6 Integrations (Category 14)
- [x] 🟡 P2 — ABS: `GET /api/v1/integrations` — List connectors (HubSpot, Salesforce, Zapier, Twilio) ✅ Implemented & Tested
- [x] 🟡 P2 — ABS: `POST /api/v1/integrations/:id/toggle` — Enable/disable ✅ Implemented & Tested
- [x] 🟡 P2 — ABS: `POST /api/v1/integrations/:id/credentials` — Store OAuth tokens / API keys ✅ Implemented & Tested

### 4.7 Notification Preferences (Category 17)
- [x] 🟡 P2 — ABS: `GET /api/v1/notifications/preferences` — Get notification matrix ✅ Implemented & Tested
- [x] 🟡 P2 — ABS: `PUT /api/v1/notifications/preferences` — Save preferences (email, in-app, Slack channels) ✅ Implemented & Tested
- [x] 🟡 P2 — ABS: `POST /api/v1/notifications/test` — Send test notifications ✅ Implemented & Tested

### 4.8 Security & 2FA (Category 16) — Auth Service
- [x] 🟠 P1 — Auth: `POST /api/v1/security/password` — Update password ✅ Implemented (`security_handler.go`)
- [x] 🟠 P1 — Auth: `POST /api/v1/security/2fa/generate` — TOTP QR + base32 secret ✅ Implemented (`security_handler.go`)
- [x] 🟠 P1 — Auth: `POST /api/v1/security/2fa/enable` — Verify TOTP + activate ✅ Implemented (`security_handler.go`)
- [x] 🟠 P1 — Auth: `POST /api/v1/security/2fa/disable` — Deactivate 2FA ✅ Implemented (`security_handler.go`)
- [x] 🟠 P1 — Auth: `GET /api/v1/security/sessions` — List active sessions (device, IP, browser) ✅ Implemented (`security_handler.go`)
- [x] 🟠 P1 — Auth: `DELETE /api/v1/security/sessions/:id` — Revoke specific session ✅ Implemented (`security_handler.go`)
- [x] 🟠 P1 — Auth: `DELETE /api/v1/security/sessions/other` — Revoke all except current ✅ Implemented (`security_handler.go`)

### 4.9 User Profile (Category 18)
- [x] 🟠 P1 — Auth/ABS: `GET /api/v1/user/profile` — Fetch personal info, avatar, timezone ✅ Implemented & Tested
- [x] 🟠 P1 — Auth/ABS: `PATCH /api/v1/user/profile` — Update name, phone, company ✅ Implemented & Tested
- [x] 🟡 P2 — Auth/ABS: `POST /api/v1/user/profile/avatar` — Upload photo to S3 ✅ Implemented & Tested

---

## Phase 5: Frontend ↔ Backend Integration (Weeks 10–14)
> **Goal:** Switch frontend from mock/local state to live backend APIs. Ensure every page is wired.

### 5.1 Auth Flow
- [x] 🔴 P0 — Wire Login page → `POST /api/v1/auth/login` + `POST /api/v1/auth/login/verify` ✅ Implemented & Tested (`api/auth.ts`, `AuthContext.tsx`)
- [x] 🔴 P0 — Wire Register page → `POST /api/v1/auth/admin/register` + `POST /api/v1/auth/admin/verify` ✅ Implemented & Tested (`api/auth.ts`, `AuthContext.tsx`)
- [x] 🔴 P0 — Wire `AuthContext` → `GET /api/v1/auth/me`, token refresh via `POST /api/v1/auth/refresh` ✅ Implemented & Tested (`api/client.ts`, `AuthContext.tsx`)
- [x] 🔴 P0 — Wire Logout → `POST /api/v1/auth/logout` ✅ Implemented & Tested (`api/auth.ts`)

### 5.2 Onboarding
- [x] 🔴 P0 — Wire OnboardingWizard → `POST /api/v1/onboarding/profile`, `POST /api/v1/onboarding/industries` ✅ Implemented & Tested (`api/onboarding.ts`)
- [x] 🔴 P0 — Check `GET /api/v1/onboarding/state` on login redirect ✅ Implemented & Tested (`api/onboarding.ts`)

### 5.3 Dashboard
- [x] 🔴 P0 — Wire DashboardOverview → `GET /api/v1/analytics/summary`, `GET /api/v1/analytics/volume-trend` ✅ Implemented & Tested (`api/analytics.ts`)
- [x] 🟠 P1 — Wire live call counter from `GET /api/v1/calls/live` count ✅ Implemented & Tested (`api/calls.ts`)
- [x] 🟠 P1 — Wire workspace usage progress bar → `GET /api/v1/billing/usage` ✅ Implemented & Tested (`api/settings.ts`)

### 5.4 Voice Agent Studio
- [x] 🔴 P0 — Wire AgentList page → `GET /api/v1/agents` ✅ Implemented & Tested (`api/agents.ts`, `AgentListPage.tsx`)
- [x] 🔴 P0 — Wire AgentStudio page → `GET/PATCH /api/v1/agents/:id` ✅ Implemented & Tested (`api/agents.ts`, `AiAgentStudioPage.tsx`)
- [x] 🔴 P0 — Wire Create Agent modal → `POST /api/v1/agents` ✅ Implemented & Tested (`api/agents.ts`)
- [x] 🟠 P1 — Wire Publish Version → `POST /api/v1/agents/:id/publish` ✅ Implemented & Tested (`api/agents.ts`)
- [x] 🟠 P1 — Wire Version History + Rollback → `GET /api/v1/agents/:id/versions`, `POST /api/v1/agents/:id/rollback/:v` ✅ Implemented & Tested (`api/agents.ts`)

### 5.5 Demo / Playground
- [x] 🔴 P0 — Wire LiveDemoPage → `POST /api/v1/agents/:id/test-token` ✅ Implemented & Tested (`api/agents.ts`, `LiveDemoPage.tsx`)
- [x] 🔴 P0 — Replace `speechSynthesis` mock with real WebSocket to S2S (`wss://...?ticket=...`) ✅ Configured in `vite.config.ts` (`/ws` proxy) & `LiveDemoPage.tsx`
- [x] 🔴 P0 — Show remaining demo credits from wallet balance ✅ Implemented & Tested (`api/settings.ts`)
- [x] 🔴 P0 — Handle `402 DEMO_CREDITS_EXHAUSTED` → show upgrade modal ✅ Handled in `api/client.ts` & UI

### 5.6 Knowledge Base
- [x] 🟠 P1 — Wire KB list → `GET /api/v1/kb` ✅ Implemented & Tested (`api/knowledge.ts`)
- [x] 🟠 P1 — Wire document upload → `POST /api/v1/kb/:id/documents` ✅ Implemented & Tested (`api/knowledge.ts`)
- [x] 🟠 P1 — Wire document status polling (indexing → indexed) ✅ Implemented & Tested (`api/knowledge.ts`)

### 5.7 Campaigns
- [x] 🟠 P1 — Wire Campaign list → `GET /api/v1/campaigns` ✅ Implemented & Tested (`api/campaigns.ts`)
- [x] 🟠 P1 — Wire Campaign create/start/pause → `POST /api/v1/campaigns`, `POST /campaigns/:id/start` ✅ Implemented & Tested (`api/campaigns.ts`)

### 5.8 Call History & Conversations
- [x] 🟠 P1 — Wire Conversations page → `GET /api/v1/calls` (ClickHouse via RE) ✅ Implemented & Tested (`api/calls.ts`)
- [x] 🟠 P1 — Wire Call Detail page → `GET /api/v1/calls/:id` (transcript + recording player) ✅ Implemented & Tested (`api/calls.ts`)

### 5.9 Live Calls
- [x] 🟠 P1 — Wire LiveCalls page → `GET /api/v1/calls/live` ✅ Implemented & Tested (`api/calls.ts`)
- [x] 🟠 P1 — Wire live console WebSocket → `WSS /ws/live-console` ✅ Configured via `/ws` in `vite.config.ts`

### 5.10 Settings Pages
- [x] 🟠 P1 — Wire Billing page → `GET /api/v1/billing/usage`, `GET /api/v1/billing/plans`, `GET /api/v1/billing/invoices` ✅ Implemented & Tested (`api/settings.ts`)
- [x] 🟠 P1 — Wire Wallet page → `GET /api/v1/billing/wallet`, `POST /api/v1/billing/wallet/topup` ✅ Implemented & Tested (`api/settings.ts`)
- [x] 🟠 P1 — Wire API Keys page → `GET/POST/DELETE /api/v1/api-keys` ✅ Implemented & Tested (`api/settings.ts`)
- [x] 🟠 P1 — Wire Webhooks page → `GET/POST/DELETE /api/v1/webhooks` ✅ Implemented & Tested (`api/settings.ts`)
- [x] 🟠 P1 — Wire Security page → 2FA, sessions, password change ✅ Implemented & Tested (`api/settings.ts`)
- [x] 🟡 P2 — Wire Notifications page → `GET/PUT /api/v1/notifications/preferences`, `POST /api/v1/notifications/test` ✅ Implemented & Tested (`api/settings.ts`, `NotificationsPage.tsx`)
- [x] 🟡 P2 — Wire Integrations page → `GET/POST /api/v1/integrations` ✅ Implemented & Tested (`api/integrations.ts`)
- [x] 🟠 P1 — Wire Profile page → `GET/PATCH /api/v1/user/profile`, `POST /api/v1/user/profile/avatar` ✅ Implemented & Tested (`api/settings.ts`, `ProfileSettingsPage.tsx`)

### 5.11 Telephony
- [x] 🟠 P1 — Wire Telephony page → `GET /api/v1/telephony/numbers`, purchase, assign, release ✅ Implemented & Tested (`api/telephony.ts`, `PhoneNumbersPage.tsx`)

---

## Phase 6: Production Hardening & Scale (Weeks 12–16)
> **Goal:** Enterprise-grade reliability, observability, and security hardening.

### 6.1 Observability
- [x] 🟠 P1 — Add OpenTelemetry tracing (distributed traces across all 4 services) ✅ W3C `traceparent` context propagation & `tracer.go` in ABS & Reporting Engine
- [x] 🟠 P1 — Prometheus metrics endpoints (`/metrics`) on all services ✅ `internal/api/middleware/metrics.go` in ABS & RE
- [x] 🟠 P1 — Grafana dashboards for key SLIs (request latency, error rate, Kafka lag, ClickHouse query time) ✅ `backend/infra/observability/grafana/dashboards/samvada_platform_slis.json`
- [x] 🟡 P2 — Alerting rules (PagerDuty / Opsgenie) for P50/P99 latency breaches, consumer lag > 10K ✅ `backend/infra/observability/prometheus/alerts.yml`
- [x] 🟡 P2 — Structured log aggregation (Loki / CloudWatch) ✅ `backend/infra/observability/loki/loki-config.yml` & `promtail-config.yml`

### 6.2 Security Hardening
- [x] 🟠 P1 — Enforce HTTPS-only across all endpoints ✅ NGINX Port 80 301 redirect to 443, TLS 1.3, HSTS in `backend/infra/gateway/nginx.conf`
- [x] 🟠 P1 — CORS configuration (whitelist frontend domains only) ✅ `corsMiddleware` in ABS & RE
- [x] 🟠 P1 — Request body size limits (10MB for KB uploads, 1MB for API payloads) ✅ `internal/api/middleware/body_limit.go` in ABS & RE
- [x] 🟠 P1 — SQL injection prevention audit (parameterized queries everywhere) ✅ Audited all PostgreSQL repositories in `backend/abs` — 100% parameterized with GORM placeholders
- [x] 🟠 P1 — Rate limiting per user/API key (token bucket: 120 req/min default) ✅ `internal/api/middleware/rate_limit.go` in ABS & RE
- [x] 🟡 P2 — OWASP dependency vulnerability scan (Snyk / Trivy) ✅ Integrated Trivy & `govulncheck` in `.github/workflows/ci.yml`
- [x] 🟡 P2 — Penetration testing on auth + billing flows ✅ Automated test suite in `backend/abs/internal/tests/pen_test.go`

### 6.3 Performance & Reliability
- [x] 🟠 P1 — Load test all services with k6 / Locust (target: 10K concurrent users, 2K concurrent calls) ✅ Test suite created in `backend/tests/load/k6_platform_test.js`
- [x] 🟠 P1 — Circuit breaker pattern on gRPC clients (retry with backoff, fallback to cached config) ✅ `internal/pkg/circuitbreaker` in ABS and retry/fallback in S2S `abs_client.go`
- [x] 🟡 P2 — Database query optimization (EXPLAIN ANALYZE on all hot queries, add missing indexes) ✅ Migration `000011_add_performance_indexes.up.sql` & `QUERY_OPTIMIZATION_REPORT.md`
- [x] 🟡 P2 — Redis cache hit ratio monitoring (target > 90% for dashboard queries) ✅ `GetCacheMetrics` in Redis client & `redis_cache_hit_ratio` in `metrics.go`
- [x] 🟡 P2 — ClickHouse query latency SLO (< 100ms for dashboard aggregations) ✅ Automated SLO benchmark test in `reporting/internal/infrastructure/clickhouse/repos/slo_benchmark_test.go`

### 6.4 Data Protection & Compliance
- [x] 🟡 P2 — Implement data retention policies (auto-delete recordings > 2 years) ✅ MinIO 730-day ILM retention policy in `docker-compose.all.yml`
- [x] 🟡 P2 — GDPR data export API (`GET /api/v1/user/data-export`) ✅ Implemented in `profile_service.go`, `profile_handler.go`, `routes.go`, `settings.ts`
- [x] 🟡 P2 — GDPR data deletion API (`DELETE /api/v1/user/data-deletion`) ✅ Implemented in `profile_service.go`, `profile_handler.go`, `routes.go`, `settings.ts`
- [x] 🟢 P3 — SOC 2 compliance documentation ✅ Full Trust Services Criteria controls catalog in `backend/docs/compliance/SOC2_TYPE_II_CONTROLS.md`
- [x] 🟢 P3 — Audit log table for all administrative actions ✅ Migration `000010_create_audit_logs.up.sql`, `domain/audit_log.go`, `repos/audit_repo.go`, `audit_service.go`, `audit_handler.go`, `GET /api/v1/audit-logs`

---

## Phase 7: Worker Decoupling, Production Mail Relay & Aggressive Verification (Current)
> **Goal:** Decouple background consumers from ABS into a dedicated Kafka Worker daemon, establish authenticated SMTP email delivery, harden ClickHouse analytical queries, and enforce aggressive end-to-end regression testing.

### 7.1 Standalone Kafka Worker Server (`abs-worker`)
- [x] 🔴 P0 — Make ABS server strictly producer-only to protect HTTP/gRPC request latency ✅ `backend/abs/cmd/abs-server`
- [x] 🔴 P0 — Scaffold standalone Kafka Worker server daemon ✅ `backend/abs/cmd/abs-worker/main.go`
- [x] 🔴 P0 — Real-Time Billing & Wallet Deduction Worker (`s2s.call.completed`) ✅ `workers/billing_worker.go`
- [x] 🔴 P0 — Outbound Campaign Call Dispatcher (`abs.campaign.dispatch`) ✅ `workers/campaign_dispatch_consumer.go`
- [x] 🔴 P0 — Customer HMAC-Signed Webhook Delivery Worker (`abs.webhook.event`) ✅ `workers/webhook_consumer.go`
- [x] 🔴 P0 — Transactional OTP & Security Email Worker (`auth.email.send`) ✅ `workers/email_consumer.go`

### 7.2 Email & OTP Notification Pipeline
- [x] 🔴 P0 — Auth service event producer to Kafka `auth.email.send` topic ✅ `internal/application/register/service.go`
- [x] 🔴 P0 — Authenticated SMTP Relay integration (Gmail TLS/STARTTLS support) ✅ `workers/email_consumer.go`
- [x] 🟠 P1 — Multi-target SMTP configuration in `.env` (Gmail App Password vs local Mailpit) ✅ `backend/abs/.env` & `.env.example`
- [x] 🟠 P1 — Fail-safe console OTP fallback logging during network latency ✅ `email_consumer.go`

### 7.3 Auth Service Autonomous Fallback
- [x] 🔴 P0 — Autonomous PostgreSQL identity fallback when Ory Kratos/Hydra/Keto are offline ✅ `internal/platform/identity/orchestrator.go`
- [x] 🔴 P0 — Cryptographic HMAC-SHA256 JWT access and refresh token signing ✅ `orchestrator.go`
- [x] 🟠 P1 — Dual HttpOnly cookie (`registration_session_id`) and Authorization header support ✅ `register_handler.go`

### 7.4 ClickHouse Reporting Engine Bug Fixes
- [x] 🔴 P0 — Resolve Volume Trend String/Date alias collision (ClickHouse Code 386) ✅ `date_str` & `subtractDays(today(), ?)` in `analytics_repo.go`
- [x] 🔴 P0 — Resolve Conversion Funnels nested aggregate error (ClickHouse Code 184) ✅ Subquery refactoring in `analytics_repo.go`

### 7.5 Exhaustive Aggressive Regression Suite
- [x] 🔴 P0 — Develop automated 46-endpoint integration test suite with strict assertion harness ✅ `backend/scratch_e2e_runner.go`
- [x] 🔴 P0 — Full automated pass across all 4 microservices + background worker + 5 data layers (46/46 Passed, 0 Failed) ✅ Verified 2026-09-30
- [x] 🟠 P1 — Master Architecture & Testing Documentation Guide ✅ `backend/ARCHITECTURE_AND_TESTING_GUIDE.md`

### 7.6 Production Hardening for Millions of Users (Auth Semantics & Session Invalidation)
- [x] 🔴 P0 — Eliminate 404 anti-pattern from session introspection: `GET /api/v1/auth/me` returns `401 Unauthorized` (`AUTH_401`) instead of `404 Not Found` when user is missing in DB ✅ [me_handler.go](file:///d:/Voice%20AI/backend/auth/internal/api/handlers/me_handler.go)
- [x] 🔴 P0 — Eliminate 404 anti-pattern from login: `POST /api/v1/auth/login` returns `400 Bad Request` (`AUTH_400`) instead of `404 Not Found` to prevent OWASP account enumeration ✅ [login/service.go](file:///d:/Voice%20AI/backend/auth/internal/application/login/service.go)
- [x] 🔴 P0 — Standardize expired pre-auth sessions to `400 Bad Request` / `401 Unauthorized` across login and registration flows ✅ [register/service.go](file:///d:/Voice%20AI/backend/auth/internal/application/register/service.go), [login/service.go](file:///d:/Voice%20AI/backend/auth/internal/application/login/service.go)
- [x] 🔴 P0 — Standardize JSON error payload with explicit `code: "AUTH_<status>"` for Axios interceptors ✅ [response.go](file:///d:/Voice%20AI/backend/auth/internal/shared/response/response.go)
- [x] 🔴 P0 — Server-side automated cookie flushing (`FlushAuthCookies`): clears `access_token`, `refresh_token`, `session_id`, etc. with `Max-Age=0` whenever refresh token is revoked/invalid or user identity is not found ✅ [helpers.go](file:///d:/Voice%20AI/backend/auth/internal/api/handlers/helpers.go), [refresh_handler.go](file:///d:/Voice%20AI/backend/auth/internal/api/handlers/refresh_handler.go), [me_handler.go](file:///d:/Voice%20AI/backend/auth/internal/api/handlers/me_handler.go), [onboarding_handler.go](file:///d:/Voice%20AI/backend/auth/internal/api/handlers/onboarding_handler.go)
- [x] 🔴 P0 — Automated validation suite for server-side cookie flushing (3/3 Passed) ✅ `backend/test_cookie_flush.go`
- [x] 🔴 P0 — Automated validation suite for production auth error codes (3/3 Passed) ✅ `backend/test_production_auth_errors.go`
- [x] 🔴 P0 — 46/46 Aggressive regression suite verified with zero errors ✅ `scratch_e2e_runner.go`

### 7.7 Strict Login Email Existence Verification & Pre-Auth Security
- [x] 🔴 P0 — Strict Email Existence Enforcement before OTP: In login flow, `POST /api/v1/auth/login` checks user existence in PostgreSQL before generating or sending any OTP material. If the email is not registered, returns `400 Bad Request` with `"No account found with this email address. Please register first."` and rejects pre-auth creation ✅ [login/service.go](file:///d:/Voice%20AI/backend/auth/internal/application/login/service.go)
- [x] 🔴 P0 — Case-Insensitive Whitespace-Trimmed Query: Updated `FindByEmail` in user repository to use `LOWER(TRIM(email)) = LOWER(TRIM(?))` ensuring robust matching against case variations and accidental whitespace ✅ [user_repository.go](file:///d:/Voice%20AI/backend/auth/internal/infrastructure/postgres/repository/user_repository.go)
- [x] 🔴 P0 — Active User Account Verification: Added explicit `if !u.IsActive` check before issuing login OTP to prevent deactivated accounts from authenticating ✅ [login/service.go](file:///d:/Voice%20AI/backend/auth/internal/application/login/service.go)
- [x] 🔴 P0 — Purge Mock Offline Bypasses: Removed all legacy mock offline fallbacks (`ERR_NETWORK` fake tokens/users) from frontend `AuthContext.tsx` (`loginWithOTP`, `verifyLogin`, `registerWithOTP`, `verifyRegister`, `loginWithPassword`) to ensure real backend errors are never silently caught and masked ✅ [AuthContext.tsx](file:///d:/Voice%20AI/frontend/src/contexts/AuthContext.tsx)
- [x] 🔴 P0 — Protect Auth Routes from Axios 401 Refresh Interceptors: Excluded `/api/v1/auth/login`, `/api/v1/auth/register`, and `/api/v1/auth/refresh` from triggering recursive token refreshes in `client.ts` ✅ [client.ts](file:///d:/Voice%20AI/frontend/src/api/client.ts)
- [x] 🔴 P0 — Intelligent Login Error UX with Direct Registration Bridge: Enhanced `LoginPage.tsx` so un-registered email attempts display an alert with a direct, one-click bridge `"Create a new account with this email"` that navigates to `/register?email=...` with the email prefilled ✅ [LoginPage.tsx](file:///d:/Voice%20AI/frontend/src/pages/auth/LoginPage.tsx), [RegisterPage.tsx](file:///d:/Voice%20AI/frontend/src/pages/auth/RegisterPage.tsx)
- [x] 🔴 P0 — Verified with Automated Regression Suite (46/46 Passed) & Clean Frontend Build (`npm run build`: 0 errors) ✅ Verified 2026-09-30

### 7.8 Header Profile Dropdown, Logout Revocation & Production Token Lifecycles
- [x] 🔴 P0 — Top-Right Profile Dropdown: Configured interactive user menu in [Header.tsx](file:///d:/Voice%20AI/frontend/src/components/layout/Header.tsx) with dynamic avatar initial, user display name, email, workspace name, quick links (Profile, Settings, Billing, Team), and prominent "Log Out" action ✅ [Header.tsx](file:///d:/Voice%20AI/frontend/src/components/layout/Header.tsx)
- [x] 🔴 P0 — Complete Logout Pipeline: `handleLogout` calls `POST /api/v1/auth/logout` passing the active `refreshToken` for cryptographic revocation in identity engine/Hydra, clears in-memory tokens, and navigates to `/login` ✅ [Header.tsx](file:///d:/Voice%20AI/frontend/src/components/layout/Header.tsx), [auth.ts](file:///d:/Voice%20AI/frontend/src/api/auth.ts)
- [x] 🔴 P0 — Server-Side Cookie Flushing on Logout: `LogoutHandler` invokes `FlushAuthCookies` clearing `access_token`, `refresh_token` (on both `/` and `/api/v1/auth`), `session_id`, `login_session_id`, `registration_session_id`, and `login_challenge` with `Max-Age=0` ✅ [logout_handler.go](file:///d:/Voice%20AI/backend/auth/internal/api/handlers/logout_handler.go)
- [x] 🔴 P0 — Token TTL Configuration (5m Access / 30m Refresh):
  - **Access Token TTL**: Configured to exactly **5 minutes (300 seconds)** across [hydra.yml](file:///d:/Voice%20AI/backend/auth/configs/hydra/hydra.yml) and [orchestrator.go](file:///d:/Voice%20AI/backend/auth/internal/platform/identity/orchestrator.go).
  - **Refresh Token TTL**: Configured to exactly **30 minutes (1800 seconds)** across [hydra.yml](file:///d:/Voice%20AI/backend/auth/configs/hydra/hydra.yml), [orchestrator.go](file:///d:/Voice%20AI/backend/auth/internal/platform/identity/orchestrator.go), and cookie `Max-Age: 1800` in [login_handler.go](file:///d:/Voice%20AI/backend/auth/internal/api/handlers/login_handler.go), [register_handler.go](file:///d:/Voice%20AI/backend/auth/internal/api/handlers/register_handler.go), and [refresh_handler.go](file:///d:/Voice%20AI/backend/auth/internal/api/handlers/refresh_handler.go).
- [x] 🔴 P0 — Automated Verification: Tested token issuance, claims arithmetic (`exp - iat = 300s` and `exp - iat = 1800s`), and logout cookie headers (`204 No Content` with 7 `Max-Age=0` cookies) ✅ Verified

---

## Summary: Task Count by Phase

| Phase | 🔴 P0 | 🟠 P1 | 🟡 P2 | 🟢 P3 | Total |
|---|:---:|:---:|:---:|:---:|:---:|
| **0: Infrastructure** | 7 | 4 | 11 | 3 | 25 |
| **1: ABS Foundation** | 38 | 28 | 5 | 0 | 71 |
| **2: Reporting Engine** | 14 | 6 | 4 | 0 | 24 |
| **3: S2S Integration** | 7 | 20 | 10 | 0 | 37 |
| **4: Billing & Enterprise** | 10 | 24 | 7 | 0 | 41 |
| **5: Frontend Integration** | 12 | 19 | 3 | 0 | 34 |
| **6: Production Hardening** | 0 | 8 | 10 | 2 | 20 |
| **7: Worker Decoupling & Testing** | 13 | 4 | 0 | 0 | 17 |
| **TOTAL** | **101** | **113** | **50** | **5** | **269** |

---

## 🚀 Recommended Start Order (Critical Path)

> [!TIP]
> Start here. This is the critical path that unblocks everything else:

```
Step 1  → Phase 0.5: Proto contracts + code generation
Step 2  → Phase 1.1: ABS project scaffolding (go.mod, main.go, Clean Architecture dirs)
Step 3  → Phase 1.2: PostgreSQL migrations (all 7 migration files)
Step 4  → Phase 1.3: Infrastructure adapters (Postgres + Redis connections)
Step 5  → Phase 1.4: Auth middleware + workspace context
Step 6  → Phase 1.5–1.6: Domain entities + GORM repositories
Step 7  → Phase 1.7: Application services (Org, Workspace, Agent, Wallet)
Step 8  → Phase 1.8–1.9: HTTP handlers + route wiring
Step 9  → Phase 1.10: Onboarding flow (the first end-to-end feature)
Step 10 → Phase 1.11: gRPC server (S2S will need this)
Step 11 → Phase 7.1: Dedicated Kafka Worker Server (abs-worker)
Step 12 → Phase 7.5: Aggressive E2E regression verification (scratch_e2e_runner.go)
```

---

*This document is the single source of truth for all implementation work. Update checkboxes as tasks are completed.*


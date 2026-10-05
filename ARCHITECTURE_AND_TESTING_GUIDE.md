# Samvada Platform — Architecture, Worker Decoupling & Testing Guide

> **Document Status:** Official Platform Guide  
> **Last Updated:** 2026-09-30  
> **Author:** Core Engineering Team  
> **Coverage:** Full-Stack (Auth, ABS, Reporting, Workers, Kafka, Databases, Testing)

---

## Table of Contents

1. [System Architecture Overview](#1-system-architecture-overview)
2. [Worker Decoupling: Standalone Kafka Worker Server](#2-worker-decoupling-standalone-kafka-worker-server)
3. [Email & OTP Notification Pipeline](#3-email--otp-notification-pipeline)
4. [Identity & Authentication Architecture](#4-identity--authentication-architecture)
5. [Reporting Engine & ClickHouse OLAP Fixes](#5-reporting-engine--clickhouse-olap-fixes)
6. [Exhaustive Aggressive Testing Framework](#6-exhaustive-aggressive-testing-framework)
7. [API Verification & Curl Cheat-Sheet](#7-api-verification--curl-cheat-sheet)
8. [Operations Runbook & Troubleshooting](#8-operations-runbook--troubleshooting)

---

## 1. System Architecture Overview

The Samvada Voice AI Platform consists of four primary microservices, a dedicated asynchronous background worker server, a React frontend, and five supporting infrastructure layers.

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                            Vite Frontend (Port 5173)                             │
│                  Reverse Proxy to Internal Microservices                         │
└──────────────┬───────────────────┬───────────────────┬───────────────────────────┘
               │                   │                   │
    /api/v1/auth, /security        │            /api/v1/analytics, /calls
               │         /api/v1/onboarding,           │
               │         /orgs, /workspaces,           │
               │         /agents, /campaigns,          │
               │         /billing, /webhooks           │
               ▼                   ▼                   ▼
    ┌────────────────────┐┌────────────────────┐┌────────────────────┐
    │    Auth Service    ││     ABS Server     ││  Reporting Engine  │
    │    (Port 8081)     ││    (Port 8082)     ││    (Port 8083)     │
    │   Ory / Fallback   ││   Producer-Only    ││  ClickHouse OLAP   │
    └──────────┬─────────┘└──────────┬─────────┘└──────────┬─────────┘
               │                     │                     │
               │ Events              │ Events              │ Ingestion
               ▼                     ▼                     ▼
    ┌──────────────────────────────────────────────────────────────────────────────┐
    │                      Apache Kafka Cluster (Port 9092)                        │
    │  Topics: auth.email.send, s2s.call.completed, abs.campaign.dispatch, ...     │
    └──────────────────────────────────────┬───────────────────────────────────────┘
                                           │
                                           │ Consumes All Topics
                                           ▼
                            ┌──────────────────────────────┐
                            │   Standalone Kafka Worker    │
                            │        (abs-worker)          │
                            │  ──────────────────────────  │
                            │  [1] Billing Deductions      │
                            │  [2] Campaign Dispatch       │
                            │  [3] Customer Webhooks       │
                            │  [4] Gmail SMTP / Mailpit    │
                            └──────────────┬───────────────┘
                                           │
                                           ▼
                            ┌──────────────────────────────┐
                            │  Postgres / Gmail SMTP Relay │
                            └──────────────────────────────┘
```

### Port & Service Inventory

| Service | Port(s) | Responsibility | Backing Storage / Protocol |
|---|---|---|---|
| **Frontend UI** | `5173` | React Dashboard, Agent Studio, Analytics | HTTP / WebSocket |
| **Auth Service** | `8081` | Registration, OTP Auth, JWT Issuance, Ory Fallback | PostgreSQL (`samvada_auth`), Redis |
| **ABS Server** | `8082` (HTTP), `9082` (gRPC) | Multi-tenant Org, Workspace, Agent, Campaign, Billing | PostgreSQL (`samvada_abs`), Redis |
| **Reporting Server** | `8083` (HTTP), `9083` (gRPC) | Real-time analytics, call transcripts, KPIs, funnels | ClickHouse (`samvada_analytics`), Redis |
| **S2S Voice Engine**| `8084` (WS), `9084` (gRPC) | Speech-to-Speech real-time voice pipeline | Redis, MinIO |
| **Kafka Worker** | Background Daemon | Asynchronous billing, campaign dispatch, webhooks, emails | Kafka, PostgreSQL, SMTP |
| **PostgreSQL** | `5432` | Relational storage for auth and tenant operations | `samvada_auth`, `samvada_abs` |
| **Redis** | `6379` | Sessions, token blocklist, rate limiting, agent caching | Cache DB 0 |
| **ClickHouse** | `8123` (HTTP), `9000` (Native) | High-speed analytical queries and aggregations | `samvada_analytics` |
| **Apache Kafka** | `9092` | Distributed event streaming bus | KRaft Mode (broker 1) |
| **Mailpit** | `1025` (SMTP), `8025` (Web UI) | Local development SMTP server and web inbox | In-memory / dev storage |

---

## 2. Worker Decoupling: Standalone Kafka Worker Server

### Architecture Decision: ABS as Producer-Only

To ensure predictable HTTP API latency and eliminate noisy-neighbor interference between customer requests and background tasks:
1. **ABS Server (`abs-server`) is strictly an event producer.** It handles incoming HTTP and gRPC requests, validates data, writes transactions to PostgreSQL, and produces event messages to Kafka topics.
2. **ABS Worker (`abs-worker`) is a dedicated background consumer daemon.** It runs independently in its own process, joins consumer groups, handles backoff/retries, and processes workloads asynchronously.

### Consumers Executed by `abs-worker`

Located at: [backend/abs/cmd/abs-worker/main.go](file:///d:/Voice%20AI/backend/abs/cmd/abs-worker/main.go)

```
[Consumer 1: Billing Worker]
  Topic:          s2s.call.completed
  Consumer Group: abs-billing-worker-group
  Action:         Parses call duration and billing seconds, calculates telephony/LLM/TTS/STT costs,
                  and performs atomic deduction from organization's wallet balance in PostgreSQL.

[Consumer 2: Campaign Dispatcher]
  Topic:          abs.campaign.dispatch
  Consumer Group: abs-campaign-worker-group
  Action:         Reads outbound campaign contact batches, queries S2S engine via gRPC,
                  and queues voice calls according to configured concurrency limits.

[Consumer 3: Webhook Delivery]
  Topic:          abs.webhook.event
  Consumer Group: abs-webhook-worker-group
  Action:         Looks up registered customer webhooks, computes HMAC-SHA256 signature
                  with webhook secret, and delivers payload with exponential backoff retries.

[Consumer 4: Transactional Email & OTP]
  Topic:          auth.email.send
  Consumer Group: abs-email-worker-group
  Action:         Receives OTP verification and notification payloads, formats MIME emails,
                  and dispatches via authenticated SMTP relay (Gmail) or local Mailpit.
```

---

## 3. Email & OTP Notification Pipeline

### Overview

When a user registers or logs in via `/api/v1/auth/admin/register` or `/api/v1/auth/login`:
1. **Auth Service** generates a cryptographically secure 6-digit OTP and hashes it using SHA-256 in PostgreSQL `pending_auth`.
2. **Auth Service** publishes a message to Kafka topic `auth.email.send`:
   ```json
   {
     "event_id": "06cb7946-ff66-452e-871b-f45181538daa",
     "recipient": "user@example.com",
     "type": "REGISTER_OTP",
     "otp": "257225"
   }
   ```
3. **`abs-worker`** consumes this message and invokes [email_consumer.go](file:///d:/Voice%20AI/backend/abs/internal/workers/email_consumer.go).
4. The worker establishes an authenticated TLS/STARTTLS connection to the configured SMTP server and delivers the message.

### SMTP Configuration Reference

Configuration is managed via [backend/abs/.env](file:///d:/Voice%20AI/backend/abs/.env):

```env
# -----------------------------------------------------------------------------
# REAL EMAIL DELIVERY VIA GMAIL
# -----------------------------------------------------------------------------
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=proyashjaiswal@gmail.com
SMTP_PASSWORD=your_16_char_app_password
SMTP_FROM=samvada@gmail.com

# -----------------------------------------------------------------------------
# LOCAL MAILPIT (Alternative for offline development)
# -----------------------------------------------------------------------------
# SMTP_HOST=localhost
# SMTP_PORT=1025
# SMTP_USER=
# SMTP_PASSWORD=
# SMTP_FROM=no-reply@samvada.ai
```

> [!NOTE]
> Even if SMTP relay encounters network timeouts, `abs-worker` prints the verification code clearly in its stdout logs, guaranteeing that local development and automated testing never block on third-party email latency.

---

## 4. Identity & Authentication Architecture

### Dual-Engine Design: Ory + Autonomous Fallback

Located at: [orchestrator.go](file:///d:/Voice%20AI/backend/auth/internal/platform/identity/orchestrator.go)

- **Primary Mode:** In enterprise Kubernetes clusters, the Auth Service delegates identity storage to Ory Kratos (`:4434`), OAuth2 token lifecycle to Ory Hydra (`:4445`), and relation-based access to Ory Keto (`:4467`).
- **Autonomous Local Mode:** When Ory containers are not running locally, the Auth Service activates its built-in identity orchestrator:
  1. Inserts the user identity into PostgreSQL `samvada_auth` (`identities` table).
  2. Generates standard HMAC-SHA256 JWT access tokens signed with `JWT_SECRET` (`samvada_jwt_super_secret_dev_2026_key`).
  3. Returns standard claims:
     ```json
     {
       "sub": "880fd9d3-2512-47a5-8540-347add8c090e",
       "email": "user@samvada.ai",
       "org_id": "31b208aa-d2b8-4221-a0e2-dec1e67f227f",
       "exp": 1774900000
     }
     ```
  4. Both `abs-server` and `reporting-server` validate this token using the shared `JWT_SECRET`.

### Security Verification Protocol

To prevent CSRF and session hijacking during registration:
1. `POST /api/v1/auth/admin/register` returns an HTTP-only session cookie: `registration_session_id`.
2. `POST /api/v1/auth/admin/verify` **requires** both `requestId`, `otp`, and the matching `registration_session_id` cookie.

---

## 5. Reporting Engine & ClickHouse OLAP Fixes

### Materialized View Architecture

The Reporting Engine leverages ClickHouse (`samvada_analytics`) for sub-second analytical aggregations:
- `call_events` — Raw fact table storing every turn, latency, sentiment, and cost metric.
- `mv_daily_usage` — `SummingMergeTree` aggregating daily total calls, minutes, spend, and average latency.
- `mv_conversion_funnels` — `SummingMergeTree` tracking funnel stages (dialed, connected, engaged, qualified, converted).
- `mv_agent_performance` — Aggregated score and sentiment metrics per agent.

### Bug Fixes Implemented in Analytics Queries

File: [analytics_repo.go](file:///d:/Voice%20AI/backend/reporting/internal/infrastructure/clickhouse/repos/analytics_repo.go)

1. **Volume Trend String/Date Type Clash (ClickHouse Code 386)**:
   - *Previous Bug:* `SELECT toString(date) AS date ... GROUP BY date` caused ClickHouse to confuse the string alias `date` with the table column `date (Date)`.
   - *Fix:* Renamed alias to `date_str` and used parameterized `subtractDays(today(), ?)` instead of raw `INTERVAL` expressions.
2. **Conversion Funnel Nested Aggregations (ClickHouse Code 184)**:
   - *Previous Bug:* `toFloat64(if(sum(total_dialed) > 0, sum(converted_calls) * 100.0 / sum(total_dialed), 0.0))` placed an aggregate function inside conditional calculation during `WHERE` evaluation.
   - *Fix:* Refactored into a clean subquery: the inner query performs the `sum()` operations grouped by date, and the outer query cleanly evaluates the conversion rate percentage.

---

## 6. Exhaustive Aggressive Testing Framework

### Test Harness Overview

Located at: [backend/scratch_e2e_runner.go](file:///d:/Voice%20AI/backend/scratch_e2e_runner.go)

The test harness executes **46 comprehensive integration tests** across the entire stack:
- **No Mocks:** Every test sends real TCP/HTTP/Kafka packets over localhost.
- **Strict Status Checking:** Any status other than the expected 200/201/202 fails the test and dumps the full server response body.

### Running the Test Suite

```powershell
cd 'd:\Voice AI\backend\auth'
go run 'd:\Voice AI\backend\scratch_e2e_runner.go'
```

### Full 46-Test Coverage Matrix

```
[1] Core Infrastructure (5/5)
  ✔ PostgreSQL (samvada_auth)                Port 5432 Auth DB connectivity
  ✔ PostgreSQL (samvada_abs)                 Port 5432 ABS DB connectivity
  ✔ Redis Cache & Sessions                   Port 6379 Cache & Token Store
  ✔ Kafka Broker & Topics                    Port 9092 Topic discovery (all 7 topics)
  ✔ Mailpit Dev SMTP Server                  Port 8025 / 1025 API reachability

[2] Auth Service APIs (8/8)
  ✔ GET  /health                             Service health check
  ✔ POST /api/v1/auth/admin/register         Admin registration & session cookie issue
  ✔ POST /api/v1/auth/admin/verify           OTP verification & JWT token generation
  ✔ GET  /api/v1/auth/me                     Token validation & user identity payload
  ✔ POST /api/v1/auth/login                  Login challenge creation
  ✔ POST /api/v1/auth/login/verify           Login OTP verification & session refresh
  ✔ POST /api/v1/auth/refresh                Token rotation & refresh token exchange
  ✔ GET  /api/v1/onboarding/state            User onboarding completion flag

[3] ABS Service APIs (20/20) - Tested with live Auth JWT
  ✔ POST /api/v1/onboarding                  Tenant org, default workspace & $5 demo credits
  ✔ POST /api/v1/workspaces                  Secondary workspace provisioning
  ✔ GET  /api/v1/workspaces                  List user workspaces
  ✔ GET  /api/v1/billing/wallet              Retrieve wallet balance & demo credits
  ✔ GET  /api/v1/billing/wallet/transactions Transaction history log
  ✔ GET  /api/v1/billing/plans               List subscription tier definitions
  ✔ POST /api/v1/agents                      Create voice assistant agent
  ✔ GET  /api/v1/agents                       List active agents in workspace
  ✔ GET  /api/v1/agents/:id                  Retrieve agent configuration by ID
  ✔ GET  /api/v1/agents/:id/versions         Agent prompt & voice version history
  ✔ POST /api/v1/audiences                   Create contact audience list
  ✔ GET  /api/v1/audiences                    List audience segments
  ✔ POST /api/v1/campaigns                   Create outbound voice campaign
  ✔ GET  /api/v1/campaigns                    List campaigns
  ✔ GET  /api/v1/phone-numbers               List provisioned telephony numbers
  ✔ GET  /api/v1/kb                          List knowledge base documents
  ✔ POST /api/v1/api-keys                    Generate developer API key
  ✔ GET  /api/v1/api-keys                     List active API keys
  ✔ POST /api/v1/webhooks                    Register customer webhook endpoint
  ✔ GET  /api/v1/webhooks                     List customer webhooks

[4] Reporting Engine APIs (6/6) - ClickHouse OLAP
  ✔ GET  /api/v1/analytics/summary            Executive KPI overview
  ✔ GET  /api/v1/analytics/volume-trend       Daily volume & spend trend
  ✔ GET  /api/v1/analytics/agents/leaderboard Agent performance leaderboard
  ✔ GET  /api/v1/analytics/funnels            Conversion funnels aggregation
  ✔ GET  /api/v1/analytics/live-calls         Active in-flight call sessions
  ✔ GET  /api/v1/calls                        Paginated call detail records (CDR)

[5] Kafka Worker Pipelines (3/3)
  ✔ Produce s2s.call.completed               Real-time billing wallet deduction
  ✔ Produce abs.campaign.dispatch            Outbound campaign S2S dispatcher
  ✔ Produce abs.webhook.event                HMAC customer webhook delivery

[6] Frontend Reverse Proxy (4/4) - Port 5173
  ✔ 5173 -> 8081 (/api/v1/auth/me)           Vite proxy to Auth Service
  ✔ 5173 -> 8082 (/api/v1/workspaces)        Vite proxy to ABS Service
  ✔ 5173 -> 8082 (/api/v1/agents)            Vite proxy to ABS Agents
  ✔ 5173 -> 8083 (/api/v1/analytics/summary) Vite proxy to Reporting Service
```

---

## 7. API Verification & Curl Cheat-Sheet

All commands use the Vite Reverse Proxy on **port 5173** (same as the browser):

### 1. Register User & Receive OTP
```bash
curl -i -X POST http://localhost:5173/api/v1/auth/admin/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@samvada.ai","displayName":"Engineering Lead"}'
```
*Response Header contains:* `Set-Cookie: registration_session_id=<UUID>; ...`  
*Response Body contains:* `data.requestId`

### 2. Verify OTP & Obtain JWT
```bash
curl -i -X POST http://localhost:5173/api/v1/auth/admin/verify \
  -H "Content-Type: application/json" \
  -b "registration_session_id=<UUID_FROM_STEP_1>" \
  -d '{"requestId":"<REQUEST_ID_FROM_STEP_1>","otp":"<6_DIGIT_OTP>"}'
```
*Response Body contains:* `accessToken`, `refreshToken`

### 3. Complete Initial Onboarding
```bash
curl -X POST http://localhost:5173/api/v1/onboarding \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <ACCESS_TOKEN>" \
  -d '{
    "company_name": "Acme Voice AI",
    "slug": "acme-voice",
    "billing_email": "test@samvada.ai"
  }'
```
*Response Body contains:* `data.organization_id`, `data.workspace_id`

### 4. Create Voice Agent
```bash
curl -X POST http://localhost:5173/api/v1/agents \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <ACCESS_TOKEN>" \
  -H "X-Workspace-ID: <WORKSPACE_ID>" \
  -d '{
    "name": "Customer Support Agent",
    "role": "Customer Support",
    "system_prompt": "You are a professional customer support assistant.",
    "voice_id": "alloy",
    "voice_provider": "fish_audio",
    "voice_name": "Alloy Voice",
    "language": "en-US",
    "model": "gpt-4o"
  }'
```

### 5. Fetch Analytics KPI Summary
```bash
curl -X GET "http://localhost:5173/api/v1/analytics/summary?workspace_id=<WORKSPACE_ID>&org_id=<ORG_ID>" \
  -H "Authorization: Bearer <ACCESS_TOKEN>" \
  -H "X-Org-ID: <ORG_ID>"
```

---

## 8. Operations Runbook & Troubleshooting

### How to Start the Entire Stack

1. **Start Backing Infrastructure Containers:**
   ```powershell
   cd 'd:\Voice AI\backend'
   docker compose -f docker-compose.infra.yml up -d
   ```
2. **Start Microservices (in separate terminal windows or as background processes):**
   - **Auth Service:**
     ```powershell
     cd 'd:\Voice AI\backend\auth'; go run ./cmd/auth-service
     ```
   - **ABS Server:**
     ```powershell
     cd 'd:\Voice AI\backend\abs'; go run ./cmd/abs-server
     ```
   - **ABS Worker (Background Consumers):**
     ```powershell
     cd 'd:\Voice AI\backend\abs'; go run ./cmd/abs-worker
     ```
   - **Reporting Server:**
     ```powershell
     cd 'd:\Voice AI\backend\reporting'; go run ./cmd/reporting-server
     ```
   - **Frontend Dev Server:**
     ```powershell
     cd 'd:\Voice AI\frontend'; npm run dev
     ```

### Troubleshooting Checklist

- **"Invalid OTP" or "Authentication required"**:
  Ensure the `registration_session_id` cookie is passed alongside `requestId` and `otp` during verify calls.
- **ClickHouse "Unknown database samvada_reporting"**:
  The analytics database is named `samvada_analytics`. Verify `config.yaml` points to `samvada_analytics`.
- **Worker not dispatching emails to Gmail**:
  Verify 2-Step Verification is active on the Google Account and use a generated 16-character App Password (without spaces) in `backend/abs/.env`.
- **Vite reverse proxy 502 Bad Gateway**:
  Verify the target service port is active (`8081` for Auth, `8082` for ABS, `8083` for Reporting).

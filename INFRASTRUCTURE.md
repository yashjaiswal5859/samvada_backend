# Samvada Infrastructure — Setup & Operations Guide

> **Document Status:** Living Document  
> **Last Updated:** 2026-09-26  
> **Audience:** All backend engineers  
> **Prerequisites:** Docker Desktop (or Docker Engine + Docker Compose v2)

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Quick Start](#2-quick-start)
3. [Service Inventory & Connection Details](#3-service-inventory--connection-details)
4. [PostgreSQL (Auth + ABS)](#4-postgresql-auth--abs)
5. [Redis (Shared Cache & Sessions)](#5-redis-shared-cache--sessions)
6. [ClickHouse (Analytics OLAP)](#6-clickhouse-analytics-olap)
7. [Apache Kafka (Event Bus)](#7-apache-kafka-event-bus)
8. [Kafka UI (Web Dashboard)](#8-kafka-ui-web-dashboard)
9. [Volume Management & Data Persistence](#9-volume-management--data-persistence)
10. [Troubleshooting](#10-troubleshooting)
11. [Environment Variable Reference](#11-environment-variable-reference)

---

## 1. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Docker Compose: samvada-infra                    │
│                    Network: samvada-network                         │
│                                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │
│  │  PostgreSQL   │  │    Redis     │  │  ClickHouse  │              │
│  │   :5432       │  │    :6379     │  │  :8123 :9000 │              │
│  │              │  │              │  │              │              │
│  │ samvada_auth │  │  Sessions    │  │ samvada_     │              │
│  │ samvada_abs  │  │  Cache       │  │ analytics    │              │
│  └──────────────┘  │  Rate Limits │  └──────────────┘              │
│                    └──────────────┘                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │
│  │  Kafka       │  │  Kafka UI    │  │   Mailpit    │              │
│  │  (KRaft)     │  │  :8088       │  │  :8025 :1025 │              │
│  │  :9092       │  │  Web Console │  │  Dev SMTP UI │              │
│  └──────────────┘  └──────────────┘  └──────────────┘              │
└─────────────────────────────────────────────────────────────────────┘
         ↑                ↑                ↑                ↑
    Auth Service     ABS Server       S2S Engine     Reporting Engine
      :8081            :8082           :8084            :8083
    (Go process)    (Producer Only) (Go process)    (Go process)
                          │ (Kafka events)
                          ▼
                   ┌──────────────┐
                   │  ABS Worker  │
                   │ (Background) │
                   └──────────────┘
```

All Go microservices and background workers run natively on the host machine during development
and connect to the Dockerized infrastructure via `localhost` ports. For complete worker decoupling and testing details, see [ARCHITECTURE_AND_TESTING_GUIDE.md](file:///d:/Voice%20AI/backend/ARCHITECTURE_AND_TESTING_GUIDE.md).

---

## 2. Quick Start

### Prerequisites

1. **Docker Desktop** installed and running  
   - Windows: [Download Docker Desktop](https://docs.docker.com/desktop/install/windows-install/)
   - Ensure WSL 2 backend is enabled (Settings → General → "Use WSL 2 based engine")
2. **Minimum Resources**: Allocate at least **4 GB RAM** and **2 CPUs** to Docker (Settings → Resources)

### Start All Infrastructure

```powershell
# Navigate to the backend directory
cd d:\Voice AI\backend

# Start all infrastructure services in detached mode
docker compose -f docker-compose.infra.yml up -d

# Watch logs (all services)
docker compose -f docker-compose.infra.yml logs -f

# Watch logs (specific service)
docker compose -f docker-compose.infra.yml logs -f postgres
docker compose -f docker-compose.infra.yml logs -f kafka
```

### Verify All Services Are Healthy

```powershell
docker compose -f docker-compose.infra.yml ps
```

Expected output:
```
NAME                    STATUS                  PORTS
samvada-postgres        running (healthy)       0.0.0.0:5432->5432/tcp
samvada-redis           running (healthy)       0.0.0.0:6379->6379/tcp
samvada-clickhouse      running (healthy)       0.0.0.0:8123->8123/tcp, 0.0.0.0:9000->9000/tcp
samvada-kafka           running (healthy)       0.0.0.0:9092->9092/tcp
samvada-kafka-init      exited (0)              —
samvada-kafka-ui        running                 0.0.0.0:8088->8080/tcp
samvada-mailpit         running (healthy)       0.0.0.0:1025->1025/tcp, 0.0.0.0:8025->8025/tcp
```

> [!IMPORTANT]
> The `kafka-init` container will show `exited (0)` — this is expected. It's a one-shot job that creates Kafka topics and then terminates.

### Stop Infrastructure

```powershell
# Stop all containers (preserves data)
docker compose -f docker-compose.infra.yml down

# Stop and DESTROY all data volumes (fresh start)
docker compose -f docker-compose.infra.yml down -v
```

---

## 3. Service Inventory & Connection Details

| Service | Container Name | Port(s) | Connection String / URL |
|---|---|---|---|
| **PostgreSQL (Auth)** | `samvada-postgres` | `5432` | `postgresql://samvada:samvada_dev_2026@localhost:5432/samvada_auth` |
| **PostgreSQL (ABS)** | `samvada-postgres` | `5432` | `postgresql://samvada:samvada_dev_2026@localhost:5432/samvada_abs` |
| **Redis** | `samvada-redis` | `6379` | `redis://localhost:6379/0` |
| **ClickHouse** | `samvada-clickhouse` | `8123` (HTTP), `9000` (TCP) | `clickhouse://samvada:samvada_ch_2026@localhost:9000/samvada_analytics` |
| **Kafka** | `samvada-kafka` | `9092` | `localhost:9092` |
| **Kafka UI** | `samvada-kafka-ui` | `8088` | http://localhost:8088 |
| **Mailpit (Dev SMTP)**| `samvada-mailpit` | `1025` (SMTP), `8025` (Web UI)| http://localhost:8025 |
| **ABS Worker (Daemon)**| Standalone Go Process | Background | Kafka consumer for billing, campaigns, webhooks, email |


---

## 4. PostgreSQL (Auth + ABS)

### Overview

A single PostgreSQL 16 instance hosts **two isolated databases**:

| Database | Service | Purpose |
|---|---|---|
| `samvada_auth` | Auth Service | Users, sessions, OTP, OAuth, 2FA |
| `samvada_abs` | ABS Service | Organizations, workspaces, agents, billing, campaigns, telephony |

### Credentials

| Field | Value |
|---|---|
| Host | `localhost` |
| Port | `5432` |
| Username | `samvada` |
| Password | `samvada_dev_2026` |
| Auth Database | `samvada_auth` |
| ABS Database | `samvada_abs` |

### Connect via CLI

```powershell
# Connect to Auth database
docker exec -it samvada-postgres psql -U samvada -d samvada_auth

# Connect to ABS database
docker exec -it samvada-postgres psql -U samvada -d samvada_abs

# List all databases
docker exec -it samvada-postgres psql -U samvada -c "\l"

# List tables in ABS
docker exec -it samvada-postgres psql -U samvada -d samvada_abs -c "\dt"
```

### Connect via GUI Tools

Use any PostgreSQL client (pgAdmin, DBeaver, DataGrip, TablePlus):
- **Host:** `localhost`
- **Port:** `5432`
- **User:** `samvada`
- **Password:** `samvada_dev_2026`
- **Database:** `samvada_auth` or `samvada_abs`

### Installed Extensions

Both databases have the following extensions pre-installed:

| Extension | Purpose |
|---|---|
| `uuid-ossp` | UUID generation functions (`uuid_generate_v4()`) |
| `pgcrypto` | Cryptographic functions (`gen_random_uuid()`, `crypt()`, `gen_salt()`) |

### Init Script

The [init-databases.sh](file:///d:/Voice%20AI/backend/infra/postgres/init-databases.sh) script runs **only on first boot** (when the volume is empty). It:
1. Creates the `samvada_abs` database (the `samvada_auth` database is created automatically by `POSTGRES_DB` env var)
2. Enables `uuid-ossp` and `pgcrypto` extensions on both databases

### Data Persistence

PostgreSQL data is stored in the Docker volume `samvada-postgres-data`. Data survives container restarts. To reset:

```powershell
docker compose -f docker-compose.infra.yml down
docker volume rm samvada-postgres-data
docker compose -f docker-compose.infra.yml up -d
```

---

## 5. Redis (Shared Cache & Sessions)

### Overview

A single Redis 7 instance shared across all 4 microservices:

| Service | Redis Usage | Key Prefix Convention |
|---|---|---|
| Auth | OTP storage, session cache, rate limiting | `auth:otp:*`, `auth:session:*`, `auth:rl:*` |
| ABS | Agent config cache, call ticket nonces | `abs:agent:*`, `abs:ticket:*` |
| S2S | Live call state registry, agent config cache | `s2s:live:*`, `s2s:agent:*` |
| RE | Dashboard query cache (60s TTL) | `re:cache:*` |

### Configuration

| Setting | Value | Reason |
|---|---|---|
| `appendonly` | `yes` | AOF persistence for session crash recovery |
| `maxmemory` | `256mb` | Dev environment limit |
| `maxmemory-policy` | `allkeys-lru` | Evict least-recently-used keys when full |
| `save` | `60 1000` | RDB snapshot every 60s if 1000+ writes |

### Connect via CLI

```powershell
# Open Redis CLI
docker exec -it samvada-redis redis-cli

# Test connection
docker exec -it samvada-redis redis-cli ping
# Expected: PONG

# Check memory usage
docker exec -it samvada-redis redis-cli info memory

# List all keys (dev only — never in production!)
docker exec -it samvada-redis redis-cli keys "*"
```

### Data Persistence

Redis data is stored in `samvada-redis-data` volume. AOF ensures session data survives restarts.

---

## 6. ClickHouse (Analytics OLAP)

### Overview

ClickHouse 24 serves as the analytical backbone for the Reporting Engine. It stores billions of call events and provides sub-100ms aggregation queries for dashboards.

### Credentials

| Field | Value |
|---|---|
| Host | `localhost` |
| HTTP Port | `8123` (for web queries and health checks) |
| TCP Port | `9000` (for Go driver — native protocol) |
| Username | `samvada` |
| Password | `samvada_ch_2026` |
| Database | `samvada_analytics` |

### Pre-Created Schema

The [init-schema.sql](file:///d:/Voice%20AI/backend/infra/clickhouse/init-schema.sql) script automatically creates:

| Object | Type | Engine | Purpose |
|---|---|---|---|
| `call_events` | Table | ReplacingMergeTree | Raw CDR (Call Detail Records) |
| `sentiment_turns` | Table | MergeTree | Per-turn sentiment/emotion telemetry |
| `mv_daily_usage` | Materialized View | SummingMergeTree | Daily org/workspace usage rollup |
| `mv_agent_performance` | Materialized View | AggregatingMergeTree | Per-agent daily performance |
| `mv_conversion_funnels` | Materialized View | SummingMergeTree | Campaign conversion funnel |

### Connect via CLI

```powershell
# Open ClickHouse CLI
docker exec -it samvada-clickhouse clickhouse-client --user samvada --password samvada_ch_2026

# List databases
docker exec -it samvada-clickhouse clickhouse-client --user samvada --password samvada_ch_2026 --query "SHOW DATABASES"

# List tables
docker exec -it samvada-clickhouse clickhouse-client --user samvada --password samvada_ch_2026 --query "SHOW TABLES FROM samvada_analytics"

# Quick health check via HTTP
curl http://localhost:8123/ping
# Expected: Ok.
```

### Connect via GUI

- **DBeaver**: Add ClickHouse connection → Host: `localhost`, Port: `8123`, Database: `samvada_analytics`
- **DataGrip**: Same as above
- **Built-in Web UI**: http://localhost:8123/play (limited query interface)

### Data Retention (TTL)

| Table | TTL | Meaning |
|---|---|---|
| `call_events` | 2 years | Auto-drops partitions older than 24 months |
| `sentiment_turns` | 1 year | Auto-drops partitions older than 12 months |

---

## 7. Apache Kafka (Event Bus)

### Overview

Kafka runs in **KRaft mode** (no Zookeeper required). This is the production-recommended mode since Kafka 3.5+. A single broker is sufficient for local development.

### Pre-Created Topics

The `kafka-init` container automatically creates all required topics on first boot:

| Topic | Partitions | Retention | Producer | Consumer(s) | Purpose |
|---|:---:|:---:|---|---|---|
| `auth.user.registered` | 6 | 7 days | Auth | ABS | Triggers default org/workspace creation |
| `abs.agent.updated` | 12 | 3 days | ABS | S2S | Invalidates agent config cache |
| `abs.campaign.dispatch` | 12 | 7 days | ABS | S2S | Triggers outbound campaign calls |
| `abs.webhook.event` | 12 | 3 days | ABS | ABS Worker | Dispatches webhook to customer URL |
| `s2s.call.started` | 24 | 14 days | S2S | ABS, RE | Increments live call counter |
| `s2s.call.completed` | 24 | 30 days | S2S | ABS, RE | CDR storage + wallet deduction |
| `s2s.call.turns` | 24 | 7 days | S2S | RE | Turn-level sentiment ingestion |

### Connect via CLI

```powershell
# List all topics
docker exec -it samvada-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka:9092 --list

# Describe a topic
docker exec -it samvada-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka:9092 --describe --topic s2s.call.completed

# Produce a test message
docker exec -it samvada-kafka /opt/kafka/bin/kafka-console-producer.sh --bootstrap-server kafka:9092 --topic s2s.call.completed

# Consume messages (from beginning)
docker exec -it samvada-kafka /opt/kafka/bin/kafka-console-consumer.sh --bootstrap-server kafka:9092 --topic s2s.call.completed --from-beginning

# Check consumer group lag
docker exec -it samvada-kafka /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server kafka:9092 --describe --all-groups
```

### Go Service Connection

All Go services connect via:
```
KAFKA_BROKERS=localhost:9092
```

---

## 8. Kafka UI (Web Dashboard)

### Access

**URL:** http://localhost:8088

### Features

- Browse all topics and their configurations
- View and search messages in any topic
- Monitor consumer group lag and offsets
- Inspect broker health and cluster metrics
- Produce test messages to any topic

---

## 9. Volume Management & Data Persistence

All data is stored in named Docker volumes:

| Volume Name | Service | Size Estimate (Dev) | Contains |
|---|---|---|---|
| `samvada-postgres-data` | PostgreSQL | ~100 MB | Auth + ABS relational data |
| `samvada-redis-data` | Redis | ~10 MB | AOF + RDB persistence files |
| `samvada-clickhouse-data` | ClickHouse | ~50 MB | Analytics tables and MV data |
| `samvada-clickhouse-logs` | ClickHouse | ~5 MB | Query logs and error logs |
| `samvada-kafka-data` | Kafka | ~50 MB | Topic partitions and logs |

### Commands

```powershell
# List all volumes
docker volume ls | findstr samvada

# Inspect a volume
docker volume inspect samvada-postgres-data

# Remove a specific volume (stops containers first)
docker compose -f docker-compose.infra.yml down
docker volume rm samvada-postgres-data

# Nuclear reset — destroy everything and start fresh
docker compose -f docker-compose.infra.yml down -v
docker compose -f docker-compose.infra.yml up -d
```

---

## 10. Troubleshooting

### PostgreSQL Won't Start

```powershell
# Check logs
docker compose -f docker-compose.infra.yml logs postgres

# Common issue: Port 5432 already in use (local PG running)
# Fix: Stop local PostgreSQL or change the port in docker-compose.infra.yml
netstat -aon | findstr :5432
```

### Kafka Init Shows Errors

```powershell
# Check kafka-init logs
docker compose -f docker-compose.infra.yml logs kafka-init

# If topics weren't created, re-run:
docker compose -f docker-compose.infra.yml up kafka-init
```

### ClickHouse "Too Many Parts" Error

This happens when too many small inserts are made rapidly. The Reporting Engine prevents this by using 5-second batch buffers. If you see this in dev:

```sql
-- Force merge all parts
OPTIMIZE TABLE samvada_analytics.call_events FINAL;
```

### Redis Connection Refused

```powershell
# Verify Redis is running
docker compose -f docker-compose.infra.yml ps redis

# Check if port is available
netstat -aon | findstr :6379
```

### Complete Reset (Fresh Start)

```powershell
docker compose -f docker-compose.infra.yml down -v
docker compose -f docker-compose.infra.yml up -d
```

---

## 11. Environment Variable Reference

Copy these into your Go service `.env` files:

### Auth Service (`backend/auth/.env`)
```env
DB_HOST=localhost
DB_PORT=5432
DB_USER=samvada
DB_PASSWORD=samvada_dev_2026
DB_NAME=samvada_auth
DB_SSLMODE=disable

REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

KAFKA_BROKERS=localhost:9092
```

### ABS Service (`backend/abs/.env`)
```env
DB_HOST=localhost
DB_PORT=5432
DB_USER=samvada
DB_PASSWORD=samvada_dev_2026
DB_NAME=samvada_abs
DB_SSLMODE=disable

REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=1

KAFKA_BROKERS=localhost:9092
```

### Reporting Engine (`backend/reporting/.env`)
```env
CLICKHOUSE_HOST=localhost
CLICKHOUSE_PORT=9000
CLICKHOUSE_HTTP_PORT=8123
CLICKHOUSE_USER=samvada
CLICKHOUSE_PASSWORD=samvada_ch_2026
CLICKHOUSE_DATABASE=samvada_analytics

REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=2

KAFKA_BROKERS=localhost:9092
```

### S2S Engine (`backend/approach_3_speech_to_speech/.env` additions)
```env
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=3

KAFKA_BROKERS=localhost:9092
```

> [!TIP]
> Each service uses a different Redis DB index (0–3) for logical namespace isolation while sharing the same Redis instance.

---

## File Structure

```
backend/
├── docker-compose.infra.yml          # Main infrastructure compose file
├── infra/
│   ├── postgres/
│   │   └── init-databases.sh         # Auto-creates samvada_auth + samvada_abs
│   └── clickhouse/
│       └── init-schema.sql           # Auto-creates tables + materialized views
├── SAMVADA_BACKEND_HLD.md            # High-Level Design document
├── SAMVADA_MASTER_TODO.md            # Master implementation checklist
└── INFRASTRUCTURE.md                 # ← This document
```

---

*Samvada Infrastructure Team — Internal Development Documentation*

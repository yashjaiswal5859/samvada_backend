# Samvada Reporting & Analytics Service

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![ClickHouse](https://img.shields.io/badge/ClickHouse-24-FFCC01?style=flat&logo=clickhouse)](https://clickhouse.com/)
[![Port](https://img.shields.io/badge/Port-8083-orange)](#)

> **High-Throughput Real-Time Telemetry, Call Quality & Billing Analytics**  
> Powered by ClickHouse columnar storage and Kafka streaming ingestion for sub-second aggregations across millions of call records.

---

## ⚡ Core Capabilities & SLI Formulas

1. **Streaming Ingestion:** Consumes call lifecycle and turn events from Kafka topics (`s2s.call.completed`, `s2s.call.turns`) via high-efficiency ring buffer batching.
2. **Columnar Aggregations (ClickHouse):** Leverages ClickHouse MergeTree tables and Materialized Views to compute:
   - **Turnaround Time (TAT) P50/P95/P99:** Total elapsed time from caller speech silence to first synthesized audio byte.
   - **Mean Opinion Score (MOS):** Telephony audio clarity estimation based on jitter, packet loss, and codec metrics.
   - **Call Completion Rate:** Ratio of completed calls without network disconnection or abnormal drops.
3. **Usage & Metering:** Aggregates minute-by-minute voice agent consumption per workspace for accurate ABS billing reconciliation.

---

## 🗄️ ClickHouse Table Architecture

Initialized automatically via [`backend/infra/clickhouse/init-schema.sql`](../infra/clickhouse/init-schema.sql):

```sql
-- 1. Call Events Table (Granular call lifecycle and quality telemetry)
CREATE TABLE IF NOT EXISTS call_events (
    call_id UUID,
    org_id UUID,
    workspace_id UUID,
    agent_id UUID,
    started_at DateTime64(3),
    ended_at DateTime64(3),
    duration_sec UInt32,
    billable_sec UInt32,
    e2e_latency_ms UInt32,
    stt_latency_ms UInt32,
    llm_latency_ms UInt32,
    tts_latency_ms UInt32,
    evaluation_score UInt8,
    disposition LowCardinality(String),
    total_cost_usd Float64
) ENGINE = MergeTree()
ORDER BY (org_id, workspace_id, started_at, call_id);

-- 2. Sentiment & Turn-by-Turn Telemetry
CREATE TABLE IF NOT EXISTS sentiment_turns (
    call_id UUID,
    turn_index UInt16,
    speaker LowCardinality(String),
    sentiment LowCardinality(String),
    turn_latency_ms UInt32,
    timestamp DateTime64(3)
) ENGINE = MergeTree()
ORDER BY (call_id, turn_index);

-- 3. Daily Usage Materialized View (Sub-millisecond billing queries)
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_daily_usage
ENGINE = SummingMergeTree()
PRIMARY KEY (org_id, workspace_id, call_date)
AS SELECT
    org_id,
    workspace_id,
    toDate(started_at) AS call_date,
    count() AS total_calls,
    sum(duration_sec) AS total_duration_sec,
    sum(total_cost_usd) AS total_cost_usd
FROM call_events
GROUP BY org_id, workspace_id, call_date;
```

---

## 📁 Package Layout

```
reporting/
├── cmd/reporting-server/main.go  # HTTP (:8083) and gRPC entrypoint
├── configs/                      # Config loader and YAML definitions
├── internal/
│   ├── api/                      # Analytics, Calls, and Usage Gin handlers
│   ├── application/              # Aggregation and analytics query services
│   ├── domain/                   # Domain entities (CallRecord, UsageMetric, SLI)
│   ├── grpc/server/              # UsageReportingService gRPC implementation
│   ├── infrastructure/
│   │   ├── clickhouse/           # ClickHouse connection pool & query repositories
│   │   ├── postgres/             # PostgreSQL metadata sync
│   │   └── redis/                # Query caching adapter
│   └── workers/                  # Kafka batch ingestion workers
└── Dockerfile                    # Multi-stage production container
```

---

## 📡 API Endpoints (`:8083`)

| Method | Endpoint | Description |
|---|---|---|
| `GET`  | `/api/v1/analytics/overview` | Platform-wide volume, duration, and latency summary |
| `GET`  | `/api/v1/analytics/sli` | Real-time P95 latency and MOS quality scores |
| `GET`  | `/api/v1/calls` | Filterable call history with transcript and audio URLs |
| `GET`  | `/api/v1/calls/:id` | Detailed call breakdown and telemetry timeseries |
| `GET`  | `/api/v1/usage/summary` | Current billing period minutes consumed per workspace |
| `GET`  | `/healthz` | Service health check |

---

## 🧪 Testing

```bash
# Run unit tests
go test -v ./...

# Run tests with race detection
go test -v -race ./...
```

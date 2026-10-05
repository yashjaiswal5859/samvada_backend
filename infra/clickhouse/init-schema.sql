-- =============================================================================
-- Samvada Reporting Engine — ClickHouse Schema Initialization
-- =============================================================================
-- This script runs ONCE on first container boot.
-- It creates the analytics database, all raw event tables, and
-- pre-aggregated materialized views for sub-100ms dashboard queries.
--
-- Database: samvada_analytics
-- Tables:
--   1. call_events         — Raw Call Detail Records (CDR)
--   2. sentiment_turns     — Per-turn sentiment & emotion telemetry
-- Materialized Views:
--   3. mv_daily_usage      — Daily org/workspace usage rollup (billing)
--   4. mv_agent_performance — Per-agent daily performance metrics
--   5. mv_conversion_funnels — Outbound campaign funnel aggregates
-- =============================================================================

-- The database is auto-created by CLICKHOUSE_DB env var, but ensure it exists:
CREATE DATABASE IF NOT EXISTS samvada_analytics;

-- Switch to the analytics database
USE samvada_analytics;

-- =============================================================================
-- 1. Raw Call Events Table (CDR)
-- =============================================================================
-- Every completed/failed call produces one row in this table.
-- Ingested from Kafka topic: s2s.call.completed
-- Engine: ReplacingMergeTree deduplicates on (call_id) during merges.
-- Partition: Monthly partitions for efficient range scans and TTL.
-- TTL: Auto-drop data older than 2 years.
-- =============================================================================
CREATE TABLE IF NOT EXISTS call_events (
    call_id UUID,
    org_id UUID,
    workspace_id UUID,
    agent_id UUID,
    campaign_id Nullable(UUID),
    direction LowCardinality(String),           -- 'inbound', 'outbound'
    caller_number String,
    called_number String,
    customer_name String,
    started_at DateTime64(3, 'UTC'),
    ended_at Nullable(DateTime64(3, 'UTC')),
    duration_sec UInt32,
    billable_sec UInt32,
    disposition LowCardinality(String),         -- 'completed', 'busy', 'failed', 'no_answer', 'in_progress', 'escalated'
    evaluation_score UInt8,                     -- 0-100 AI quality score

    -- Component Latency Benchmarks (milliseconds)
    stt_latency_ms UInt16,
    llm_latency_ms UInt16,
    tts_latency_ms UInt16,
    e2e_latency_ms UInt16,

    -- Financials (USD with 6 decimal precision)
    cost_telephony_usd Decimal64(6),
    cost_stt_usd Decimal64(6),
    cost_llm_usd Decimal64(6),
    cost_tts_usd Decimal64(6),
    total_cost_usd Decimal64(6),

    -- Conversation Metrics
    turn_count UInt16,
    overall_sentiment LowCardinality(String),   -- 'positive', 'neutral', 'negative'
    recording_url String,
    transcript_summary String,
    created_at DateTime64(3, 'UTC')
) ENGINE = ReplacingMergeTree(created_at)
PARTITION BY toYYYYMM(started_at)
PRIMARY KEY (org_id, workspace_id, started_at, call_id)
ORDER BY (org_id, workspace_id, started_at, call_id)
TTL toDateTime(started_at) + INTERVAL 2 YEAR;


-- =============================================================================
-- 2. Turn-Level Sentiment & Emotion Stream
-- =============================================================================
-- Every conversational turn (agent or customer utterance) produces one row.
-- Ingested from Kafka topic: s2s.call.turns
-- Used for: Live sentiment graphs, emotion heatmaps, transcript analysis.
-- TTL: Auto-drop after 1 year (turn-level data is high-volume).
-- =============================================================================
CREATE TABLE IF NOT EXISTS sentiment_turns (
    call_id UUID,
    org_id UUID,
    workspace_id UUID,
    agent_id UUID,
    turn_index UInt16,
    speaker LowCardinality(String),             -- 'agent', 'customer'
    timestamp DateTime64(3, 'UTC'),
    text String,
    sentiment LowCardinality(String),           -- 'positive', 'neutral', 'negative'
    emotion LowCardinality(String),             -- 'happy', 'frustrated', 'neutral', 'angry', 'confused'
    sentiment_score Float32,                    -- -1.0 to +1.0
    latency_ms UInt16
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
PRIMARY KEY (org_id, workspace_id, timestamp, call_id)
ORDER BY (org_id, workspace_id, timestamp, call_id, turn_index)
TTL toDateTime(timestamp) + INTERVAL 1 YEAR;


-- =============================================================================
-- 3. Materialized View: Daily Organization & Workspace Usage
-- =============================================================================
-- Auto-aggregates call_events into daily usage rollups.
-- Primary consumer: ABS billing engine (via RE gRPC GetUsageSummary).
-- Engine: SummingMergeTree auto-sums numeric columns on merge.
-- =============================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_daily_usage
ENGINE = SummingMergeTree()
PRIMARY KEY (org_id, workspace_id, date)
ORDER BY (org_id, workspace_id, date)
AS SELECT
    org_id,
    workspace_id,
    toDate(started_at) AS date,
    count() AS total_calls,
    countIf(disposition = 'completed') AS completed_calls,
    countIf(disposition = 'failed') AS failed_calls,
    countIf(disposition = 'escalated') AS escalated_calls,
    sum(duration_sec) AS total_duration_sec,
    sum(billable_sec) AS total_billable_sec,
    sum(total_cost_usd) AS total_spend_usd,
    sum(cost_telephony_usd) AS spend_telephony_usd,
    sum(cost_stt_usd) AS spend_stt_usd,
    sum(cost_llm_usd) AS spend_llm_usd,
    sum(cost_tts_usd) AS spend_tts_usd,
    avg(e2e_latency_ms) AS avg_e2e_latency_ms
FROM call_events
GROUP BY org_id, workspace_id, date;


-- =============================================================================
-- 4. Materialized View: Agent Performance Metrics
-- =============================================================================
-- Per-agent daily rollup for leaderboards and agent comparison dashboards.
-- Engine: AggregatingMergeTree stores intermediate aggregate states.
-- =============================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_agent_performance
ENGINE = AggregatingMergeTree()
PRIMARY KEY (org_id, workspace_id, agent_id, date)
ORDER BY (org_id, workspace_id, agent_id, date)
AS SELECT
    org_id,
    workspace_id,
    agent_id,
    toDate(started_at) AS date,
    countState() AS total_calls,
    sumState(duration_sec) AS total_duration_sec,
    countIfState(disposition = 'completed') AS successful_calls,
    avgState(evaluation_score) AS avg_score,
    avgState(e2e_latency_ms) AS avg_latency_ms,
    countIfState(overall_sentiment = 'positive') AS positive_sentiment_calls,
    countIfState(overall_sentiment = 'negative') AS negative_sentiment_calls
FROM call_events
GROUP BY org_id, workspace_id, agent_id, date;


-- =============================================================================
-- 5. Materialized View: Conversion Funnel Aggregation
-- =============================================================================
-- Outbound campaign funnel: Dialed → Connected → Engaged → Qualified → Converted
-- Used by: Campaign analytics dashboard and ROI reports.
-- =============================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_conversion_funnels
ENGINE = SummingMergeTree()
PRIMARY KEY (org_id, workspace_id, date)
ORDER BY (org_id, workspace_id, date)
AS SELECT
    org_id,
    workspace_id,
    toDate(started_at) AS date,
    count() AS total_dialed,
    countIf(disposition != 'failed' AND disposition != 'no_answer') AS connected_calls,
    countIf(turn_count >= 3) AS engaged_calls,
    countIf(duration_sec >= 60) AS qualified_calls,
    countIf(disposition = 'completed' AND evaluation_score >= 80) AS converted_calls
FROM call_events
GROUP BY org_id, workspace_id, date;

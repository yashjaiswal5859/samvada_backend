# Samvada Voice AI Platform — SOC 2 Type II Compliance & Security Controls

> **Document Classification:** Confidential — Enterprise Security & Compliance  
> **Standard:** AICPA Trust Services Criteria (TSC) — Security, Availability, Processing Integrity, Confidentiality, and Privacy  
> **Platform Version:** Samvada v1.0.0  
> **Last Updated:** 2026-09-27  

---

## 1. Overview & Scope
This document outlines the operational and technical controls implemented across the **Samvada Voice AI Platform** architecture, comprising the four primary microservices (`Auth Service`, `Application & Billing Service (ABS)`, `Reporting Engine`, and `Speech-to-Speech (S2S) Engine`), the NGINX API Gateway, and managed data stores (PostgreSQL, Redis, ClickHouse, Apache Kafka, and MinIO).

---

## 2. Trust Services Criteria (TSC) Mapping

### CC1: Control Environment & Governance
* **Control CC1.1 (Separation of Duties & RBAC):** Role-Based Access Control is enforced at both the API and database levels. Roles (`owner`, `admin`, `member`, `billing_admin`) dictate privileges. Standard members cannot access billing or user administration endpoints (`internal/api/middleware/rbac_middleware.go`).
* **Control CC1.2 (Mandatory Multi-Factor Authentication):** 2FA using TOTP (Time-based One-Time Passwords) is supported and enforced for all administrative and billing actions.
* **Control CC1.3 (Security Code Reviews):** Branch protection rules require automated CI pass (`backend-verification`, `frontend-verification`, `owasp-security-scan`) and peer reviews prior to merging into `main`.

### CC2: Communication & Transparency
* **Control CC2.1 (Incident Escalation):** Automated Prometheus alerting rules (`alerts.yml`) trigger PagerDuty and Opsgenie escalations on P99 latency breaches (>500ms), 5xx error spikes (>1%), and Kafka consumer lag (>10K records).
* **Control CC2.2 (Security Runbooks):** Standard operating procedures and runbooks are maintained for service outage, latency spikes, and credential compromise.

### CC3: Risk Assessment & Vulnerability Management
* **Control CC3.1 (Automated Dependency Scanning):** Every Pull Request triggers automated vulnerability audits using `govulncheck` across all Go modules, `trivy-action` for container and filesystem scanning, and `npm audit` for frontend bundles (`.github/workflows/ci.yml`).
* **Control CC3.2 (Automated Penetration Testing):** Regression test suites in `backend/abs/internal/tests/pen_test.go` validate tenant boundary enforcement, JWT tampering rejection, SQL injection immunity, and negative balance exploit prevention.

### CC4: Monitoring & Operational Observability
* **Control CC4.1 (Distributed Tracing):** W3C TraceContext standards (`traceparent`, `X-Trace-ID`, `X-Correlation-ID`) are injected at the API Gateway and propagated across all microservices (`backend/abs/internal/pkg/telemetry/tracer.go`).
* **Control CC4.2 (Metrics & Dashboards):** Grafana dashboards monitor Golden Signals (P50/P90/P99 latency, traffic throughput, error rate, Kafka lag, ClickHouse query speed, and Redis cache hit ratio).
* **Control CC4.3 (Centralized Log Aggregation):** Structured JSON logs are captured by Promtail and indexed in Grafana Loki with correlation and trace IDs.

### CC5: Logical Multi-Tenancy & Data Isolation
* **Control CC5.1 (Tenant Data Boundary Enforcement):** Every database query is scoped by `org_id` and `workspace_id`. Cross-tenant data access is rejected with HTTP 403 Forbidden.
* **Control CC5.2 (SQL Injection Prevention):** Zero raw string concatenations in database access. 100% of PostgreSQL queries use GORM parameterized placeholders, and ClickHouse queries use native prepared statements.
* **Control CC5.3 (Rate Limiting & DoS Protection):** Token-bucket rate limiting (120 req/min default per user/IP) prevents API abuse and denial-of-service attempts (`internal/api/middleware/rate_limit.go`).
* **Control CC5.4 (Request Body Limits):** API payloads are capped at 1MB, and multipart file uploads (Knowledge Base documents and avatars) are strictly capped at 10MB (`internal/api/middleware/body_limit.go`).

### CC6: Logical Access Controls & Cryptography
* **Control CC6.1 (Data in Transit Encryption):** All external traffic is enforced over TLS 1.3 / TLS 1.2 with HSTS (`Strict-Transport-Security: max-age=31536000; includeSubDomains; preload`). Plaintext HTTP requests on port 80 are redirected with HTTP 301 to HTTPS.
* **Control CC6.2 (Data at Rest Encryption):** Storage volumes for PostgreSQL, ClickHouse, and MinIO use AES-256 filesystem-level encryption (LUKS / dm-crypt) and cloud KMS customer-managed keys.
* **Control CC6.3 (Ephemeral Call Ticket Nonces):** Real-time WebSocket connections require single-use, cryptographically generated ticket nonces with a 60-second TTL stored in Redis to prevent replay attacks.

### CC7: System Availability & Resilience
* **Control CC7.1 (Horizontal Pod Autoscaling):** S2S Speech Engine and ABS microservices scale dynamically via Kubernetes HPA manifests (`backend/infra/k8s/helm/samvada/templates/hpa-s2s.yaml`).
* **Control CC7.2 (Circuit Breaker & Fallback):** S2S clients employ circuit breaker patterns with exponential backoff and a 24-hour local disk cache fallback to maintain uninterrupted telephony operations during transient ABS network outages (`abs_client.go`).
* **Control CC7.3 (Database Replication & Failover):** PostgreSQL read-replicas are configured via `gorm.io/plugin/dbresolver` with automatic read offloading and primary failover support.

### CC8: Change Management & Traceability
* **Control CC8.1 (Immutable Deployments):** Container images are tagged with Git SHA hashes and signed. No in-place updates are performed on production nodes.
* **Control CC8.2 (Database Migrations):** All schema alterations are version-controlled using sequential migration files (`000001_...up.sql` through `000011_...up.sql`) with verified rollback scripts.

### CC9: Data Protection, Privacy & GDPR
* **Control CC9.1 (GDPR Data Portability):** End-users can request an automated machine-readable JSON archive of all personal information, call recordings metadata, and activity history via `GET /api/v1/user/data-export`.
* **Control CC9.2 (GDPR Right to Erasure):** Account deletion API (`DELETE /api/v1/user/data-deletion`) permanently anonymizes personally identifiable information (PII) and schedules recording purges.
* **Control CC9.3 (Data Retention Policies):** Call audio recordings and transcripts older than 730 days (2 years) are automatically purged via MinIO Object Lifecycle Management (ILM) policies.
* **Control CC9.4 (Administrative Audit Trail):** All administrative actions (user invitation, role change, workspace creation, API key generation, and billing top-up) are permanently logged to the `audit_logs` table (`GET /api/v1/audit-logs`) with caller IP, timestamp, and correlation ID.

---

## 3. Compliance Verification Checklist

| Control ID | Description | Automated Verification Artifact | Status |
|:---|:---|:---|:---:|
| **SEC-01** | HTTPS & TLS 1.3 Strict Enforcement | `backend/infra/gateway/nginx.conf` | **COMPLIANT** |
| **SEC-02** | Automated OWASP & Vulnerability Scans | `.github/workflows/ci.yml` (Trivy + Govulncheck) | **COMPLIANT** |
| **SEC-03** | Multi-Tenant Data Isolation & Pen Tests | `backend/abs/internal/tests/pen_test.go` | **COMPLIANT** |
| **SEC-04** | Distributed Tracing (W3C TraceContext) | `backend/abs/internal/pkg/telemetry/tracer.go` | **COMPLIANT** |
| **SEC-05** | SLI & Alerting Rules | `backend/infra/observability/prometheus/alerts.yml` | **COMPLIANT** |
| **SEC-06** | Real-time SLI Monitoring Dashboards | `backend/infra/observability/grafana/dashboards/` | **COMPLIANT** |
| **SEC-07** | Centralized Log Aggregation | `backend/infra/observability/loki/` | **COMPLIANT** |
| **SEC-08** | Composite Database Query Optimization | Migration `000011_add_performance_indexes.up.sql` | **COMPLIANT** |
| **SEC-09** | Redis Cache Hit Ratio Monitoring (>90%) | `internal/api/middleware/metrics.go` (`redis_cache_hit_ratio`) | **COMPLIANT** |
| **SEC-10** | ClickHouse Query Latency SLO (<100ms) | `internal/infrastructure/clickhouse/repos/slo_benchmark_test.go` | **COMPLIANT** |
| **SEC-11** | GDPR Export & Deletion Rights | `profile_service.go`, `ProfileSettingsPage.tsx` | **COMPLIANT** |
| **SEC-12** | Immutable Audit Log System | Migration `000010_create_audit_logs.up.sql`, `AuditLogsPage.tsx` | **COMPLIANT** |

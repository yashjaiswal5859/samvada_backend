# Samvada Platform Test Suite

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![K6](https://img.shields.io/badge/K6-Load_Testing-7D64FF?style=flat&logo=k6)](https://k6.io/)

> **End-to-End Integration, Security, and Load Testing Framework**

---

## 🧪 Test Matrix

| Category | Location | Runner | Description |
|---|---|---|---|
| **E2E Platform Suite** | `e2e/scratch_e2e_runner.go` | `go run` | 46 automated integration assertions across Auth, ABS, Reporting, Kafka, and Gateway |
| **Auth Security Tests** | `e2e/test_production_auth_errors.go` | `go run` | Validates error payloads, rate limiting, and malicious payload rejection |
| **Cookie Flush Tests** | `e2e/test_cookie_flush.go` | `go run` | Tests refresh token expiration and revocation semantics |
| **Performance / Load** | `load/k6_platform_test.js` | `k6 run` | High-concurrency stress test for HTTP/WebSocket endpoints |

---

## 🚀 Running the Tests

### 1. Run the 46-Step Integration Test Suite
Ensure the platform is running (`docker compose -f ../docker-compose.all.yml up -d`):

```bash
cd backend/tests
go run e2e/scratch_e2e_runner.go
```

The runner verifies:
- Health check endpoints (`/healthz`) on all services
- OTP request & verification flow
- JWT cookie setting and rotation
- Workspace creation and RBAC permission checks
- Agent creation and telephony route assignments
- Kafka event production and ClickHouse consumption

### 2. Run K6 Load Tests
```bash
k6 run load/k6_platform_test.js
```


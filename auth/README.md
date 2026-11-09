# Samvada Auth & Identity Service

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Ory Ecosystem](https://img.shields.io/badge/Ory-Kratos_|_Hydra_|_Keto-blue)](https://www.ory.sh/)
[![Port](https://img.shields.io/badge/Port-8081-orange)](#)

> **Enterprise-Grade Multi-Tenant Authentication & Authorization Service**  
> Integrated with the Ory Open Source Identity stack, supporting passwordless OTP login, OAuth2/OIDC token rotation, and Relationship-Based Access Control (ReBAC).

---

## 🔒 Security Architecture Highlights

1. **Passwordless OTP Login:** Secure 6-digit one-time passcodes delivered via email with HMAC-SHA256 verification and replay protection.
2. **Zero JWT in Local Storage:** Access tokens are short-lived (5m). Refresh tokens are stored exclusively in `HttpOnly; SameSite=Lax; Secure` cookies scoped to `/api/v1/auth/refresh`.
3. **Breach Detection & Token Rotation:** Every refresh request issues a new token pair and invalidates previous refresh tokens. Reusing an old refresh token instantly terminates all sessions for that identity.
4. **Relationship-Based Access Control (ReBAC):** Fine-grained permission checks powered by Ory Keto (`configs/keto/keto.yml`).
5. **Multi-Tenant Workspace Context:** Enforces organization-level and workspace-level permission gates on all endpoints via `X-Workspace-ID` and `X-Org-ID` headers.

---

## 📁 Package Structure

```
auth/
├── cmd/
│   ├── auth-service/main.go   # API server entrypoint (:8081)
│   └── migrate/main.go        # Database migration runner
├── configs/                   # Ory Kratos, Hydra, and Keto YAML configurations
├── deployments/compose/       # Standalone Docker Compose for Ory stack
├── docs/                      # OpenAPI 3.0 spec, Swagger UI, and architecture diagrams
├── internal/
│   ├── api/                   # Handlers, DTOs, Routes, and Middleware
│   ├── application/           # Login, Session Cleanup, and Onboarding services
│   ├── domain/                # User, Workspace, Session, and Event entities
│   ├── infrastructure/        # PostgreSQL repositories, Redis caching, Ory providers
│   └── platform/              # Identity orchestration and token management
├── migrations/                # 8 versioned up/down SQL migration scripts
└── Makefile                   # Developer CLI targets
```

---

## 📡 API Endpoints & `curl` Examples

### 1. Request Passwordless OTP
```bash
curl -X POST http://localhost:8081/api/v1/auth/request-otp \
  -H "Content-Type: application/json" \
  -d '{
    "email": "operator@samvada.ai"
  }'
```
**Response (`200 OK`):**
```json
{
  "success": true,
  "correlation_id": "corr-8f92a10c",
  "data": {
    "message": "OTP sent successfully to email",
    "expires_in_sec": 300
  }
}
```

### 2. Verify OTP & Issue Token Pair
```bash
curl -X POST http://localhost:8081/api/v1/auth/verify-otp \
  -H "Content-Type: application/json" \
  -d '{
    "email": "operator@samvada.ai",
    "otp": "274916"
  }' \
  -c cookies.txt
```
**Response (`200 OK` + `Set-Cookie: refresh_token=...; HttpOnly; SameSite=Lax`):**
```json
{
  "success": true,
  "correlation_id": "corr-8f92a10c",
  "data": {
    "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
    "token_type": "Bearer",
    "expires_in": 300,
    "user": {
      "id": "u-9428-abc",
      "email": "operator@samvada.ai",
      "name": "Operator Lead",
      "role": "org_admin"
    }
  }
}
```

### 3. Silent Token Refresh (Cookie Scoped)
```bash
curl -X POST http://localhost:8081/api/v1/auth/refresh \
  -b cookies.txt \
  -c cookies.txt
```

### 4. Fetch Current Identity Profile
```bash
curl -X GET http://localhost:8081/api/v1/user/me \
  -H "Authorization: Bearer <ACCESS_TOKEN>" \
  -H "X-Workspace-ID: ws-consumer-care"
```

---

## 🛠️ Developer Commands

```bash
# Run unit & handler tests
make test

# Run tests with race detection
go test -v -race ./...

# Apply database migrations
make migrate-up

# Rollback latest database migration
make migrate-down

# Build Docker container (distroless minimal image)
make docker-build
```

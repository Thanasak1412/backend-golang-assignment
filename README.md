# Backend Golang Coding Test

Two independent parts: **(1) User Management REST API (implemented in Go)** and **(2) Lottery Ticket Search System (design only)**.

> **Observed verification (2026-10-27, Asia/Bangkok):** Independently re-ran `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...` successfully on Go 1.27.1. Docker Compose build/start, live HTTP/MongoDB smoke checks, the 30-request duplicate-email race (one 201, 29 conflicts), the unique index, worker timing, and API shutdown/restart all passed. `go.sum` is tracked in Git. The assignment files are committed locally in focused groups; publishing and recipient-access confirmation remain pending. See [executed evidence and limitations](docs/verification.md). Lottery remains **design only**. The Docker builder was subsequently updated to Go 1.27.3; that image has not yet been rebuilt with the new pin.

## Deliverables

- Go HTTP API with MongoDB, HS256 JWT authentication, bcrypt password hashes, input validation, and JSON error responses.
- Request logger (method, path, status, duration), 10-second user counter, signal-aware shutdown.
- Repository port / MongoDB adapter; mock-based service and HTTP tests.
- Local Docker Compose stack (API + MongoDB).
- Lottery design **without implementation**: [`docs/lottery-system-design.md`](docs/lottery-system-design.md).
- Architecture, test plan, and implementation status: [`docs/user-management-design.md`](docs/user-management-design.md), [`docs/test-plan.md`](docs/test-plan.md), [`docs/implementation-plan.md`](docs/implementation-plan.md).

## Prerequisites

- Go **1.25+** for direct builds (MongoDB Go Driver v2.9.x requires Go >= 1.25). The Docker builder is pinned to Go **1.27.3**.
- Docker Engine and Docker Compose plugin for container setup; or a local MongoDB server for direct Go run.
- Network access for first-time module downloads.

## Quick start (Docker)

```bash
test -e .env || cp .env.example .env
# For a new .env, replace JWT_SECRET with an unpredictable secret.
# Keep any existing valid local secret:
# openssl rand -hex 32
# Do not commit .env.
docker compose up --build -d
curl -i http://127.0.0.1:8080/healthz
docker compose logs --tail=100 api
```

Run the commands from the repository root. Startup waits for MongoDB health; retry the health probe briefly if the HTTP listener is still starting. To run the live checks on this local development stack:

```bash
python3 scripts/smoke_test.py --base-url http://127.0.0.1:8080
```

The smoke script creates temporary users and cleans them up when possible.

Compose binds the API to `127.0.0.1:8080` and runs MongoDB without externally publishing port 27017. This is a local evaluation stack, **not** production deployment hardening.

## Direct run (Go)

```bash
test -e .env || cp .env.example .env
# IMPORTANT: environment variables are NOT loaded automatically from .env by Go.
# Export them in your shell or use your preferred env-loading tool.
export MONGODB_URI=mongodb://localhost:27017
export MONGODB_DATABASE=user_management
export JWT_SECRET="$(openssl rand -hex 32)"
export JWT_ISSUER=backend-golang-assignment
export JWT_TTL_HOURS=1
export HTTP_ADDR=:8080

go mod download   # uses the supplied go.mod and go.sum

go run ./cmd/api
```

For local MongoDB without Docker, start `mongod` separately. At startup the API pings MongoDB and creates a unique index on `users.email`; a failed connection/index creation fails startup.

### Environment variables

The defaults below apply to direct Go execution. Compose reads `JWT_SECRET` from the shell or `.env` and sets the MongoDB URI/database, issuer, token lifetime, and HTTP bind address explicitly in `compose.yaml`; editing those other `.env` values does not override the Compose configuration.

| Variable | Required | Default | Explanation |
|---|---|---|---|
| `JWT_SECRET` | Yes | none | Random secret at least 32 **bytes**, HS256 signing; never commit |
| `MONGODB_URI` | No | `mongodb://localhost:27017` | Connection string |
| `MONGODB_DATABASE` | No | `user_management` | Database name |
| `HTTP_ADDR` | No | `:8080` | Bind address |
| `JWT_ISSUER` | No | `backend-golang-assignment` | `iss` claim |
| `JWT_TTL_HOURS` | No | `1` | Token lifespan, 1–24 hours |

## Authentication & JWT guide

1. **Register**: `POST /api/v1/auth/register` with name, email, password (at least 8 and at most 72 bytes).
2. **Login**: `POST /api/v1/auth/login` with email and password; copy `access_token` from JSON response.
3. Send `Authorization: Bearer <access_token>` to every `/api/v1/users` endpoint.
4. JWT is signed with HS256, includes `sub` (MongoDB user ObjectID), `iss`, `iat`, `exp`. Middleware checks signature, algorithm, issuer, expiration, and current user existence.
5. Login again after expiration. Refresh tokens, token revocation lists, rate limiting, and role management are beyond this exercise.

### API endpoints

| Method | Path | Auth | Response |
|---|---|---|---|
| GET | `/healthz` | No | `200` |
| POST | `/api/v1/auth/register` | No | `201`, `409`, `400` |
| POST | `/api/v1/auth/login` | No | `200`, `401`, `400` |
| POST | `/api/v1/users` | JWT | `201`, `409`, `400`, `401` |
| GET | `/api/v1/users` | JWT | `200`, `400`, `401` |
| GET | `/api/v1/users/{id}` | JWT | `200`, `400`, `401`, `404` |
| PATCH | `/api/v1/users/{id}` | JWT; own account | `200`, `400`, `401`, `403`, `404`, `409` |
| DELETE | `/api/v1/users/{id}` | JWT; own account | `204`, `401`, `403`, `404` |

`GET /api/v1/users?page=1&limit=20` paginates with `1 <= limit <= 100`. IDs are 24-character MongoDB ObjectID hex strings.

### Sample requests and responses

**Register**

```bash
curl -i -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"Alice","email":"alice@example.com","password":"secure-password123"}'
```

Expected HTTP 201:

```json
{"user":{"id":"507f1f77bcf86cd799439011","name":"Alice","email":"alice@example.com","created_at":"2026-10-27T00:00:00Z"}}
```

The ID and timestamp are illustrative; actual values are generated. The password hash is **never** serialized.

**Login**

```bash
curl -i -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"secure-password123"}'
```

Expected HTTP 200:

```json
{"access_token":"<signed-HS256-JWT>","token_type":"Bearer"}
```

**Fetch and list**

```bash
TOKEN='<access_token-from-login>'
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8080/api/v1/users?page=1&limit=20"
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/users/507f1f77bcf86cd799439011
```

**Create another user (authenticated demonstration operation)**

```bash
curl -i -X POST http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Bob","email":"bob@example.com","password":"secure-password123"}'
```

**Update own name/email**

Replace `<your-user-id>` in the update/delete URLs with the ID returned by registration.

```bash
curl -i -X PATCH "http://localhost:8080/api/v1/users/<your-user-id>" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Alice Updated"}'
```

**Delete own account**

```bash
curl -i -X DELETE "http://localhost:8080/api/v1/users/<your-user-id>" \
  -H "Authorization: Bearer $TOKEN"
```

Expected HTTP 204 (no response body). After deletion the old token no longer authorizes operations because middleware checks the user exists.

**Error example (duplicate email)**

```json
{"error":{"code":"conflict","message":"email already exists"}}
```

### Core design assumptions and decisions

- **Authorization ambiguity:** All authenticated users may create and read user records for this assignment; update/delete are restricted to one's own account. This is an intentionally simplified demo policy, **not** a safe general-purpose production directory policy. A production system needs roles/permissions, privacy controls, and registration policy.
- Registration and authenticated `POST /users` use the same UserService business logic.
- Emails are trimmed/lowercased, enforced unique by a **MongoDB unique index** (not just a preflight read). Case-folding international email addresses has additional complexities beyond scope.
- Passwords use bcrypt (`DefaultCost`) and the 72-byte bcrypt input limit is validated.
- Deletes are hard deletes (no audit retention specified); no password-reset endpoint.
- `GET /users` uses bounded offset pagination (appropriate for demo; cursor pagination is preferable at high scale).
- Database timestamps are UTC; unknown JSON fields are rejected, JSON bodies limited to 1MiB.
- The 10-second worker issues actual `CountDocuments` with a 5-second timeout and exits on context cancellation.

## Run tests

```bash
go mod tidy
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

`go test ./...` uses in-memory fake repositories; **no MongoDB required** for unit/handler tests. The local Compose integration smoke test passed on 2026-10-27; see [verification evidence](docs/verification.md). These checks do not establish production readiness.

## Final verification before submission

- [x] Go 1.27.1 installed; supplied `go.mod` and `go.sum` preserved. Candidate reports `go mod tidy` passed; this session did not re-run it against local files.
- [x] `go test ./...` succeeds.
- [x] `go test -race ./...` succeeds.
- [x] `go vet ./...` and `go build ./...` succeed.
- [x] The previous Docker Compose build/start passed with cached layers and `/healthz` returned HTTP 200. The builder has since moved to Go 1.27.3; rebuilding that image is pending.
- [x] Register/login, create/get/list/update/delete tested with live MongoDB; response secret checks pass.
- [x] 30 concurrent registrations: exactly one HTTP 201 and 29 HTTP 409; MongoDB `unique_email` has `unique: true`.
- [x] Live routes reject missing/malformed/deleted-user JWT; unit tests reject tampered/expired/wrong-algorithm JWT.
- [x] Worker logs `users total` approximately every 10 seconds; idle graceful stop exits 0 in 0.222 seconds; restart returns HTTP 200.
- [x] `go.sum` tracked and committed locally; `.env` ignored and excluded from Git history.
- [x] Lottery document included as design only; no lottery implementation added.
- [x] Commit assignment files in focused, reviewable groups on the existing local branch.
- [ ] Publish to the candidate-authorized destination after explicit approval; confirm recipient access.
- [ ] Candidate confirms actual deadline/time zone and assessment AI-use/disclosure requirements.

Verification used **cached Docker layers and a reused MongoDB volume**. Graceful shutdown was checked while idle; **shutdown under load was not tested**.

All checked results refer to the existing local development stack. See [full verification record](docs/verification.md) for command outcomes and remaining limits.

## References

- Official MongoDB Go Driver: https://www.mongodb.com/docs/drivers/go/current/
- golang-jwt Parser Options: https://pkg.go.dev/github.com/golang-jwt/jwt/v5
- PostgreSQL locking reference (lottery proposal): https://www.postgresql.org/docs/current/sql-select.html

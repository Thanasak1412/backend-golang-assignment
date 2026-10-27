# User Management API — Technical Design

## Scope and boundaries

Implementation of registration, login with HS256 JWT, protected user CRUD, MongoDB persistence, HTTP method/path/latency logging, and a user-counter goroutine. Optional features selected for their payoff: validation, hexagonal-ish ports/adapters, Docker, and graceful shutdown. **gRPC is intentionally excluded** to prioritize mandatory correctness under the deadline.

## Components and dependency direction

```text
client -> net/http router -> HTTP handlers / auth middleware
                            -> UserService / TokenService
                            -> domain.UserRepository interface
                            -> MongoDB repository adapter -> MongoDB

main.go owns config, dependencies, HTTP server lifecycle, and 10s worker.
```

The service depends only on domain ports, not MongoDB or net/http. HTTP handlers depend on services; MongoDB adapter depends on `domain`. A mock repository is sufficient to unit-test application logic. This is a practical lightweight ports-and-adapters design, rather than a full use-case class hierarchy.

## Domain model

| Property | Type | Constraint |
|---|---|---|
| `_id` | BSON ObjectID | auto-generated, immutable |
| `name` | string | trimmed, 1–100 Unicode code points |
| `email` | string | trimmed, lowercase, valid mail address, <=254 bytes; unique index |
| `password_hash` | string | bcrypt, >=8 and <=72 plaintext bytes |
| `created_at` | BSON datetime | server-side UTC creation timestamp |

`domain.User.PasswordHash` uses `json:"-"`; HTTP responses cannot serialize hashes accidentally.

## API and authorization

See README endpoint table. For this exercise, anonymous registration/login; all JWT-authenticated users may create and read users; user updates/deletes require `token.sub == path.id`. This balances a meaningful self-service access control with the assignment's unspecified permissions. It is **not a recommended policy for production PII**. Errors are mapped to 400/401/403/404/409/500 and database internals are not exposed to clients.

## Authentication and security

- bcrypt hashes; no plaintext or hash in API responses.
- JWT HS256 with key length >=32 bytes, `exp`, `iss`, `iat`, and MongoDB hex ObjectID in `sub`.
- Middleware requires `Authorization: Bearer ...`, validates HS256 *explicitly*, signature, expiration and issuer, then loads the current user; deleted users lose access.
- Limit JSON request bodies to 1MiB; reject unknown fields; email normalization plus database uniqueness resolves race conditions.
- All operations receive request contexts. MongoDB errors are logged as generic 5xx (logging enhancement possible); failures are not leaked in error JSON.
- Out of scope: rate limiting, refresh tokens, distributed token revocation, RBAC, password recovery, secrets management, TLS termination, login timing equalization, tracing.

## MongoDB and indexes

Use official `go.mongodb.org/mongo-driver/v2` and collection `users`. On startup `EnsureIndexes` creates unique ascending index `unique_email`. An attempted duplicate insert or email update is mapped to `domain.ErrConflict`. List query sorts deterministically by `created_at`, `_id` and uses `limit`/`offset`; large-offset queries could be optimized with keyset pagination.

## Reliability and lifecycle

`main` creates a signal-aware context, pings MongoDB, creates indexes, and then launches HTTP and `UserCounter.Run`. The worker has a single ticker with a 10-second interval, sequential count queries (no goroutine pile-up) and per-query 5s timeout. SIGINT/SIGTERM trigger HTTP server shutdown; the worker is joined before MongoDB disconnects. The HTTP server has read-header, read, write, and idle timeouts.

## Design trade-offs

| Decision | Advantage | Limitation |
|---|---|---|
| net/http routing | no web framework, demonstrates idiomatic Go | no framework conveniences |
| Repository interfaces | test service without DB | adds one abstraction layer |
| bcrypt | well-understood password hash | 72-byte input limit; work factor cost |
| Stateless JWT | simple scalable auth | tokens can't be selectively revoked without state; deleted-user DB lookup partially mitigates |
| Offset pagination | simple evaluation contract | large offsets slow |
| MongoDB direct count / 10s | meets exact assignment requirement | cost grows with collection size |

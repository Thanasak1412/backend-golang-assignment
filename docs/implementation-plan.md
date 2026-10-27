# Implementation Plan — Deadline-Oriented

**Deadline:** 2026-10-28 (confirm actual submission cutoff/time zone with recruiter). **Owner:** candidate; **AI:** draft/implementation assistant, subject to assessment's AI-usage policy. Scope is split between User API implementation and **lottery design only**.

## Observed status — 2026-10-27

Phase 4 local integration and Go checks passed. Phase 5 documentation and assignment files are committed locally in focused groups on the existing branch. Push, recipient-access confirmation, deadline confirmation, and assessment policy review remain pending. See [command evidence](verification.md). No core API changes were necessary; lottery remains design only.

## Priorities

- **P0 (mandatory):** User model, registration/login with JWT, protected CRUD, MongoDB via official driver, HTTP timing logger, goroutine count every 10s, Go unit tests, README, lottery design document.
- **P1 (high-value bonuses):** Repository interfaces, input validation, Docker Compose, graceful shutdown, lightweight hexagonal layout.
- **P2 (optional, deliberately deferred):** gRPC CreateUser/GetUser and full proto pipeline. Never sacrifice P0 correctness for P2.

## Phases and acceptance criteria

### Phase 1: Initialize and design

- [x] Go module / package boundaries (`domain` -> ports, `service` -> application, `adapters` -> infrastructure)
- [x] Config validation + `.env.example`
- [x] MongoDB adapter and unique-email index creation
- [x] Dockerfile / Compose skeleton
- [x] Candidate reports `go mod tidy` passed; supplied `go.mod`/`go.sum` preserved. Local tests/build independently passed; local tidy was not repeated.

**Accept when:** app starts with valid DB/config, rejects weak JWT secret, fails loudly on connection/index error. Local Compose startup, health, and index verification passed; weak-secret unit tests passed.

### Phase 2: Core features

- [x] Register, Login, JWT HS256 (enforced method, exp/iat/iss/sub)
- [x] CRUD via protected `/api/v1/users` endpoints
- [x] Explicit authorization policy: self update/delete; authenticated create/read
- [x] Email normalization + unique DB index; bcrypt password hashes
- [x] Basic validation and consistent JSON errors
- [x] Live smoke test passed; 30 concurrent duplicate-email requests yielded one 201 and 29 conflicts.

**Accept when:** all endpoint status codes match README, no password hashes in responses, same email cannot be registered twice even concurrently.

### Phase 3: Reliability

- [x] HTTP method/path/status/duration structured logging
- [x] Count users every 10 seconds using a single goroutine / time.Ticker
- [x] Context cancellation, request timeouts, SIGINT/SIGTERM graceful shutdown
- [x] Observed repeated ~10-second logs; idle Docker stop exited 0 in 0.222 seconds; API restarted and health passed.

**Accept when:** worker does not leak, no DB access after disconnect, shutdown drains HTTP operations within timeout. Worker cancellation/race tests and idle live shutdown passed; in-flight shutdown drain under load was not separately exercised.

### Phase 4: Test and review

- [x] Mock-based service tests (register/login/CRUD/invalid input/db errors)
- [x] Handler tests (security, status, invalid JSON)
- [x] JWT tests (tamper, expiry, wrong algorithm)
- [x] Worker cancellation test
- [x] `gofmt` source validation and config unit test in offline Go 1.23 environment
- [x] Independently executed `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...` on Go 1.27.1; all exited 0. Earlier authoring-environment blockers no longer apply to this local run.
- [x] Real MongoDB/Compose smoke test exited 0 with `SUCCESS`; unique index and cleanup verified.

**Accept when:** all build/test commands pass on Go1.25+, and test outcomes are recorded accurately.

### Phase 5: Final documentation & delivery

- [x] README based on written implementation + API samples
- [x] User-management technical design
- [x] Lottery design only — PostgreSQL + index + atomic reservation + performance tradeoffs
- [x] Test plan and AGENTS.md for Codex follow-up
- [x] Keep README response IDs/timestamps explicitly illustrative; record actual executed results in `docs/verification.md`.
- [x] Preserve candidate-generated `go.sum` in the local commit series; `.env` excluded from Git history.
- [x] Commit reviewed source, tests, setup, and documentation in focused local commits.
- [ ] Push to the candidate-controlled remote after explicit approval and confirm recipient access. The remote initially contained only its README.
- [ ] Verify assessment AI-usage restrictions and any disclosure requirements

**Accept when:** clean Git checkout passes README setup and every doc accurately states verified vs unverified claims.

## Hard stop / submission order

1. First resolve `go mod tidy` and all compilation errors on Go1.25+ (no silent omissions).
2. Run unit tests + race detector; resolve failures.
3. Run Docker API/MongoDB health + register/login/CRUD smoke sequence, capture actual results.
4. Verify Lottery proposal invariants and absence of implementation.
5. Review the local commit series. Push to the candidate-authorized destination only after explicit approval, then confirm recipient access before cutoff. No push was performed.

## Codex scope rules

Run each phase as an independent task. After each: summarize changed files, run the relevant tests, state actual output, and avoid changes outside assigned phase. No changes to lottery code (there should be none). Do not fabricate test evidence, do not embed secrets in source, and do not add gRPC unless P0 is complete and user explicitly prioritizes it.

# Test Plan and Quality Gates

## Unit tests (fake repository; no MongoDB required)

| Case | Expected result | File |
|---|---|---|
| Valid register + login | email normalized, password hashed, token/auth works | `internal/service/user_test.go` |
| Duplicate email | conflict | service test + MongoDB live smoke (passed 2026-10-27) |
| Empty name / invalid email / short or >72-byte password | invalid input | service tests |
| Unknown user / incorrect password | unauthorized | service tests |
| DB error while logging in | propagated as server error, not invalid credentials | service test |
| Get invalid ID | invalid input | service tests |
| Update self / delete self | success, deleted record absent | HTTP and service tests |
| Protected route without token | 401 | HTTP tests |
| Attempt update another user's ID | 403 | HTTP tests |
| Existing token after self-delete | 401 | HTTP tests |
| Extra JSON field / multiple JSON values | 400 | HTTP tests |
| Valid JWT | accepted | token tests |
| Tampered/expired/wrong-HMAC-method JWT | 401 | token tests |
| Invalid JWT secret | creation rejected | token tests, config tests |
| User worker stops on context cancellation | goroutine terminates | worker tests |

## Integration tests (local Compose smoke passed 2026-10-27)

The following is the intended test inventory. Executed coverage and remaining limits are recorded below and in [verification.md](verification.md); not every item is part of the smoke script.

1. Start Compose, wait for `/healthz` 200, confirm collection/index exists.
2. Register email A and login A; verify authenticated list/get.
3. Repeat register A -> 409; PATCH A's email to already-owned B's email -> 409.
4. 30 concurrent register requests with same canonical email should yield exactly one 201, remaining 409; verify collection count increased by exactly one.
5. Register B; try B PATCH/DELETE A -> 403; verify A unchanged.
6. DELETE A -> 204; use A's old access token -> 401.
7. Send invalid JSON, unknown field, invalid ID, negative page, limit >100, missing JWT, malformed JWT and expired JWT; verify status and no internal error leakage.
8. Observe JSON logs for method/path/duration and repeated user count approximately every 10s; signal shutdown and verify orderly exit.
9. Run `go test -race ./...` and `go vet ./...` in an internet-connected Go1.25+ environment.

## Lottery design-only verification

Review the document logically for these **invariants** (no runnable lottery tests were implemented):

- 10 million physical tickets with possibly repeated 6-digit numbers are distinct by ticket ID.
- Pattern `******` / `****23` / `1****5` / `123***` results are mathematically correct.
- Identical and overlapping patterns cannot allocate the same physical ticket ID to two simultaneous users.
- A committed reservation is the only mechanism that makes an item exclusive; search preview is nonexclusive.
- Expired/cancelled reservation cannot confirm a subsequently reassigned ticket (reservation fencing).
- Retries with same request key cannot silently create an extra reservation.
- Database unavailable => fail closed, never promise an uncommitted ticket.

## Actual verification record (2026-10-27)

- **PASS, independently executed:** `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`; each exit 0 with Go 1.27.1 on darwin/arm64.
- **PASS:** `docker compose up --build -d` (cached build); API running, MongoDB healthy; `curl -i http://127.0.0.1:8080/healthz` returned HTTP 200 and `{"status":"ok"}`.
- **PASS:** `python3 scripts/smoke_test.py --base-url http://127.0.0.1:8080` exited 0 and printed `SUCCESS`. Includes register/login, authenticated create/get/list, self update/delete, duplicate-email update conflict, cross-user PATCH/DELETE 403, missing/malformed JWT 401, deleted-user JWT 401, malformed JSON/unknown field/invalid email/ID/pagination 400, and password/hash response checks.
- **PASS:** Default concurrency 30; script asserts exactly one 201 and 29 conflicts. Database counts were zero before the smoke and zero after cleanup; the peak count during the race was not separately queried.
- **PASS:** Live MongoDB index `unique_email`, key `{email: 1}`, `unique: true` observed and asserted through `mongosh`.
- **PASS:** `docker compose logs --tail=100 api` showed method/path/status/duration and repeated `users total` at ~10-second intervals, including after restart.
- **PASS:** `docker compose stop -t 15 api` completed in 0.222 seconds; `docker compose ps -a --format json api` showed `State: exited`, `ExitCode: 0`. API restarted with `docker compose up -d api`; health returned 200.
- **PASS:** Supplied `go.sum` is tracked in a local commit; `.env` is ignored and excluded from Git history. Candidate Go files, module files, and existing smoke script remained byte-for-byte unchanged during verification.
- **CANDIDATE-REPORTED ONLY:** `go mod tidy` success. It was not repeated against local files during this session. Earlier authoring-only gofmt/config checks are historical evidence.
- **LIMITS:** Expired/tampered/wrong-algorithm JWT rejection was checked by passing unit tests, not separate signed live requests. Dedicated cases for multiple JSON values and page-size >100, direct database peak-count sampling, and in-flight shutdown/load behavior were not independently exercised live. No lottery execution or benchmark was performed.
- **PENDING SUBMISSION:** Assignment files are committed locally; no push was made. Confirm the destination branch, recipient access, actual deadline/time zone, and assessment AI-use policy before delivery.

The earlier authoring environment's Go/Docker restrictions do not describe this workstation verification. Full commands, evidence, Git baseline, and remaining risks: [verification.md](verification.md).

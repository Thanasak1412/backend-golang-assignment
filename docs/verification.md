# Phase 4 and Phase 5 verification — 2026-10-27

Executed against `/Users/mac/Documents/backend-golang-assignment` and its existing local Docker Compose development stack on 2026-10-27 (Asia/Bangkok, UTC+07). Container logs use UTC. Candidate-reported earlier Go results were independently re-run as listed below. No core API source, module files, Docker configuration, smoke script, or lottery design was changed.

## Command results

| Command | Result | Observed evidence |
|---|---|---|
| `go version` | PASS, exit 0 | `go version go1.27.1 darwin/arm64` |
| Initial `git status --short`, `git diff`, `git ls-files go.sum .env` | BLOCKED initially | No `.git` existed; Git reported “not a git repository”. There was no local Git diff or staging state to inspect. |
| `ls -l go.sum` | PASS | Existing 4,510-byte candidate file; contents preserved. |
| `.env` preparation | PRESERVED | Existing local file retained; no copy, secret replacement, or secret output. |
| `docker compose up --build -d` | PASS, exit 0 | Image built using cached layers; existing API running; MongoDB healthy. |
| `docker compose ps` | PASS, exit 0 | API running on `127.0.0.1:8080`; MongoDB healthy with no published host port. API has no separate Docker healthcheck; HTTP health was probed directly. |
| `curl --max-time 10 -i http://127.0.0.1:8080/healthz` | PASS, exit 0 | HTTP 200, `{"status":"ok"}`. |
| `python3 scripts/smoke_test.py --base-url http://127.0.0.1:8080` | PASS, exit 0 | `SUCCESS: Live HTTP + MongoDB smoke checks passed.` |
| `docker compose logs --tail=100 api` | PASS, exit 0 | HTTP timing logs and repeated `users total` approximately every 10 seconds; see samples below. |
| `docker compose exec -T mongo mongosh --quiet user_management --eval 'db.users.getIndexes()'` | PASS, exit 0 | `unique_email` on `{email: 1}`, `unique: true`. A subsequent assertion of name, unique flag, and key also exited 0. |
| MongoDB `countDocuments` before/after smoke | PASS, exit 0 | `total: 0`, `smokeUsers: 0` both before and after cleanup. |
| `time docker compose stop -t 15 api` | PASS, exit 0 | `0.222 total`; container stopped without reaching the 15-second limit. |
| `docker compose ps -a --format json api` after stop | PASS, exit 0 | `State: exited`, `ExitCode: 0`. This was an idle shutdown check. |
| `docker compose up -d api` | PASS, exit 0 | API restarted; MongoDB remained running/healthy. |
| Health probe after restart | PASS, exit 0 | HTTP 200, `{"status":"ok"}` at `2026-10-27T13:21:37Z`. |
| `go test ./...` | PASS, exit 0 | httpapi, config, service, worker tests passed; other packages have no test files. |
| `go test -race ./...` | PASS, exit 0 | Same tested packages passed with race detector; no race reported. |
| `go vet ./...` | PASS, exit 0 | No diagnostics. |
| `go build ./...` | PASS, exit 0 | No build errors. |
| `go mod tidy` | CANDIDATE-REPORTED | Not repeated against local files; supplied module files were preserved. Dockerfile's tidy layer was cached. |
| Final `git ls-files --error-unmatch go.sum` | PASS | Candidate `go.sum` is tracked and included in the local API implementation commit. |
| Final `git check-ignore .env` and Git history inspection | PASS | `.env` ignored and absent from tracked paths/history; no local credentials or JWT signing secret committed. |
| Final `git diff --cached --check` | PASS | Staged submission has no whitespace errors. |

Initial sandbox Docker access failed with socket permission denial, and sandbox loopback curl failed to connect. Both succeeded when repeated with approved access; these were environment restrictions, not API failures. Git clone initially failed DNS in the sandbox and succeeded with approved network access.

## Live smoke coverage

The existing script printed 29 PASS lines and exited 0. It checked registration and email normalization, duplicate registration, login and incorrect password, authenticated user creation, get/list, invalid ID and pagination, cross-user PATCH/DELETE rejection, duplicate-email update rejection, self update and login using the changed email, self deletion, deleted-token rejection, deleted-user 404, missing/malformed JWT rejection, malformed/unknown-field JSON, and invalid email rejection. Its password/hash checks passed on registration/create/get/list/update responses.

Observed concurrency output:

```text
PASS  concurrent duplicate-email test (30 requests: 1 created)
SUCCESS: Live HTTP + MongoDB smoke checks passed.
```

The script only prints that PASS after asserting `statuses.count(201) == 1` and `statuses.count(409) == 29` for its default 30 requests. The API logs also show the winning 201 followed by conflicts at `13:17:40Z`. All temporary users were deleted successfully; database counts returned to the initial zero. No separate database query sampled the peak count while the race winner still existed.

Index evidence (no user documents or hashes printed):

```text
{ v: 2, key: { email: 1 }, name: 'unique_email', unique: true }
{ total: 0, smokeUsers: 0 }
```

## Worker and shutdown evidence

After API restart, selected actual log lines were:

```json
{"time":"2026-10-27T13:20:05.98861693Z","level":"INFO","msg":"users total","count":0}
{"time":"2026-10-27T13:20:15.987758462Z","level":"INFO","msg":"users total","count":0}
{"time":"2026-10-27T13:20:25.988225592Z","level":"INFO","msg":"users total","count":0}
```

These intervals are approximately 9.999 and 10.000 seconds. Stop completed in 0.222 seconds with exit code 0; restart restored health and the count worker. No dedicated shutdown log is implemented. This verifies orderly idle termination; draining long-running requests and shutdown during a slow/unavailable database were not separately exercised.

## Git baseline, changed files, and delivery state

The input directory was a source copy without `.git`. After the candidate supplied `https://github.com/Thanasak1412/backend-golang-assignment.git`, read-only inspection and a temporary no-checkout clone established remote baseline `0429e11c6e7f9f4b28c963d1fc5563036a7d9b04` (`Initial commit`), containing only a 27-byte README. Remote `go.sum` did not exist (404).

That Git history was attached locally without checking out or overwriting candidate files. Local branch: `codex/phase45-verification`; origin points to the supplied repository. The first review staged all assignment files, then this commit-sequencing task split them into focused local commits. `.env` and `.DS_Store` were excluded. No push or pull request was made.

During the Phase 4 integration pass, these four project file contents changed from the candidate's starting copy:

- `README.md`: observed verification notice/checklist, safe existing-env setup, and supplied module-file instructions.
- `docs/implementation-plan.md`: mark executed local gates passed and retain pending delivery gates.
- `docs/test-plan.md`: actual results, live versus unit coverage, and explicit limitations.
- `docs/verification.md`: this command/evidence and submission record (new).

Git metadata was added and the reviewed files were initially staged. At that point, candidate project files including Go source/tests, `go.mod`, `go.sum`, `scripts/smoke_test.py`, `.env`, and the lottery document matched the verified starting copy. The initial diff was large because the remote baseline contained only README; it did not reflect a reimplementation during verification.

## Outstanding submission risks

- Submission is not delivered until the reviewed staged files are committed and pushed, and recipient access is confirmed. The remote inspected at the start of delivery verification lacked the application and lottery document.
- Candidate must confirm actual deadline/cutoff/time zone and applicable AI-use/disclosure requirements.
- Verification used an existing local MongoDB volume and cached Docker layers. A fresh no-cache build and fresh-volume setup were not independently tested.
- Expired/tampered/wrong-algorithm JWTs passed unit coverage; separate signed live cases were not run. The smoke script does not cover every test-plan permutation or shutdown under load.
- Lottery remains a design proposal only. Its benchmarks, concurrency invariants, and performance estimates have not been implemented or experimentally verified.

API and MongoDB were left running after verification. No volumes were deleted and no production target was used.


## Final submission review — 2026-10-27

Review outcome: no critical business-logic defect or real credential exposure was found. Application behavior did not require changes. The subsequent commit-sequencing task recorded the assignment files locally in focused commits; nothing was pushed.

### Scope and checks

- Inspected the working tree, Git index, and all reachable commits. Baseline HEAD remains `0429e11c6e7f9f4b28c963d1fc5563036a7d9b04`; the existing remote baseline contains only README.
- Secret inspection covered all 31 staged paths and the one reachable commit, with manual review of credential-related literals plus targeted pattern checks for private keys, access tokens, literal JWTs, and URLs containing passwords. The existing local JWT signing secret was compared in memory against all scanned content without displaying it. No real credential or runtime-secret exposure was found; `.env` is absent from history and the index. Documented passwords, smoke inputs, and test signing keys are deliberately fictitious fixtures, not local or deployed credentials. This was a targeted local review, not an external secret-scanning service audit.
- Confirmed `go.mod`, `go.sum`, `Dockerfile`, `compose.yaml`, `.dockerignore`, `.env.example`, all five `*_test.go` files, and `scripts/smoke_test.py` are staged. README links point to existing deliverables.
- `go mod verify`: executed during this final review, exit 0, `all modules verified`.
- `go list -mod=readonly -m -json go.mongodb.org/mongo-driver/v2`: executed, exit 0; v2.9.2 declares Go 1.25.0, matching the README prerequisite.
- `docker compose config --quiet`: executed, exit 0. This validates configuration only; it does not repeat Docker build/start, health, shutdown, or live smoke tests.
- Compared README routes, validation, authentication, setup, and environment defaults with `httpapi`, configuration, token service, `compose.yaml`, and `Dockerfile`. Corrected command ordering, quoted shell-sensitive sample URLs, documented explicit Compose environment values, included the existing smoke command, and listed pagination HTTP 400.
- Reviewed the lottery proposal for 10M physical tickets versus 1M number values, wildcard patterns/cardinalities, indexes, atomic cross-pattern reservation, TTL, fencing, idempotency, failure cases, complexity, and benchmark limits. Required design topics are present; no runnable lottery code is included. Corrected the unavailable-database trade-off to **correctness over availability**. No lottery implementation or benchmark was run.
- Extended `.gitignore` to exclude `.env.*` while retaining `.env.example`. Existing `.env` content and all MongoDB volumes were preserved.
- Checked README shell examples with `zsh -f -n`; checked curl samples with a shell stub to confirm URL arguments survive zsh expansion. These are syntax/argument checks, not new live API requests.
- Final index checks: `git diff --cached --check` passed; staged content matches the worktree; no unstaged or untracked nonignored files remain. Env-ignore checks passed for `.env`, `.env.local`, and `.env.production`; `.env.example` remains included.

### Changes made by the final review

- `.gitignore`: ignore local environment variants, permit the placeholder example.
- `README.md`: setup/sample corrections, Compose defaults, existing smoke command, and explicit verification limitations.
- `docs/lottery-system-design.md`: one wording correction to the fail-closed trade-off; design-only scope preserved.
- `docs/verification.md`: this final review and clarification that the earlier changed-file list describes the integration pass.

### Final state and submission actions

At the end of the initial review, branch `codex/phase45-verification` had **30 staged additions and one staged README modification** relative to the initial commit. This describes the precommit review state; the files were subsequently committed in focused local groups. Check `git status` and `git log --oneline -n 10` for the current state. The commits were not pushed.

The earlier executed Go tests/race/vet/build and live smoke evidence remains recorded above. Go source/tests, module files, Docker files, smoke script, and local `.env` are unchanged since those successful checks. Those commands were **not re-run** during this documentation-only final review.

Verification limitations remain: **Docker cached layers**, **reused MongoDB volume**, and **graceful shutdown under load not tested**. No fresh-volume or no-cache result is claimed. The local commits still need a reviewed push to the intended destination, then recipient access must be confirmed. The candidate must confirm the submission cutoff and any assessment AI-use/disclosure requirements. Push only after explicit approval.

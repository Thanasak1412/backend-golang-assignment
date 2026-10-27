# Lottery Search System — Production-Oriented Design Proposal

**Scope:** Design only. No lottery code implementation is included. This is independent of the Go/MongoDB API in Part 1.

## 1. Clarify the problem and assumptions

- Inventory comprises **10,000,000 physical tickets**. Every ticket has a globally unique `ticket_id` and a six-character `ticket_number` in `000000..999999`.
- There are **1,000,000 possible six-digit numbers**; at least some numbers repeat across the 10M physical tickets. **Different physical tickets with the same number may be allocated independently.** If the business actually requires uniqueness of the **number**, a stronger per-number constraint must be added.
- Search pattern consists of **exactly six** characters `[0-9*]`. Each `*` matches **one arbitrary digit**, not any-length text. `1****5` matches `100005`, `123455`, etc. Leading zeros are preserved.
- “Must not return the same ticket to multiple users simultaneously” is interpreted as **exclusive active allocation by `ticket_id`**, across both identical and overlapping/different patterns.
- Because even read-only search could otherwise expose the same item to multiple users, clients **do not receive a ticket ID until a single atomic search-and-reserve transaction succeeds**. Unreserved browsing may return aggregate counts, not promises of exclusive tickets.
- Default reservation lifetime: **120 seconds**, configurable. The client may confirm or cancel a valid reservation. Confirmed tickets remain allocated. A random/priority selection policy can be tuned separately.

## 2. Recommended production storage

**PostgreSQL as the authoritative inventory and reservation store.** It provides ACID transactions, fine-grained row locks, conditional updates, indexes, monitoring and operationally mature replication. `SELECT ... FOR UPDATE SKIP LOCKED` enables competing allocators to avoid waiting on currently locked candidate rows. `SKIP LOCKED` deliberately gives an inconsistent view (appropriate for work queue/allocation, not general financial reporting); correctness is established by transactionally changing each selected ticket state, not by treating a search preview as a reservation.

Alternatives:

| Choice | Strength | Trade-off / why not first choice |
|---|---|---|
| PostgreSQL | transactional correctness + indexing together | lock contention and scanning if poorly indexed |
| MongoDB | atomic single-document updates | wildcard positional indexing and high-contention inventory need careful modeling and retry design |
| Redis (cache / bitmap) | fast in-memory candidate discovery | cache alone is not durable source of truth; synchronization complexity |
| Elasticsearch/OpenSearch | complex text search | eventual refresh, operational burden; cannot replace transactional reservation |

Begin with a **modular monolith** (Go HTTP app with Search/Reserve/Confirm modules) and one PostgreSQL primary. Redis or bitmap services are contingent optimizations after measurement, not hard dependencies.

## 3. Data structures

### Ticket (logical schema)

| Column | Concept | Notes |
|---|---|---|
| `ticket_id` | UUID or 64-bit ID (PK) | identity of *physical* ticket |
| `ticket_number` | `CHAR(6)` / constrained text | preserves `000001`, never unique globally |
| `digit_1` ... `digit_6` | digits 0..9 or derived values | convenient selective predicates; stored/generated design choice |
| `status` | AVAILABLE, RESERVED, ALLOCATED | row's current availability |
| `reserved_by` | user ID nullable | ownership of active reservation |
| `reservation_id` | UUID nullable | fencing/ownership token for confirmation |
| `reserved_until` | UTC timestamp nullable | reservation expiry |
| `updated_at` | UTC timestamp | operations / audit support |

### Reservation and audit history

Separate `reservation` table: immutable `reservation_id`, `ticket_id`, `user_id`, `created_at`, `expires_at`, `status` (ACTIVE, CONFIRMED, CANCELLED, EXPIRED), `request_key` (client idempotency key) and event timestamps. Enforce uniqueness of `(user_id, request_key)` for retry deduplication and ensure the user/request context cannot replay a key with a different search pattern. The ticket row is the current-ownership source of truth; reservation history provides evidence and reconciliation. Status changes for ticket and reservation must occur in the **same transaction**.

## 4. Wildcard matching algorithm

Validate pattern length == 6 and characters in `[0-9*]`. Translate every fixed digit into an equality predicate on its position; skip `*`. For `1****5`, predicate is `(digit_1=1 AND digit_6=5)`; for `****23` it is `(digit_5=2 AND digit_6=3)`. The predicate always includes available/expired-reservable state when selecting allocation candidates.

For `k` fixed digits there are `10^(6-k)` matching **number values**, assuming unrestricted `000000..999999`. With uniformly distributed ten copies per number, the expected candidate **ticket** count is `10 × 10^(6-k)`:

| Pattern | Fixed digits `k` | Matching number values | Approx matching tickets |
|---|---:|---:|---:|
| `123456` | 6 | 1 | 10 |
| `123***` | 3 | 1,000 | 10,000 |
| `1****5` | 2 | 10,000 | 100,000 |
| `****23` | 2 | 10,000 | 100,000 |
| `******` | 0 | 1,000,000 | 10,000,000 |

These are **cardinality estimates, not verified query results**; inventory may be skewed and availability further reduces results.

### Database indexing — recommended baseline

- Unique B-tree primary key on `ticket_id`.
- B-tree index for prefix patterns on `ticket_number`, e.g. `123***` can be expressed as bounded lexical range for fixed-width, normalized six-digit ASCII strings.
- Partial or composite indexes on `status` and frequently queried number-prefix/equality columns to reduce candidates; test whether the query planner uses them. Consider composite `(status, digit_1, digit_6, ticket_id)` for the common `1****5` class or digit-pair indexes for frequent templates.
- For arbitrary positions, consider indexes on individual `digit_i` and PostgreSQL Bitmap Index AND or selective combinations. A single digit is only ~10% selective, so six independent indexes may cost more in write amplification/storage than they save. **Do not build all combinations** (2^6 position sets, with many digit permutations) blindly.
- Keep index choices workload-driven: sample real pattern frequencies, analyze with `EXPLAIN (ANALYZE, BUFFERS)` over 10M seeded rows and mixed statuses, observe real plans and p95/p99.
- Query returns only a bounded batch of IDs or selects one immediately; **never return 10M full rows** for `******`. Pagination is for nonexclusive browsing/aggregate results only.

### Optional advanced bitmap optimization

Index **distinct number values**, not 10M physical tickets, in 60 positional bitmaps: 6 positions × 10 digits. Each bitmap has 1M bits, about **125 KB uncompressed**; all 60 together ~**7.5 MB** plus metadata. For `1****5`, AND bitmaps `(position1=1)` and `(position6=5)`; the matching number list then looks up available ticket IDs by `ticket_number` in PostgreSQL. For `k` fixed positions, naive bitmap intersection requires roughly `O(k × U / word_bits)` machine-word work, where `U=1,000,000`; sparse compressed bitmap performance differs. **Bitmap is a non-authoritative candidate accelerator only**. If the cache is stale, PostgreSQL's transaction still enforces exclusivity. Operational and synchronization complexity warrant a benchmark before adoption.

## 5. Correct allocation under concurrency

### Atomic search-and-reserve transaction

1. Authenticate request; validate six-character pattern, requested quantity bounds, and idempotency key.
2. Start PostgreSQL transaction, using **READ COMMITTED** as a sensible initial isolation level.
3. Look up an existing `(user_id, request_key)` reservation and return the same outcome for a duplicate request; serialize competing inserts through the unique constraint and retry/read on conflict. Do not blindly create a fresh reservation on network retries.
4. Find a bounded set of matching candidate tickets that are `AVAILABLE`, or whose `RESERVED` lease is **provably expired**. Select candidates with **row-level `FOR UPDATE SKIP LOCKED`**; choose one or an explicitly limited batch. The selection and state update occur in **one transaction**.
5. For an expired reservation that is reclaimed, atomically mark its previous reservation `EXPIRED`. Update selected ticket to `RESERVED`, set `reserved_by`, a new random `reservation_id`, and `reserved_until = database_current_time + TTL`; insert corresponding ACTIVE reservation record.
6. Commit. **Only then** return ticket ID, six-digit number, reservation ID and expiry. On error/rollback, release locks and return no ticket.
7. If zero candidates were found, distinguish 'no currently available tickets' from a temporary skip-locked busy condition where helpful; optionally retry bounded times with jitter, not an infinite spin loop.

`SKIP LOCKED` is **not** a long-lived reservation. Exclusive status and `reservation_id` stored after commit are what prevent returning the ticket again while the first user's lease remains active.

### Confirm, cancel, expire

- **Confirm:** in one transaction, conditionally transition the ticket and reservation from RESERVED/ACTIVE to ALLOCATED/CONFIRMED only if `(ticket_id, reservation_id, reserved_by, status=RESERVED, reserved_until > database_current_time)` all match. Return conflict for stale, expired, cancelled or wrong-owner requests. An old reservation must not confirm a newer reservation's ticket (fencing).
- **Cancel:** update only if the same reservation is still active and owned by caller, and return the ticket to AVAILABLE atomically.
- **Expire:** scheduled sweeper scans due active leases in bounded batches, locking rows before making ticket AVAILABLE and reservation EXPIRED. Alternatively allow allocation to reclaim expired leases inline in the transaction; combining with a sweeper improves cleanup latency. Cleanup races with confirmation/reclaim are resolved by row locks and reservation-ID/state predicates. **Never release an item merely because a stale sweeper snapshot says it expired.**
- **Idempotency:** stable request key returns the existing reservation/result on client retry; confirmation and cancellation may return the current finalized status for repeat same-owner requests.

### Why locking the pattern is wrong

User A requesting `123***` and User B requesting `*23***` both match physical ticket `123456` (same `ticket_id`). Pattern-scoped locks do not conflict. Row-level locking on **ticket_id** plus transactional status update prevents duplicate assignment across all patterns.

### Invariants

- At any point, a ticket has **at most one active, unexpired reservation** by `ticket_id`.
- Every allocation claim refers to a committed reservation owned by the requesting user; no search preview exposes exclusive ticket IDs without a successful reservation.
- An expired or superseded reservation ID cannot confirm an item.
- A confirmed allocation is terminal unless an explicit, separately designed refund/return process exists.
- Two simultaneously running application instances share exactly the same PostgreSQL source of truth (no process-local locks for correctness).

## 6. Capacity, performance, failure scenarios

| Scenario | Expected behavior | Trade-off / observability |
|---|---|---|
| 10M tickets; `123456` | narrow indexed candidate set | duplicates per number may exist |
| `******` | bounded reservation, avoid materializing 10M | ensure available/status index and fair selection |
| dozens of clients same pattern | distinct ticket IDs or no availability | lock skips, finite retries, watch contention |
| overlapping patterns | distinct physical IDs | shared DB row locks, not pattern locks |
| API crashes before commit | transaction rolls back; no ticket promised | retry with same request key |
| API crashes after commit before response | reservation survives; client retry resolves by request key | idempotency record persisted atomically |
| client never confirms | lease expires/reclaimed safely | sweeper lag + TTL semantics |
| expired confirm races with new reserve | only one atomic transaction wins | fencing by reservation ID/time |
| replica delay / cache staleness | write/reserve from PostgreSQL primary | read replicas/Redis may only provide advisory results |
| PostgreSQL unavailable | fail closed, no speculative allocation | correctness over availability |

**Complexity:** Full table scan is O(N) = 10M examined rows per broad query. With a selective B-tree index, lookup is approximately O(log N + m) where `m` is matching index entries considered; selecting with status and locks adds contention and candidate filtering. This is a *logical model*, not an end-to-end latency guarantee. For some patterns, especially `******`, `m` is large; bounded candidate selection and workload-tuned indexes are essential. Memory bitmap candidate discovery over unique numbers may be fast but still needs an authoritative reservation transaction.

### Benchmark plan (not executed)

- Generate **10M distinct physical ticket rows**, distributing numbers uniformly and with Zipf/skewed hot combinations.
- Test `123456`, `123***`, `****23`, `1****5`, `******` with 0%, 50%, 90% already allocated/reserved.
- Compare PostgreSQL query plans using `EXPLAIN (ANALYZE, BUFFERS)` and realistic concurrency.
- Measure p50/p95/p99 latency, reservations/sec, lock wait/skip count, rows examined, I/O, CPU/memory, index size, reservation expiration accuracy.
- Run 100–1,000 parallel clients with identical **and overlapping** patterns, assert zero concurrent duplicate `ticket_id`s, zero stale confirmation success, and no lost confirmations.
- Set SLO targets only after load testing with realistic hardware and business traffic goals; no unsupported sub-ms or TPS promise.

## 7. Design trade-offs and alternatives

**PostgreSQL-first** offers correctness with fewer moving parts. Its main risks are hot-number competition and the cost of dynamically assembled indexes. Add targeted composite/partial indexes based on actual workload, and consider in-memory bitmaps solely if measured search performance warrants them. Sharding should not be a starting assumption at 10M rows; if future scale forces partitioning, a globally consistent per-ticket reservation invariant must still hold. Consider reservation TTL UX, reconciliation jobs, and retention/audit needs with product stakeholders.

## 8. Sources

- PostgreSQL `SELECT` locking / SKIP LOCKED: https://www.postgresql.org/docs/current/sql-select.html
- PostgreSQL combining multiple indexes / bitmap scan: https://www.postgresql.org/docs/current/indexes-bitmap-scans.html
- PostgreSQL partial indexes: https://www.postgresql.org/docs/current/indexes-partial.html
- PostgreSQL transaction isolation: https://www.postgresql.org/docs/current/transaction-iso.html

**Status:** Design proposal only; no lottery implementation or performance benchmark was performed.

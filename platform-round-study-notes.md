# Platform Round Study Notes
### For Staff/Lead Backend Interviews (Agoda-style)

---

## 1. Caching — Invalidation & Race Conditions

**Core problem:** cache correctness under concurrent writes and refreshes.

- **Cache-aside (lazy loading):** app checks cache, on miss reads DB and populates cache. Simple, but stale-read window exists between DB write and cache invalidation.
- **Write-through:** every write goes to cache and DB together (synchronously). Keeps cache fresh but adds write latency and a two-phase-write failure mode (what if DB write succeeds but cache write fails, or vice versa?).
- **Write-behind (write-back):** write to cache first, async flush to DB. Fast, but risks data loss if the cache node dies before flush.
- **The "thundering herd" / dogpile problem:** when a hot key expires, many concurrent requests miss simultaneously and all hit the DB at once. Fix with request coalescing (singleflight pattern in Go) or staggered TTLs with jitter.
- **The torn-read / in-place overwrite problem** (like the FX rate example): if a cache is a mutable map being overwritten in place while readers iterate/read, you can get partial writes or readers seeing a half-updated state. Fixes:
  - **Atomic swap:** build the new map fully off to the side, then swap the pointer atomically (works well in Go with `atomic.Value` or a `sync.RWMutex` guarding a pointer swap, not the map itself).
  - **Versioned cache entries:** each entry carries a version/timestamp; readers can detect staleness.
  - **Copy-on-write:** never mutate in place.
- **Multi-instance cache consistency:** if each pod has its own in-memory cache (no shared Redis/Memcached), you get **cache divergence** — different pods can serve different values for the same key. Options: centralize the cache (Redis), use a pub/sub invalidation broadcast (e.g., Redis pub/sub or a Kafka topic each pod subscribes to for invalidation events), or accept eventual consistency with a bounded staleness window and document the SLA.
- **Silent failure to refresh:** always alert on stale-data age, not just on cron job failure. "Last successful refresh was 6 hours ago" should page someone.

---

## 2. Connectivity & API Integration

- **Timeouts:** never inherit one global timeout across all downstream calls. Each dependency should have its own timeout tuned to its latency profile (fast internal service: 100-300ms; external partner: 2-5s). A single slow dependency with a shared long timeout can exhaust connections for everyone.
- **Retries:**
  - Only retry **idempotent** operations, or make the operation idempotent first (idempotency keys).
  - Use **exponential backoff with jitter** to avoid retry storms (all clients retrying in lockstep amplifies load on a struggling service).
  - Cap retry attempts and have a clear "give up and degrade" path.
- **Idempotency keys:** client generates a unique key per logical operation (e.g., booking attempt); server stores the key with the result and returns the cached result on duplicate submission instead of reprocessing. Critical for payment/booking flows where client retries on timeout.
- **Circuit breakers:** trip after N consecutive failures or an error-rate threshold; stop calling a failing dependency for a cooldown period; allow a trickle of "half-open" requests to test recovery. Prevents cascading failures and wasted resources hammering a dead service.
- **Bulkheads:** isolate resource pools (thread pools, connection pools) per dependency so one slow dependency can't starve resources needed by others. This directly maps to the "single shared DB connection pool across all endpoints" anti-pattern — one bad query type can starve the whole monolith.
- **Backpressure:** when a downstream can't keep up, the system should signal upstream to slow down (bounded queues, load shedding, 429s) rather than silently queuing unboundedly until OOM.
- **Contract/version compatibility:** additive-only schema changes, consumer-driven contract testing, deprecation windows, and never silently changing a field's meaning.

---

## 3. Queues & Async Processing

- **At-least-once vs exactly-once vs at-most-once delivery:** most systems (Kafka, Kinesis, SQS) give at-least-once by default — duplicates are possible, so consumers must be idempotent (dedup by message ID/idempotency key).
- **Dead Letter Queues (DLQ):** poison messages (malformed, or that fail processing repeatedly) should move to a DLQ after N retries rather than blocking the whole queue or being silently dropped. Always alert on DLQ depth.
- **Ordering guarantees:** Kafka guarantees order within a partition only; Kinesis similarly within a shard. If ordering matters (e.g., events for the same booking), partition/shard by a consistent key (e.g., booking ID).
- **Consumer crash mid-batch:** design for resumability — commit offsets only after successful processing (not before), and make batch processing idempotent so a partial-then-retried batch doesn't double-apply.
- **Fan-out patterns:** for notification-style fan-out (email/SMS to many subscribers), don't loop synchronously in one function — publish one event per recipient to a queue so failures are isolated and retryable per-recipient, not all-or-nothing.
- **Kinesis vs Kafka (since you've used Kinesis):** Kinesis shards vs Kafka partitions are conceptually similar; Kinesis has simpler ops (managed) but is AWS-locked and has shard-level throughput limits (1MB/s or 1000 records/s per shard) — resharding is a known operational pain point worth mentioning if asked "why not Kafka."

---

## 4. Datastore Tradeoffs (tailored to your stack)

- **MySQL (relational):** strong consistency, transactions, joins — good for data with relational integrity needs (loan records, KYC status tied to user). Weak point: harder to horizontally scale writes; connection pool exhaustion under high concurrency if not tuned (max connections, pool sizing per service).
- **DynamoDB (NoSQL, key-value/document):** scales horizontally very well, predictable latency, but no joins and eventual-consistency-by-default reads (strongly consistent reads cost more and add latency). Partition key design is critical — a poorly chosen key causes **hot partitions** (e.g., partitioning by date when all writes land on "today"). Know when you'd choose it: high-throughput, simple access patterns, need horizontal scale over relational complexity.
- **Neo4j (graph):** excels at relationship-heavy queries (fraud rings, multi-hop connections) that would require expensive multi-way joins in SQL. Weak point: expensive traversal queries can be slow under load — don't put them in a synchronous hot path (like the fraud-scoring example); consider precomputing/caching scores or moving to an async check where possible.
- **General rule interviewers probe for:** "why this store for this data shape and access pattern" — not "which is objectively better." Always frame in terms of access pattern, consistency needs, and scale characteristics.

---

## 5. Resilience & Failure-Mode Thinking

- Always ask: **what happens when each dependency is slow? down? returns malformed data? returns success but is actually wrong (silent corruption)?**
- **Read-then-write race conditions** (the overbooking example): "check availability, then decrement" is a classic TOCTOU (time-of-check-to-time-of-use) bug. Fix with: DB-level atomic decrement with a `WHERE quantity > 0` guard, optimistic locking (version column), or pessimistic row locks for low-contention cases.
- **In-memory rate limiters on horizontally scaled pods:** don't work as intended — each pod enforces the limit independently, so effective limit multiplies by pod count. Fix: centralize in Redis (e.g., token bucket via Redis + Lua script for atomicity), or use a dedicated rate-limiting service/API gateway feature.
- **Fire-and-forget without a DLQ or delivery guarantee:** anything business-critical (audit logs, payment events) needs guaranteed delivery — either synchronous with retry+idempotency, or async via a durable queue with DLQ and monitoring, never "just fire and hope."

---

## 6. Testing Methodology for Distributed/Async Systems

- **Test pyramid still applies:** unit tests for business logic, integration tests for service boundaries, a thin layer of end-to-end tests.
- **Contract testing** (e.g., Pact) for API integrations — catches breaking changes between services without needing full E2E environments.
- **Idempotency tests:** explicitly test that replaying the same request/message twice produces the same end state.
- **Chaos/failure-injection testing:** simulate a dependency timing out, returning errors, or returning malformed data — verify circuit breakers and fallbacks actually engage.
- **Race condition testing:** for concurrency bugs (like the overbooking case), write tests that fire concurrent requests and assert invariants hold (e.g., inventory never goes negative).
- **Shadow traffic / dark launches:** for risky migrations (e.g., PHP-to-Go rewrite), run the new system in parallel on production traffic, compare outputs, before cutting over — catches subtle bugs like currency rounding differences before they cause real damage.

---

## 7. Code Review — What "Deep" Looks Like

Beyond style/formatting, a strong reviewer checks:
- **Correctness under concurrency** — race conditions, missing locks, non-atomic read-modify-write.
- **Failure handling** — are errors from downstream calls actually handled, or just logged and ignored? Is there a fallback or is failure silent?
- **Idempotency** — will retrying this operation cause duplicate side effects?
- **Backward compatibility** — does this change break existing consumers of an API/schema?
- **Observability** — are there metrics/logs/traces for the new code path, especially failure paths?
- **Blast radius** — if this is wrong, what breaks, and how would we know quickly?

---

## 8. Leadership / Staff-Level Framing

Structure stories using: **Scope → Decision → Tradeoff → Impact beyond your team**

- Have 2-3 ready: one where you drove a technical decision with organizational impact, one where you influenced another team without direct authority, one where you changed your mind after being challenged.
- For the cross-team schema-drift scenario style questions: emphasize the **process fix**, not just the technical fix — e.g., contract tests, schema change notifications, ownership boundaries — since Staff-level answers are graded on whether the fix prevents recurrence, not just resolves the incident.
- Incident leadership structure: **stabilize → communicate → root cause → postmortem → durable fix → follow-up ownership**. Interviewers listen for whether you assign follow-up ownership and drive it to closure, not just "we had a meeting."

---

## 9. Interview Technique

- **Always scope first:** ask about scale (requests/sec, data volume), consistency requirements, existing constraints, and team/ownership boundaries before proposing a design.
- **Diagram early, diagram often:** a simple box-and-arrow sketch while narrating shows structured thinking; practice doing this in under 2-3 minutes.
- **State tradeoffs explicitly, don't just pick one answer:** "I'd choose X because of Y, at the cost of Z — here's when I'd choose differently."
- **Go one layer deeper than the surface API** for any technology you name — be ready for "how does that actually work under the hood."
- **Structure your answers:** problem → constraints → options considered → chosen approach → tradeoffs → what you'd monitor/test for.

---

## Quick Reference: Anti-Pattern → Fix Cheat Sheet

| Anti-pattern | Fix |
|---|---|
| Synchronous chained calls in one request | Async where possible (queue), parallelize independent calls |
| No idempotency, client retries cause duplicates | Idempotency keys |
| Single shared connection pool for all endpoints | Per-dependency pools (bulkheads) |
| No circuit breaker | Add circuit breaker + fallback/degrade path |
| In-place cache overwrite (torn reads) | Atomic pointer swap / copy-on-write |
| Per-pod in-memory rate limiter | Centralize in Redis with atomic ops |
| Read-then-write race (overbooking) | Atomic DB operation with guard condition, or optimistic locking |
| Fire-and-forget with no DLQ | Durable queue + DLQ + alerting |
| Silent cron/refresh failure | Alert on staleness age, not just job failure |
| Informal code review ("LGTM") | Explicit checklist: concurrency, failure handling, idempotency, compat, observability |

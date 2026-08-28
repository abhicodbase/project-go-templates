# Addendum: Connectivity Patterns, Extra Code Smells, Observability, Time Management

---

## 1. REST vs gRPC — have the tradeoff ready

| | REST (JSON/HTTP) | gRPC (Protobuf/HTTP2) |
|---|---|---|
| Payload | Text, human-readable, larger | Binary, compact, faster to (de)serialize |
| Contract | Loose (OpenAPI optional) | Strict (`.proto` schema, generated code) |
| Streaming | Awkward (SSE/long-poll hacks) | Native bidirectional streaming |
| Browser support | Universal | Needs grpc-web/proxy for browsers |
| Debuggability | Easy (curl, browser devtools) | Harder (needs proto-aware tooling) |
| Best fit here | Public/partner-facing APIs (suppliers, webhooks) — needs universal compatibility | Internal service-to-service calls at scale — needs speed + strict contracts (e.g., your KYC ↔ Cross-Sell internal calls) |

**Say this line:** "I'd default to REST at the edge/partner boundary for compatibility, and gRPC internally between services I control, where I can enforce the contract and want the performance."

---

## 2. Webhooks vs Pub/Sub vs Polling — comparison to recite

| | Webhooks | Pub/Sub (Kafka/Kinesis/SNS) | Polling |
|---|---|---|---|
| Who initiates | Sender pushes to your endpoint | Sender publishes, consumers subscribe | You pull on a schedule |
| Delivery guarantee | At-least-once if sender retries; you must dedupe | At-least-once typically; consumer offsets/acks | You control frequency — no "guarantee," just freshness vs cost tradeoff |
| Failure mode | If your endpoint is down, sender retries or drops (depends on their SLA) | Durable — message sits in the log/queue until consumed | Missed poll = stale data until next poll |
| When to use | Real-time updates from a partner who supports them | Decoupling internal services, fan-out to many consumers, replay needed | No push available (your flight-pricing case), or when near-real-time isn't required |
| Your two case studies | N/A here — no supplier push | Internal event fan-out (order placed → notify, ship, audit) | Flight pricing aggregator — this is literally why demand-weighted polling existed in that design |

**Say this line:** "The choice is really about who controls delivery — push means the sender controls timing and I need idempotent handling; pull means I control cost and freshness tradeoffs myself."

---

## 3. Pagination patterns

- **Offset/limit** (`?offset=100&limit=20`): simple, but breaks under concurrent writes (page drift — an insert before your offset shifts every subsequent page), and gets slow at high offsets (DB still scans+discards prior rows).
- **Cursor-based** (`?after=<opaque_token>`): stable under concurrent writes, consistent performance regardless of depth, but can't jump to an arbitrary page number. Preferred for high-write, real-time, or infinite-scroll style APIs — mention this as the fix when a snippet uses raw offset/limit on a busy table.

---

## 4. Two more code smells to actively hunt for (called out explicitly in the brief)

- **Expensive resource initialization per call** — e.g., opening a new DB connection, HTTP client, or crypto context *inside* a request handler instead of reusing a pooled/shared instance created once at startup. This silently tanks throughput and can exhaust connection limits under load. Watch for `sql.Open(...)` or `http.Client{}` construction inside a function body rather than injected/package-level.
- **Hardcoded vendor keys/secrets** — API keys, credentials, or endpoints hardcoded as string literals rather than pulled from config/secrets manager. Flag immediately even in a "just improve this snippet" exercise — it's a security finding, not a style nitpick, and reviewers specifically watch whether you catch it unprompted.

---

## 5. Observability — the three-part answer

When asked "how would you monitor this," structure the answer as:
- **Logs** — structured (not string-concatenated), include correlation/request IDs so you can trace one request across services.
- **Metrics** — latency percentiles (p50/p95/p99) per dependency, error rates, and business metrics (e.g., "eligibility approvals per minute") not just infra metrics.
- **Tracing** — distributed tracing (e.g., OpenTelemetry) across the call chain so a slow request can be attributed to the specific downstream call that caused it — critical in a system like the loan eligibility engine with 5 sequential dependencies.
- Close with: "I'd alert on SLA burn (error rate or latency breaching threshold) rather than raw counts, and alert on staleness/DLQ depth for anything async."

---

## 6. Backpressure controls — concrete mechanisms to name

- **Bounded queues** — reject/shed load once a queue is full rather than growing unboundedly toward OOM.
- **Load shedding** — return 429/503 under overload rather than degrading every request's latency equally.
- **Rate limiting at the edge** — token bucket per client/partner, enforced before expensive work happens.
- **Circuit breakers** (again) double as backpressure — stop sending load to a struggling dependency.

---

## 7. Time management in the round itself

- **Don't over-invest in line-by-line code minutiae.** Spend ~5-7 minutes on the snippet critique, then explicitly say "I want to zoom out to the system level" and move on — this signals seniority. Getting stuck polishing one function for 20 minutes is a common way candidates fail this round even with correct answers.
- **Narrate a mental timer out loud**: "Let me flag the top 3 issues here, then I'd like to talk about how this fits into the broader system" — this is literally the "manage time and scope" criterion being graded.
- **If the interviewer redirects you, follow immediately** — don't finish your point first. Redirects are often deliberate time management from their side too.

---

## 8. "Ground theory in practice" — the technique

Every time you name a concept (idempotency, circuit breaker, cursor pagination), follow it with one sentence tying it to something you've actually built — e.g., "similar to how I'd handle retries on partner callbacks in the Cross-Sell Personal Loan flow" or "the way KYC document uploads need idempotent retries when the virus-scan step times out." You don't need a long story — one grounding clause per concept is enough to satisfy this criterion without derailing your pacing.

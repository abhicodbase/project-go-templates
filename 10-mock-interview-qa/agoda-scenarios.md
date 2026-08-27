# Mock Interview Q&A — Agoda Platform Engineering

> **Format**: 60 minutes | Narrative scenario + Generic Q&A
> Practice these out loud. Time your answers (aim for 3-5 minutes per design question).

---

## 🏗️ System Design Q&A

### Q1: Design a distributed rate limiter for Agoda's API Gateway

**Model Answer** (2-3 min):
> I'd implement a sliding window counter using Redis for distributed state.
> 
> **Clarifications first**: Rate per API key or IP? Global or per-endpoint? Hard reject or queue? What if Redis is down?
>
> **Algorithm**: Sliding window counter — two Redis keys per client per minute (current and previous window), atomically check and increment using a Lua script to prevent race conditions. Rate ≈ `current_count + (prev_count × (1 - elapsed_fraction))`.
>
> **Architecture**: Each Gateway instance calls Redis (< 1ms) on every request. Redis Cluster for HA. Response headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `Retry-After` on 429.
>
> **Failure mode**: If Redis is down → fail open (allow traffic) — rate limiting is best-effort, not a critical path.
>
> **Trade-off I made**: Sliding window is slightly approximate but prevents the boundary burst problem of fixed windows, with better memory than sliding log.

---

### Q2: Design the Agoda hotel search system

**Model Answer** (3-4 min):
> 
> **Clarifications**: Scale (DAU, QPS)? Real-time availability? How many hotels? Global or regional?
>
> **Architecture**:
> - **Search**: Elasticsearch for full-text + faceted search (city, amenities, rating). Index hotel data, update via Kafka events when hotels change.
> - **Availability**: Don't cache tightly — call hotel suppliers via gRPC with circuit breaker + timeout. Cache at 30s TTL with explicit invalidation on booking.
> - **Results**: Cache search results in Redis for popular queries (city + dates). Key: `search:Bangkok:2024-01-15:2024-01-18`. TTL: 2 minutes.
> - **CDN**: Hotel images and static content from CDN (Cloudflare/Akamai).
> - **Scaling**: Stateless search service behind ALB, auto-scale on CPU.
>
> **Trade-offs**: Elasticsearch gives great search UX but has sync delay (eventual consistency). Cache may show slightly stale prices — acceptable for search, not for booking confirmation.

---

### Q3: How would you design the Agoda booking system to prevent double booking?

**Model Answer**:
> Double booking = two users booking the last room simultaneously.
>
> **Solution**: Pessimistic locking with `SELECT FOR UPDATE` on the inventory row within a transaction.
>
> ```sql
> BEGIN;
> SELECT available_rooms FROM inventory 
>   WHERE hotel_id=123 AND date='2024-01-15' FOR UPDATE;  -- blocks other transactions
> -- If available_rooms > 0:
> UPDATE inventory SET available_rooms = available_rooms - 1 WHERE hotel_id=123 AND date='2024-01-15';
> INSERT INTO bookings ...;
> COMMIT;
> ```
>
> **Alternative at scale**: Optimistic locking with version column — no DB lock, but retry on conflict. Better for low-conflict scenarios.
>
> **At very large scale**: Use Redis DECR on inventory counter (atomic), then persist to DB asynchronously. Sub-millisecond vs DB transaction.

---

### Q4: Notification system handling 10M notifications/day

> See [notification-system case study](../01-system-design/case-studies/notification-system.md) for full design.
>
> **Key points**:
> 1. Kafka for decoupling — booking service publishes, notification service consumes
> 2. Priority queues — booking confirmation (high) vs promo (low) on separate topics
> 3. Deduplication via Redis `SETNX` before calling external APIs
> 4. Dead Letter Queue for failed deliveries
> 5. At-least-once delivery + idempotent processing

---

## 🔌 Connectivity / API Design Q&A

### Q5: When would you use gRPC vs REST?

> REST: Public-facing APIs, browser clients, external integrations — universally supported, human-readable JSON.
>
> gRPC: Internal service-to-service — 5-10x faster (binary protobuf), native streaming, strongly typed contracts. Hotel Service calling Pricing Service calling Payment Service.
>
> At Agoda: External partner APIs → REST. Internal microservice calls → gRPC. Mobile apps → REST (or GraphQL for flexibility).

---

### Q6: How do you design an API for backward compatibility?

> 1. URL versioning: `/v1/hotels` vs `/v2/hotels`
> 2. Within a version, only additive changes (new optional fields, new endpoints)
> 3. Never remove or rename fields in the same version
> 4. Deprecation headers (`Deprecation: true`, `Sunset: 2024-12-31`) for advance notice
> 5. Support multiple versions simultaneously during migration
> 6. Semantic versioning in `info.version` in OpenAPI spec

---

## ✅ Quality Assurance Q&A

### Q7: How do you test a service that depends on Kafka?

> Three approaches:
> 1. **Unit tests**: Mock the Kafka producer/consumer interface — test business logic in isolation
> 2. **Integration tests**: Use testcontainers to start a real Kafka broker in a Docker container, produce messages, assert consumer behavior — this tests the actual Kafka interaction
> 3. **Contract tests**: Verify that the message format produced by Service A is consumable by Service B, without running both simultaneously (using Pact or similar)
>
> Key: never test business logic against real Kafka in unit tests — too slow, brittle, hard to parallelize.

---

### Q8: What is the test pyramid and why does it matter?

> Test pyramid: many unit tests (fast, cheap, isolated) → some integration tests (real deps) → few E2E tests (full system, slow, expensive).
>
> Why it matters: inverted pyramid (many E2E, few unit) = slow CI, flaky tests, hard to debug failures, expensive to run.
>
> My approach: unit test business logic with mocks, integration test repository layer against real DB (testcontainers), E2E test critical user journeys (booking flow) in staging.

---

## 👁️ Code Review Q&A

### Q9: Review this code snippet — what's wrong?

```go
func GetUser(id string) User {
    db.Query("SELECT * FROM users WHERE id = " + id)
    // ...
}
```

> Issues:
> 1. **SQL Injection** — string concatenation; use parameterized query `$1`
> 2. **Missing error handling** — `db.Query` return value ignored
> 3. **Missing context** — no `ctx context.Context` for cancellation/timeout
> 4. **SELECT \*** — select only needed columns
> 5. **No rows.Close()** — resource leak if rows is not closed
> 6. **Missing return error** — function signature hides errors

---

### Q10: What SOLID principles are violated here?

```go
type UserService struct {
    db   *sql.DB
    smtp SMTPClient
    pdf  PDFGenerator
}

func (s *UserService) Register(email, password string) error {
    // hash password, save to db, send welcome email, generate PDF receipt
}
```

> SRP: UserService handles user data + email sending + PDF generation — three reasons to change.
>
> DIP: Depends on `*sql.DB` (concrete) instead of a `UserRepository` interface.
>
> Fix: Split into UserService (business logic) + UserRepository (data access) + NotificationService (email). Use constructor injection with interfaces.

---

## 🗣️ Communication Tips (Practice These Phrases)

```
"Let me clarify the requirements first — do you need X or Y?"
"I'll think through this out loud so you can follow my reasoning..."
"The trade-off here is X gives us Y but costs us Z..."
"I chose this approach because... An alternative would be... I rejected it because..."
"Does this approach make sense before I go deeper?"
"The biggest risk with this design is... Here's how I'd mitigate it..."
"I'd start with the simplest version and evolve it as scale demands..."
```

---

## 📋 Agoda-Specific Scenarios (Likely Narratives)

### Scenario A: Slow Hotel Search
*"Our hotel search API has p99 latency of 3s. Investigate and fix."*

> Framework:
> 1. Measure — where is the time? Add tracing spans
> 2. DB queries — check EXPLAIN ANALYZE, N+1 queries, missing indexes
> 3. External calls — check hotel supplier API latency, add circuit breaker
> 4. No caching — add Redis cache for search results
> 5. Sequential calls — parallelize independent calls with goroutines/errgroup

### Scenario B: Inventory Bug
*"We have overbooking incidents — two users booking the same last room."*

> 1. Root cause: race condition — concurrent transactions reading same inventory without locking
> 2. Fix: `SELECT FOR UPDATE` or optimistic locking with version column
> 3. Detection: Add unique constraint, monitoring alerts on booking count vs inventory
> 4. Prevention: Integration test with concurrent booking simulation

### Scenario C: Service Cascade Failure
*"Hotel supplier API went down and it's taking our entire platform with it."*

> 1. Root cause: no circuit breaker — requests pile up waiting for timeout
> 2. Fix: Circuit breaker + timeout on all external calls
> 3. Graceful degradation: return cached/stale prices with a "prices may vary" warning
> 4. Monitoring: Alert on circuit breaker open rate
> 5. Future: Bulkhead to isolate supplier failures from other functionality

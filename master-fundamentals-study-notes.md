# Master Study Notes — Fundamentals Deep Dive

This fills the gaps not yet covered in the other files: core system design patterns,
SOLID principles, common design patterns for code review, and non-functional testing
in depth. Read this alongside the other 4 files — this one is the "why it works this
way" layer underneath them.

---

## 1. System Design Patterns

### Monolith vs Microservices
- **Monolith**: one deployable unit, shared DB, simple to develop/test early, but
  scaling means scaling everything together, and one bug can take down the whole app.
  Your loan eligibility monolith example is exactly this failure mode.
- **Microservices**: independently deployable services, each own their data, scale
  independently — but you now pay a **network tax** on every cross-service call, need
  service discovery, distributed tracing, and eventual consistency between services.
- **What to say in the interview:** "I don't treat microservices as inherently better —
  it's a tradeoff of operational complexity for independent scalability and deployment.
  I'd split a monolith when a specific component has different scaling needs or
  ownership boundaries than the rest, not by default."

### Event-Driven Architecture
- Services communicate by publishing/consuming events rather than direct synchronous
  calls. Decouples producers from consumers — a producer doesn't need to know who's
  listening or wait for them to respond.
- **Choreography vs orchestration:**
  - *Choreography*: each service reacts to events independently, no central
    coordinator (e.g., "order placed" event triggers inventory, shipping, and email
    services independently). Flexible, but hard to see the overall flow — "distributed
    spaghetti" if overused.
  - *Orchestration*: a central coordinator (e.g., a saga orchestrator) explicitly
    sequences steps and handles compensation on failure. Easier to reason about and
    debug, but the orchestrator becomes a critical component itself.
- **Saga pattern**: for multi-step transactions across services with no distributed
  transaction support (no cross-service ACID), each step has a compensating action to
  undo it if a later step fails. Example: book flight → charge payment → if charge
  fails, compensating action releases the flight hold.

### Scalability
- **Vertical scaling** (bigger machine) vs **horizontal scaling** (more machines) —
  horizontal is generally preferred for resilience (no single point of failure) but
  requires the app to be stateless or externalize state (session store, shared cache).
- **Sharding**: splitting data across multiple DB instances by a shard key (e.g., user
  ID hash). Enables horizontal DB scaling but complicates cross-shard queries and
  transactions — pick a shard key that matches your dominant access pattern.
- **Load balancing**: round-robin, least-connections, or consistent hashing (useful
  when you want the same client routed to the same backend for cache locality).
- **Caching layers**: CDN (static/edge) → application cache (Redis) → DB query cache —
  each layer trades freshness for latency reduction, exactly the tension in the flight
  pricing design.

### Fault Tolerance
- **Redundancy**: multiple instances of every critical component, across availability
  zones/regions, so no single failure takes the system down.
- **Failover**: automatic switch to a standby/replica when the primary fails — for
  DBs, this means replication (sync for stronger consistency, async for lower write
  latency but risk of losing recent writes on failover).
- **Graceful degradation**: when a non-critical dependency fails, serve a reduced
  experience rather than failing entirely — e.g., flight search still works if the
  "reviews" service is down, just without star ratings shown.
- **Health checks**: liveness (is the process alive) vs readiness (is it ready to
  serve traffic) — a service can be alive but not ready (e.g., still warming a cache).

### Tradeoffs — the vocabulary interviewers expect
- **CAP theorem**: under a network partition, you choose Consistency or Availability,
  not both. In practice, most systems pick **AP** (available, eventually consistent)
  for user-facing reads and **CP** (consistent, may reject requests) for anything
  involving money/inventory correctness — e.g., seat/room booking leans CP at the
  moment of commit, AP for browsing/search.
- **Latency vs consistency**: strongly consistent reads are slower (must check with a
  quorum/leader); eventually consistent reads are fast but can be stale — this is the
  entire flight-pricing cache design tension again.
- **Cost vs performance**: more caching/replication/pre-computation = faster but more
  infrastructure cost and more staleness/complexity to manage.

---

## 2. SOLID Principles (for code review discussions)

- **S — Single Responsibility**: a class/function should have one reason to change.
  The `FulfillPendingOrders` example violates this — it owns DB access, HTTP calls, and
  business logic all in one function.
- **O — Open/Closed**: open for extension, closed for modification — e.g., adding a
  new supplier shouldn't require editing a big switch statement everywhere suppliers
  are handled; use an interface/strategy pattern instead (see below).
- **L — Liskov Substitution**: a subtype should be usable anywhere its parent type is
  expected without breaking behavior — relevant when reviewing interface implementations.
- **I — Interface Segregation**: prefer many small, focused interfaces over one giant
  interface that forces implementers to support methods they don't need.
- **D — Dependency Inversion**: depend on abstractions (interfaces), not concrete
  implementations — e.g., `PricingService` should depend on a `SupplierClient`
  interface, not a concrete `SupplierAClient` struct, so it's testable and extensible.

**How to use this in a review:** you don't need to name the letter — say the *effect*.
"This function is doing three unrelated things — DB read, HTTP call, and business
logic — I'd split it so each piece can be tested and changed independently" is a
Single-Responsibility critique without ever saying "SRP."

---

## 3. Common Design Patterns worth recognizing in code review

- **Strategy pattern**: swap an algorithm/behavior at runtime via an interface — e.g.,
  different pricing/discount strategies per user segment, selected via a common
  interface rather than if/else chains.
- **Factory pattern**: centralize object creation logic — e.g., a `SupplierClientFactory`
  that returns the right adapter for a given supplier ID, so callers don't need to know
  construction details.
- **Adapter pattern**: wrap an external/incompatible interface (like each supplier's
  differently-shaped API) behind a common internal interface — this is exactly what the
  "Supplier Adapter Layer" in the flight-pricing design is.
- **Observer pattern**: subscribers react to state changes without tight coupling to
  the publisher — conceptually the same idea as event-driven architecture applied at
  the code level (e.g., pub/sub within a single process).
- **Decorator pattern**: wrap a component to add behavior (logging, retry, caching)
  without modifying its core logic — useful to mention when asked "how would you add
  retry logic without changing every call site" (a retrying HTTP client decorator).

**Why this matters for the round:** when you spot tight coupling or an if/else chain
handling multiple suppliers/types, naming "I'd use a strategy or adapter pattern here"
signals design maturity beyond just fixing the immediate bug.

---

## 4. Non-Functional Testing — deeper than the pyramid

The toolkit calls out "non-functional requirements (resilience, performance)"
explicitly — have concrete answers for each:

- **Load testing**: sustained expected traffic, verifying latency/error rate stay
  within SLA. Tools: k6, Locust, JMeter.
- **Stress testing**: push beyond expected capacity to find the breaking point and
  confirm the system degrades gracefully (sheds load, doesn't crash) rather than
  falling over entirely.
- **Soak testing**: run at moderate load for an extended period (hours) to catch slow
  memory leaks or resource exhaustion that short tests miss.
- **Chaos/failure-injection testing**: deliberately kill a dependency, inject latency,
  or drop network packets (tools: Chaos Monkey, Gremlin) to verify circuit breakers,
  retries, and fallbacks actually engage as designed — don't just assume they work
  because you wrote them.
- **Security testing**: basics worth naming — input validation/injection testing,
  authN/authZ boundary testing (can user A access user B's data by guessing an ID —
  IDOR), secrets scanning in CI.
- **Concurrency/race testing**: exactly the overbooking/double-sell scenario — fire
  concurrent requests at a shared resource and assert invariants hold. In Go, `go test
  -race` catches data races; for logical race conditions (like TOCTOU on inventory),
  you need targeted concurrent integration tests.

**How to frame this when asked "how do you test resilience":** "I'd validate the
happy path with integration tests, but resilience specifically needs failure
injection — I'd simulate the fraud service timing out and assert the circuit breaker
trips and the fallback path serves a degraded-but-correct response, not just that the
happy path works."

---

## 5. Communication — concrete techniques, not just "be clear"

The toolkit's "Communication is Key" section is vague by design — here's how to make
it concrete:

- **Structure every answer**: problem → constraints → options considered → chosen
  approach → tradeoffs. If you're mid-answer and realize you're rambling, say "let me
  structure this" and restart the sentence — interviewers read this as self-awareness,
  not weakness.
- **State assumptions out loud before proceeding**: "I'm assuming this needs to handle
  ~1000 requests/sec — let me know if that's off" — this both clarifies scope and
  demonstrates the "clarify goals first" criterion explicitly.
- **Use the diagram as a pointer, not a prop**: while narrating, physically point to
  (or say "as you can see here on the left") the part of the diagram you're discussing
  — keeps the interviewer's attention synced with your explanation.
- **Adapt to pushback, don't just defend**: if the interviewer says "why not X," the
  strong answer is "X is actually reasonable — here's when I'd choose it over my
  approach" rather than immediately defending your original choice. Shows you're
  reasoning, not just committed to being right.
- **Close each major section explicitly**: "so to summarize this part — I'd go with Y
  because Z, next I want to cover testing" — gives the interviewer a clean handoff
  point instead of you trailing off and them having to redirect you.

# 🚀 Agoda Platform Engineering IV2 — Interview Prep (Go)

> **Interview Format**: 60 minutes | Narrative scenario + Generic Q&A
> **Language**: Go | **Level**: Staff/Lead Engineer

## 📋 5 Core Interview Attributes

| Attribute | What's Tested |
|-----------|---------------|
| 🏗️ **System Analysis & Design** | Requirements breakdown, bottlenecks, scalability, fault tolerance |
| 🔌 **Connectivity Management** | REST/gRPC/GraphQL, error handling, auth, sync vs async |
| ✅ **Quality Assurance** | Test pyramid, TDD, integration tests, non-functional testing |
| 👁️ **Code Review** | SOLID, design patterns, code smells, clean code |
| 🗣️ **Communication** | Clear articulation, trade-off discussions, thought process |

---

## 📅 7-Day Study Plan

### Day 1 — System Design Foundations
- [ ] `01-system-design/scalability.md` — Horizontal/vertical scaling, load balancing
- [ ] `01-system-design/fault-tolerance.md` — Circuit breakers, retries, bulkheads
- [ ] `01-system-design/microservices-patterns.md` — Service mesh, API gateway, sidecar
- [ ] `01-system-design/case-studies/rate-limiter-design.md` — Full design walkthrough

### Day 2 — API Design & Connectivity
- [ ] `02-connectivity-api/rest-best-practices.md` — REST principles, versioning, pagination
- [ ] `02-connectivity-api/grpc-golang.md` — Proto3, streaming, interceptors in Go
- [ ] `02-connectivity-api/api-error-handling.md` — Error codes, retry strategies
- [ ] `02-connectivity-api/auth-authn-authz.md` — JWT, OAuth2, mTLS
- [ ] `02-connectivity-api/sync-vs-async.md` — Trade-offs and patterns

### Day 3 — Concurrency in Go
- [ ] `05-concurrency/goroutines-channels.md` — Goroutines, channels, select
- [ ] `05-concurrency/sync-primitives.md` — Mutex, RWMutex, WaitGroup, atomic
- [ ] `05-concurrency/concurrency-patterns.md` — Worker pool, fan-out/in, pipeline
- [ ] `05-concurrency/context-cancellation.md` — context.Context patterns
- [ ] Run all examples in `05-concurrency/examples/`

### Day 4 — Quality Assurance & TDD
- [ ] `03-quality-assurance/test-pyramid.md` — Test strategy overview
- [ ] `03-quality-assurance/tdd-golang.md` — TDD cycle + table-driven tests
- [ ] `03-quality-assurance/integration-testing.md` — testcontainers, mocking
- [ ] Study and run `03-quality-assurance/examples/`

### Day 5 — Databases, Kafka & Caching
- [ ] `06-databases/sql-fundamentals.md` — ACID, indexes, query optimization
- [ ] `06-databases/caching-strategies.md` — Cache-aside, write-through, eviction
- [ ] `06-databases/sharding-replication.md` — Consistency trade-offs
- [ ] `07-kafka-messaging/kafka-internals.md` — Topics, partitions, consumer groups
- [ ] `07-kafka-messaging/kafka-golang.md` — Producer/consumer code
- [ ] `07-kafka-messaging/patterns.md` — Exactly-once, DLQ, outbox pattern

### Day 6 — Code Review & LLD Patterns
- [ ] `04-code-review/solid-principles.md` — SOLID with Go examples
- [ ] `04-code-review/design-patterns.md` — GoF patterns in Go
- [ ] `04-code-review/code-smells.md` — Common smells and fixes
- [ ] `04-code-review/clean-code-go.md` — Go-specific idioms
- [ ] Do all exercises in `04-code-review/review-exercises/`
- [ ] `08-observability/` — Logging, metrics, tracing
- [ ] `09-kubernetes/` — K8s core concepts for microservices

### Day 7 — Mock Interview Practice
- [ ] `10-mock-interview-qa/system-design-qa.md` — 20 system design Q&A
- [ ] `10-mock-interview-qa/coding-qa.md` — Concurrency + LLD problems
- [ ] `10-mock-interview-qa/api-qa.md` — API design Q&A
- [ ] `10-mock-interview-qa/agoda-scenarios.md` — Agoda-specific platform scenarios
- [ ] Do a full timed mock: 60 mins, pick a scenario, talk through it

---

## 📁 Folder Structure

```
platform-go/
├── README.md                    ← You are here
├── 01-system-design/
├── 02-connectivity-api/
├── 03-quality-assurance/
├── 04-code-review/
├── 05-concurrency/
├── 06-databases/
├── 07-kafka-messaging/
├── 08-observability/
├── 09-kubernetes/
└── 10-mock-interview-qa/
```

## 💡 Interview Tips (from Agoda's own toolkit)

1. **Think out loud** — Walk the interviewer through your thought process
2. **Highlight trade-offs** — Don't just give solutions, discuss pros/cons
3. **Start with "why"** for tests — explain value each test type brings
4. **Frame code feedback positively** — offer alternatives, not just criticisms
5. **Adapt your explanation** — show you can talk to both technical and non-technical stakeholders
6. **Be specific** — mention real practices you follow in real projects

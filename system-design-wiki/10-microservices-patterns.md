# 10 — Microservices & Design Patterns

## TL;DR
> Microservices = small, independent services with clear boundaries. Each owns its data.  
> Circuit Breaker prevents cascade failures. Saga handles distributed transactions.  
> CQRS separates reads and writes. Event Sourcing stores state as events.

---

## 1. Microservices vs Monolith

### Monolith
```
┌─────────────────────────────────────────┐
│              Monolith                    │
│  ┌──────┐  ┌──────────┐  ┌──────────┐  │
│  │Users │  │ Orders   │  │Payments  │  │
│  └──────┘  └──────────┘  └──────────┘  │
│       Single deployable unit             │
│       Shared database                   │
└─────────────────────────────────────────┘
```

### Microservices
```
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│ User Service │   │Order Service │   │Payment Svc   │
│   (own DB)   │   │  (own DB)    │   │   (own DB)   │
└──────────────┘   └──────────────┘   └──────────────┘
  independently deployable, independently scalable
```

### Comparison

| Aspect | Monolith | Microservices |
|--------|---------|--------------|
| **Deployment** | Single unit | Independent services |
| **Scaling** | Scale entire app | Scale individual services |
| **Data** | Shared DB | Each service owns its DB |
| **Failure isolation** | One bug can crash all | Isolated failures |
| **Technology** | Single stack | Polyglot (each service chooses) |
| **Development speed** | Fast initially | Faster at scale (parallel teams) |
| **Operational complexity** | Low | High (many services to manage) |
| **Network overhead** | None (in-process calls) | Significant (HTTP/gRPC) |
| **Testing** | Easier | Harder (contract testing) |

> **Start with a monolith**. Extract microservices only when you hit specific scale or team coordination problems.

---

## 2. Service Discovery

How do services find each other in a dynamic environment (instances come and go)?

### Client-Side Discovery
```
Service A wants to call Service B:
  1. A queries Service Registry (Consul, etcd) → gets list of B instances
  2. A chooses one instance (client-side load balancing)
  3. A calls chosen instance directly

Pros: no extra network hop
Cons: discovery logic in every client
```

### Server-Side Discovery
```
Service A calls Load Balancer / API Gateway:
  1. A sends request to LB (just one address to know)
  2. LB queries Service Registry
  3. LB forwards to a healthy B instance

Pros: client is simple
Cons: extra hop through LB
```

### Service Registry Tools
| Tool | Type | Features |
|------|------|---------|
| **Consul** | External | Health checks, KV store, DNS |
| **etcd** | External | Key-value, Raft consensus |
| **Kubernetes DNS** | Platform | Automatic service DNS |
| **Eureka** | External | Netflix OSS, Java-heavy |
| **Zookeeper** | External | Strong consistency |

### Kubernetes Service Discovery
```yaml
# Service B created in Kubernetes
apiVersion: v1
kind: Service
metadata:
  name: order-service
spec:
  selector:
    app: order-service

# Service A calls via DNS name:
# http://order-service.namespace.svc.cluster.local:8080
```

---

## 3. Circuit Breaker Pattern

Prevent a service from repeatedly calling a failing downstream service.

### States
```
         success                    success
CLOSED ──────────▶ OPEN ─(timeout)──▶ HALF-OPEN ──▶ CLOSED
  ↑   failures exceed threshold   test request      ↑
  │                                                 │
  └──────────────────────────────────── success ────┘
                                         │
                                    OPEN (if fail again)
```

| State | Behavior |
|-------|---------|
| **CLOSED** | Normal operation; calls pass through; failures counted |
| **OPEN** | Immediately reject all calls; return fallback/error |
| **HALF-OPEN** | Allow limited test requests; if success → CLOSED, if fail → OPEN |

### Why Circuit Breaker?
```
Without CB:               With CB:
Service A → B (slow)      Service A → CB OPEN → immediate fail
A waits 30s per call      A fails fast (< 1ms)
A thread pool fills up    A can serve degraded experience
A → cascade failure       A → isolated failure
```

### Implementation (Pseudocode)
```go
type CircuitBreaker struct {
    state           string    // CLOSED, OPEN, HALF_OPEN
    failureCount    int
    threshold       int       // open circuit after N failures
    timeout         time.Duration  // time before trying HALF_OPEN
    lastFailureTime time.Time
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    switch cb.state {
    case OPEN:
        if time.Since(cb.lastFailureTime) > cb.timeout {
            cb.state = HALF_OPEN
        } else {
            return ErrCircuitOpen  // fast fail
        }
    }

    err := fn()
    if err != nil {
        cb.failureCount++
        cb.lastFailureTime = time.Now()
        if cb.failureCount >= cb.threshold {
            cb.state = OPEN
        }
        return err
    }

    cb.failureCount = 0
    cb.state = CLOSED
    return nil
}
```

### Libraries
- **Go**: `sony/gobreaker`, `afex/hystrix-go`
- **Java**: Resilience4j, Hystrix (deprecated)
- **Service Mesh**: Istio, Envoy handle circuit breaking automatically

---

## 4. Saga Pattern (Distributed Transactions)

Manage data consistency across multiple services without distributed locks.

### Problem
```
Booking flight + hotel:
  1. Book flight (Flight Service) ✓
  2. Book hotel (Hotel Service) ✗ (hotel fully booked!)
  3. Now flight is booked but hotel is not → INCONSISTENT STATE!
```

### Saga: Choreography-Based
Services react to events without central coordinator:
```
Order Service:      emit "OrderCreated"
Payment Service:    listens → process payment → emit "PaymentCompleted" OR "PaymentFailed"
Inventory Service:  listens to "PaymentCompleted" → reserve stock → emit "StockReserved"
Shipping Service:   listens to "StockReserved" → schedule pickup → emit "ShipmentScheduled"

Compensation (on failure):
  "PaymentFailed" → Order Service listens → emit "OrderCancelled"
  "StockReservationFailed" → Payment Service listens → issue refund
```

### Saga: Orchestration-Based
Central coordinator (Saga Orchestrator) tells each service what to do:
```
Saga Orchestrator:
  1. ──▶ Payment Service: "charge card"
  2. ◀── PaymentCompleted
  3. ──▶ Inventory Service: "reserve stock"
  4. ◀── StockReservationFailed!
  5. ──▶ Payment Service: "refund card" (compensating transaction)
  6. ──▶ Order Service: "cancel order"
```

### Choreography vs Orchestration

| Aspect | Choreography | Orchestration |
|--------|-------------|---------------|
| Coordination | Distributed (event-driven) | Centralized (saga orchestrator) |
| Coupling | Loose (services only know events) | Tighter (orchestrator knows all services) |
| Visibility | Hard to track overall flow | Easy to trace in orchestrator |
| Complexity | Distributed logic | Single place to debug |
| Use for | Simple linear flows | Complex, conditional flows |

---

## 5. CQRS (Command Query Responsibility Segregation)

Separate the **write model** (commands) from the **read model** (queries).

```
Without CQRS:
  Same model/DB for reads and writes
  Write-optimized schema ≠ read-optimized (e.g., joins, aggregations)

With CQRS:
  ┌─────────────────┐      ┌──────────────────────┐
  │  Command Side   │      │    Query Side         │
  │  (Write Model)  │──▶   │    (Read Model)       │
  │  Normalized DB  │ sync │  Denormalized DB      │
  │  (PostgreSQL)   │      │  (Elasticsearch/Redis)│
  └─────────────────┘      └──────────────────────┘
      Handles:                  Handles:
      CREATE/UPDATE/DELETE      GET/SEARCH/LIST
```

### Example: E-commerce

**Command Side** (write):
```sql
-- Normalized, ACID
INSERT INTO orders (user_id, total) VALUES (123, 99.00);
INSERT INTO order_items (order_id, product_id, qty) VALUES (1, 456, 2);
```

**Query Side** (read):
```json
// Denormalized, in Elasticsearch
{
  "order_id": 1,
  "user_name": "Alice",
  "user_email": "alice@example.com",
  "items": [{"name": "Widget", "qty": 2, "price": 49.50}],
  "total": 99.00,
  "status": "SHIPPED"
}
// Single document — no joins needed for display
```

### Benefits
- Read and write can scale independently
- Query model optimized for reads (no joins)
- Different storage for different needs

### Challenges
- Eventual consistency between write and read models
- More complexity (two models to maintain)
- Sync mechanism needed (event bus or CDC)

---

## 6. Event Sourcing

Store the **log of events** that happened, not just the current state.

```
Traditional:
  orders: { id:1, status:"SHIPPED", amount:100, updated_at: "..." }

Event Sourcing:
  event_log:
    { type:"OrderPlaced",  order_id:1, amount:100, ts: "2026-01-01" }
    { type:"PaymentTaken", order_id:1, amount:100, ts: "2026-01-01" }
    { type:"ItemPicked",   order_id:1,             ts: "2026-01-02" }
    { type:"OrderShipped", order_id:1, tracking:"XYZ", ts: "2026-01-03" }

Current state = REPLAY all events (or use a snapshot)
```

### Benefits
- **Complete audit trail** — know exactly what happened and when
- **Replay** — reconstruct state at any point in time
- **Multiple projections** — same events → multiple read models
- **Temporal queries** — "what did the order look like yesterday?"

### Snapshots (Performance Optimization)
```
Without snapshot: replay all events from beginning (slow for old aggregates)
With snapshot:
  Load latest snapshot at event #1000
  Replay only events #1001 to #1050
```

### Event Sourcing + CQRS (Natural Pair)
```
Command ──▶ Aggregate ──▶ Emit Events ──▶ Event Store
                                          │
                                          ├──▶ Read Model Projection 1 (DB)
                                          └──▶ Read Model Projection 2 (Search)
```

---

## 7. Bulkhead Pattern

Isolate components so failure in one doesn't drain resources from others.

```
Without Bulkhead:
  All requests share one thread pool
  Slow service X → pool fills up → ALL services starved

With Bulkhead:
  ┌─────────────────────────────────────────────┐
  │ Thread pool A (User Svc): 20 threads        │
  │ Thread pool B (Order Svc): 30 threads       │
  │ Thread pool C (Inventory Svc): 10 threads   │
  └─────────────────────────────────────────────┘
  Inventory pool saturated → only Inventory Svc affected, others fine
```

Named after ship bulkheads — compartments that prevent entire ship from flooding.

---

## 8. Strangler Fig Pattern (Monolith Migration)

Incrementally replace a monolith with microservices.

```
Phase 1: Monolith handles everything
  Users ──▶ Monolith

Phase 2: Extract one service (e.g., User Service)
  Users ──▶ [Proxy/API Gateway]
              ├── /users/* ──▶ New User Microservice
              └── /*       ──▶ Monolith (everything else)

Phase 3: Extract more services...
  Eventually:
  Users ──▶ API Gateway ──▶ Many Microservices (monolith gone)
```

---

## 9. Retry Pattern with Exponential Backoff

```
Attempt 1: immediate
Attempt 2: wait 1s
Attempt 3: wait 2s
Attempt 4: wait 4s
Attempt 5: wait 8s + random jitter
Give up after N attempts → return error to caller

// Formula:
wait = min(cap, base * 2^attempt) + random(0, jitter)
```

**With jitter**: prevents thundering herd when many clients retry simultaneously.

---

## 10. Idempotency Pattern

Make operations safe to retry without side effects.

```
// Bad: POST /payments (not idempotent → double charge on retry)
// Good: POST /payments with Idempotency-Key header

// Server logic:
func CreatePayment(key string, amount int) {
    if result := cache.Get(key); result != nil {
        return result  // return cached result, don't re-process
    }
    result = processPayment(amount)
    cache.Set(key, result, 24*time.Hour)
    return result
}
```

---

## Key Takeaways

1. **Microservices**: each service owns its data — never share databases between services
2. **Circuit Breaker**: fast-fail to prevent cascade failures; return fallback response
3. **Saga**: for distributed transactions; prefer orchestration for complex flows
4. **CQRS**: separate write and read models — optimize each independently
5. **Event Sourcing**: immutable event log = audit trail + replay + multiple projections
6. **Bulkhead**: separate thread pools per downstream service to contain failures
7. **Strangler Fig**: extract microservices incrementally, never big-bang rewrite
8. **Retry + Jitter + Circuit Breaker**: the trio that makes distributed systems resilient

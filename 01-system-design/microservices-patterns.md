# Microservices Patterns

## API Gateway

Single entry point for all client requests. Handles cross-cutting concerns.

```
Client ──► API Gateway ──► Hotel Service
                      ├──► Booking Service
                      ├──► Payment Service
                      └──► Notification Service
```

**Responsibilities**:
- Authentication/Authorization (validate JWT once, not in every service)
- Rate limiting
- Request routing
- SSL termination
- Response transformation/aggregation
- Logging & observability injection

```go
// API Gateway middleware chain in Go
func NewGateway() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/hotels/", proxyTo("hotel-service:8080"))
    mux.HandleFunc("/bookings/", proxyTo("booking-service:8081"))

    // Wrap with middleware (applied in reverse order)
    handler := mux
    handler = rateLimitMiddleware(handler)
    handler = authMiddleware(handler)
    handler = loggingMiddleware(handler)
    handler = tracingMiddleware(handler)
    return handler
}
```

---

## Sidecar Pattern

Deploy a helper container alongside your main service container (in the same Pod in K8s).

```
┌──────────────────────────────────┐
│            K8s Pod               │
│  ┌──────────────┐ ┌───────────┐ │
│  │  App Service │ │  Sidecar  │ │
│  │  (business   │ │ (Envoy/   │ │
│  │   logic)     │ │  Istio)   │ │
│  └──────────────┘ └───────────┘ │
└──────────────────────────────────┘
```

**Sidecar handles**:
- mTLS between services (service mesh)
- Metrics collection
- Distributed tracing
- Log forwarding
- Service discovery

**Why**: Keeps the app code clean — no need to implement these concerns in each service.

---

## Service Mesh (Istio/Linkerd)

Network of sidecars that manages service-to-service communication.

```
Service A ─► Envoy Proxy ──────► Envoy Proxy ─► Service B
              │                    │
              └──► Control Plane ◄─┘
                   (Istiod)
                   - traffic policies
                   - certificate management
                   - telemetry
```

**Benefits**: mTLS by default, traffic splitting (canary), retries/timeouts, observability.

---

## Strangler Fig Pattern

Migrate from monolith to microservices incrementally.

```
Phase 1:  All traffic → Monolith
Phase 2:  Hotel Search → Microservice | rest → Monolith
Phase 3:  Hotel Search + Booking → Microservices | rest → Monolith
Phase N:  All → Microservices | Monolith retired
```

```go
// Facade/proxy that routes to new or old system
func (h *HotelHandler) Search(w http.ResponseWriter, r *http.Request) {
    if featureFlag.IsEnabled("use_new_search_service") {
        // Delegate to new microservice
        h.newSearchClient.Search(r.Context(), r.URL.Query())
    } else {
        // Use legacy monolith logic
        h.legacySearch(w, r)
    }
}
```

---

## Saga Pattern (Distributed Transactions)

Manage distributed transactions without 2PC (two-phase commit).

### Choreography-based Saga
```
BookingService  ──publishes──► BookingCreated
                                    │
                                    ▼
HotelService    ──publishes──► RoomReserved
                                    │
                                    ▼
PaymentService  ──publishes──► PaymentCharged
                                    │
                                    ▼
NotificationSvc ──publishes──► BookingConfirmed
```

If payment fails → PaymentFailed event → HotelService releases room → BookingService cancels

### Orchestration-based Saga
```
SagaOrchestrator controls:
  1. Reserve room in HotelService
  2. Charge payment in PaymentService
  3. Send notification
  If any step fails → execute compensating transactions
```

---

## CQRS (Command Query Responsibility Segregation)

Separate read and write models.

```go
// Write side — normalized, transactional
type BookingCommandRepository interface {
    Create(ctx context.Context, booking Booking) error
    Cancel(ctx context.Context, bookingID string) error
}

// Read side — denormalized, optimized for queries
type BookingQueryRepository interface {
    GetUserBookings(ctx context.Context, userID string) ([]BookingSummary, error)
    GetBookingDetail(ctx context.Context, bookingID string) (*BookingDetail, error)
}
```

**Benefits**:
- Write side: optimized for consistency (SQL with transactions)
- Read side: optimized for performance (denormalized, can use Elasticsearch/Redis)
- Scale independently

---

## Interview Q&A

**Q: What is the difference between API Gateway and Service Mesh?**
> A: API Gateway sits at the **north-south** boundary — it handles traffic from external clients into the system. It focuses on entry-point concerns: authentication, rate limiting, routing. Service Mesh sits at the **east-west** boundary — it manages traffic *between* internal microservices. It focuses on reliability (retries, timeouts), security (mTLS), and observability. In practice, you'd use both: API Gateway for client-facing traffic, Service Mesh for internal service-to-service calls.

**Q: How would you implement a distributed transaction for a hotel booking?**
> A: I'd use the Saga pattern, specifically orchestration-based for better visibility and control. The BookingOrchestrator would: (1) Reserve room in HotelService, (2) Charge in PaymentService, (3) Send confirmation. Each step has a compensating transaction: if payment fails, release the room reservation. I'd use Kafka as the event bus for reliability. The key advantage over 2PC is that services don't need to be available simultaneously — they process events asynchronously with local transactions, maintaining eventual consistency.

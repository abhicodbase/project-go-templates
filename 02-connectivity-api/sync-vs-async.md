# Sync vs Async Communication

## Synchronous (Request-Response)

```
Client ──────────────────────────────────► Service
       ◄──────────────────────────────────
       (client blocks waiting for response)
```

**Protocols**: HTTP/REST, gRPC, GraphQL

**Use when**:
- Client needs the result immediately to continue
- Strong consistency required
- Simple query/response pattern
- Example: `GET /hotels/123` — user is waiting for hotel details

---

## Asynchronous (Message-Based)

```
Publisher ──► Message Broker (Kafka/RabbitMQ) ──► Subscriber
              (broker persists message)             (processes when ready)
```

**Use when**:
- Client doesn't need the result immediately
- Work takes a long time (email sending, PDF generation)
- Durability needed (message survives crashes)
- Fan-out: one event, multiple consumers
- Example: BookingCreated → triggers notification + invoice + analytics

---

## Decision Matrix

| Factor | Sync | Async |
|--------|------|-------|
| **Response time** | Low latency | Higher latency |
| **Coupling** | Tight (caller knows callee) | Loose (via broker) |
| **Availability** | Both services must be up | Broker buffers if consumer down |
| **Flow control** | Natural (back-pressure) | Need to manage queue depth |
| **Error handling** | Immediate | Need DLQ, retry logic |
| **Ordering** | Guaranteed | Depends on broker |
| **Simplicity** | Simple | Complex (infra, monitoring) |

---

## Hybrid Pattern (Best of Both Worlds)

```go
// Sync API that triggers async processing

// 1. Client calls: POST /bookings
// 2. Service creates booking record (PENDING state) + publishes event
// 3. Returns 202 Accepted with booking ID immediately
// 4. Background worker processes the booking asynchronously
// 5. Client polls GET /bookings/{id} or receives webhook when done

func (h *BookingHandler) Create(w http.ResponseWriter, r *http.Request) {
    var req CreateBookingRequest
    json.NewDecoder(r.Body).Decode(&req)

    // Synchronous: persist booking as PENDING
    booking := &Booking{
        ID:     uuid.New().String(),
        Status: "PENDING",
        ...
    }
    h.repo.Create(r.Context(), booking)

    // Async: publish event for processing
    h.publisher.Publish(r.Context(), "booking.created", BookingCreatedEvent{
        BookingID: booking.ID,
        ...
    })

    // Return 202 Accepted — processing happens asynchronously
    w.Header().Set("Location", "/bookings/"+booking.ID)
    writeJSON(w, http.StatusAccepted, map[string]string{
        "booking_id": booking.ID,
        "status": "PENDING",
        "message": "Booking is being processed",
    })
}

// Client polls for status
func (h *BookingHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    booking, _ := h.repo.Get(r.Context(), id)
    writeJSON(w, http.StatusOK, booking) // status: PENDING → CONFIRMED → FAILED
}
```

---

## Backpressure

Preventing a fast producer from overwhelming a slow consumer.

```go
// Approach 1: Bounded channel (built-in Go backpressure)
jobs := make(chan Job, 100) // if full, producer blocks

// Approach 2: Reject with 503 when queue is full
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
    select {
    case h.jobQueue <- job:
        writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
    default:
        // Queue full — reject
        w.Header().Set("Retry-After", "30")
        writeError(w, http.StatusServiceUnavailable, "QUEUE_FULL", "System busy, please retry")
    }
}
```

---

## Interview Q&A

**Q: You need to send a confirmation email after a booking. Would you do this synchronously or asynchronously?**
> A: Asynchronously. The booking creation should succeed or fail on its own merits, independently of whether the email can be sent. If we do email sending synchronously: (1) if the email service is down, bookings fail — bad UX; (2) email sending adds latency to the booking API response. Async with Kafka: publish a `BookingConfirmed` event, email service consumes it and sends the email. If email fails, we retry with backoff without affecting the booking. The trade-off: email may arrive slightly later than the booking confirmation page, but that's acceptable.

**Q: How do you ensure an async message is not lost?**
> A: Three mechanisms: (1) **Outbox pattern** — write the event to an outbox table in the same DB transaction as the booking; a separate poller reads and publishes to Kafka; (2) **Durable message broker** — Kafka persists messages to disk and replicates; (3) **At-least-once delivery** with idempotent consumers — the consumer marks which messages it has processed, so redeliveries are handled safely. The combination ensures no message is lost even if the app crashes between creating the booking and publishing the event.

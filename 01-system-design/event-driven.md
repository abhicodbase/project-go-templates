# Event-Driven Architecture

## Core Concepts

**Event**: Something that happened in the past ("BookingCreated", "PaymentProcessed")

### Event vs Command vs Query
| | Event | Command | Query |
|---|---|---|---|
| **Nature** | Happened (past tense) | Do this (imperative) | Ask this |
| **Coupling** | Loose — publisher doesn't know consumers | Tight — knows the target | N/A |
| **Response** | None expected | May return ack | Returns data |
| **Example** | `BookingCreated` | `CreateBooking` | `GetBooking` |

---

## Event Sourcing

**Idea**: Store *events* as the source of truth, not current state.

```
Traditional: DB stores current state
  bookings table: {id: 123, status: "confirmed", hotel: "Grand"}

Event Sourced: DB stores events
  booking_events:
    {seq:1, type: "BookingCreated", data: {...}}
    {seq:2, type: "PaymentReceived", data: {...}}
    {seq:3, type: "BookingConfirmed", data: {...}}
  
  Current state = replay of all events
```

```go
type Event struct {
    ID        string
    Type      string
    AggregateID string
    Data      json.RawMessage
    Timestamp time.Time
    Version   int
}

type BookingAggregate struct {
    ID      string
    Status  string
    HotelID string
    Version int
    changes []Event // uncommitted events
}

func (b *BookingAggregate) Apply(e Event) {
    switch e.Type {
    case "BookingCreated":
        var data BookingCreatedData
        json.Unmarshal(e.Data, &data)
        b.HotelID = data.HotelID
        b.Status = "pending"
    case "PaymentReceived":
        b.Status = "confirmed"
    case "BookingCancelled":
        b.Status = "cancelled"
    }
    b.Version = e.Version
}

// Rebuild state from events
func RebuildBooking(events []Event) *BookingAggregate {
    booking := &BookingAggregate{}
    for _, e := range events {
        booking.Apply(e)
    }
    return booking
}
```

**Benefits**:
- Complete audit trail
- Time-travel (state at any point in time)
- Enables event replay, projections

**Downsides**:
- Eventual consistency for read models
- Event schema evolution is hard
- Storage grows unboundedly (mitigated by snapshots)

---

## Outbox Pattern

Guarantee that domain events are published even if the broker is temporarily unavailable.

```
┌─────────────────────────────────────────────────────┐
│                   Single Transaction                 │
│  1. Write booking to bookings table                 │
│  2. Write event to outbox table (same DB tx)        │
└─────────────────────────────────────────────────────┘
                          │
                ┌─────────▼──────────┐
                │   Outbox Poller    │  (background goroutine)
                │ reads unpublished  │
                │ events from DB     │
                └─────────┬──────────┘
                          │
                          ▼
                     Kafka Topic

```

```go
// In the booking service — single transaction
func (s *BookingService) CreateBooking(ctx context.Context, req CreateBookingRequest) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // 1. Create booking
    booking := Booking{ID: uuid.New().String(), HotelID: req.HotelID}
    if err := s.repo.CreateTx(ctx, tx, booking); err != nil {
        return err
    }

    // 2. Insert into outbox (same transaction — atomicity!)
    event := OutboxEvent{
        ID:      uuid.New().String(),
        Type:    "BookingCreated",
        Payload: mustMarshal(BookingCreatedPayload{BookingID: booking.ID}),
        Status:  "pending",
    }
    if err := s.outboxRepo.InsertTx(ctx, tx, event); err != nil {
        return err
    }

    return tx.Commit()
}

// Separate outbox poller goroutine
func (p *OutboxPoller) Run(ctx context.Context) {
    ticker := time.NewTicker(500 * time.Millisecond)
    for {
        select {
        case <-ticker.C:
            events, _ := p.outboxRepo.GetPending(ctx, 100)
            for _, e := range events {
                if err := p.producer.Publish(ctx, e); err == nil {
                    p.outboxRepo.MarkPublished(ctx, e.ID)
                }
            }
        case <-ctx.Done():
            return
        }
    }
}
```

---

## Interview Q&A

**Q: What is the difference between Event Sourcing and CQRS? Do they have to go together?**
> A: They're complementary but independent patterns. CQRS separates the read and write models — you have different representations for commands (writes) and queries (reads). Event Sourcing is about *how* you persist state — by storing events rather than current state. You can use CQRS without Event Sourcing (e.g., separate read/write DB models with regular state). You can also use Event Sourcing without CQRS (though it's unusual). They work very well together: Event Sourcing provides the write side (event log), and you build CQRS read projections by consuming those events.

**Q: How do you ensure an event is processed exactly once in an event-driven system?**
> A: Truly "exactly-once" is very hard. In practice, we design for **at-least-once delivery** + **idempotent consumers**. The consumer checks if it has already processed an event (by storing processed event IDs in a DB or using idempotent operations). For example, "reserve room for booking 123" is idempotent — if processed twice, the second time sees the room is already reserved and skips. Combined with the Outbox pattern for reliable publishing, this gives us effective exactly-once semantics from the business perspective.

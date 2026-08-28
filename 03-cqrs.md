# CQRS (Command Query Responsibility Segregation)

## Use case
Your order write model is normalized (orders table, users table). But the "show me
this user's order history with running totals" screen would need an expensive join +
aggregation on every read if served directly from the write model. CQRS splits reads
and writes into separate models: writes go to a normal transactional store; a
projector asynchronously updates a denormalized read model shaped exactly for the
queries the UI needs.

## Diagram

```
   Command (PlaceOrder)
          │
          v
   ┌─────────────┐        event         ┌────────────┐
   │ Write Store   │ ──────────────────> │  Projector   │
   │ (normalized)  │   "OrderPlaced"      │ (async)      │
   └─────────────┘                       └──────┬─────┘
                                                   │ updates
                                                   v
                                          ┌─────────────────┐
   Query (GetUserSummary) <────────────── │  Read Store       │
                                          │ (denormalized,     │
                                          │  purpose-built)    │
                                          └─────────────────┘
```

## Code

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// ---- Write side ----

type Order struct {
	ID     string
	UserID string
	Amount float64
	Status string
}

type WriteStore struct {
	mu     sync.Mutex
	orders map[string]Order
}

func NewWriteStore() *WriteStore { return &WriteStore{orders: make(map[string]Order)} }

// PlaceOrderCommand is the write path. It only touches the write model,
// then emits an event — it does NOT update the read model directly,
// keeping the two sides decoupled.
func (w *WriteStore) PlaceOrderCommand(o Order, onCommitted func(Order)) {
	w.mu.Lock()
	o.Status = "PLACED"
	w.orders[o.ID] = o
	w.mu.Unlock()
	onCommitted(o) // in production: publish an event instead of a direct callback
}

// ---- Read side ----

// UserOrderSummary is denormalized, shaped exactly for the query
// "user's recent orders with running total" — no joins needed at read time.
type UserOrderSummary struct {
	UserID      string
	OrderCount  int
	TotalSpent  float64
	LastOrderAt time.Time
}

type ReadStore struct {
	mu       sync.RWMutex
	byUserID map[string]*UserOrderSummary
}

func NewReadStore() *ReadStore { return &ReadStore{byUserID: make(map[string]*UserOrderSummary)} }

// ProjectOrderPlaced is the projector — consumes the write side's event
// and updates the read model. The window between write commit and this
// projection applying IS the eventual-consistency cost of CQRS.
func (r *ReadStore) ProjectOrderPlaced(o Order) {
	r.mu.Lock()
	defer r.mu.Unlock()
	summary, ok := r.byUserID[o.UserID]
	if !ok {
		summary = &UserOrderSummary{UserID: o.UserID}
		r.byUserID[o.UserID] = summary
	}
	summary.OrderCount++
	summary.TotalSpent += o.Amount
	summary.LastOrderAt = time.Now()
}

func (r *ReadStore) GetUserSummary(userID string) (UserOrderSummary, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byUserID[userID]
	if !ok {
		return UserOrderSummary{}, false
	}
	return *s, true
}

func main() {
	writeStore := NewWriteStore()
	readStore := NewReadStore()

	orders := []Order{
		{ID: "o1", UserID: "u1", Amount: 1200},
		{ID: "o2", UserID: "u1", Amount: 800},
		{ID: "o3", UserID: "u2", Amount: 500},
	}
	for _, o := range orders {
		writeStore.PlaceOrderCommand(o, readStore.ProjectOrderPlaced)
	}

	summary, _ := readStore.GetUserSummary("u1")
	fmt.Printf("u1 summary (read model): %+v\n", summary)
	fmt.Printf("write-side record for o1: %+v\n", writeStore.orders["o1"])
}
```

**Verified output:**
```
u1 summary (read model): {UserID:u1 OrderCount:2 TotalSpent:2000 LastOrderAt:...}
write-side record for o1: {ID:o1 UserID:u1 Amount:1200 Status:PLACED}
```

## Why it matters for the interview
- Answers "how would you serve a heavy read pattern without hammering the write DB
  with joins on every request" — a recurring flavor of question across dashboards,
  summaries, and search results (same idea as the flight-pricing cache, generalized).
- Be upfront about the tradeoff: **eventual consistency** on the read side. If asked
  "what if a user refreshes immediately after placing an order and doesn't see it,"
  the honest answer is: that's the cost of this pattern, mitigated by keeping the
  projection lag small or reading from the write model for that one specific
  post-action confirmation screen.
- Don't over-apply it — CQRS adds real complexity (two models, a projector, lag to
  manage). Use it when read and write patterns/scale genuinely diverge, not by default.

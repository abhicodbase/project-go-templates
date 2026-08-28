# Optimistic Locking (Preventing Overbooking)

## Use case
The classic "read-then-write" race: two concurrent buyers both read "1 seat left,"
both decide it's available, both try to buy it. Without protection, both succeed and
you've oversold the seat. Optimistic locking guards the write with a version check —
the write only succeeds if the row's version still matches what was read; otherwise
the caller retries against fresh state.

## Diagram

```
   Buyer A                          Buyer B
      │ Read seat (avail=1, v=0)        │ Read seat (avail=1, v=0)
      │                                  │
      │ CAS(v=0) -> avail=0, v=1  SUCCESS │
      │                                  │ CAS(v=0) -> v mismatch (now v=1)  FAIL
      │                                  │ Read seat again (avail=0, v=1)
      │                                  │ -> ErrOutOfStock, no retry
```

## Code

```go
package main

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var ErrOutOfStock = errors.New("no seats available")
var ErrVersionConflict = errors.New("version conflict, retry")

type Seat struct {
	ID        string
	Available int
	Version   int64
}

type SeatStore struct {
	mu    sync.Mutex
	seats map[string]*Seat
}

func NewSeatStore() *SeatStore { return &SeatStore{seats: make(map[string]*Seat)} }

func (s *SeatStore) Init(id string, available int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seats[id] = &Seat{ID: id, Available: available, Version: 0}
}

// Read returns a COPY — the caller "thinks" using this snapshot without
// holding any lock (that's the "optimistic" part).
func (s *SeatStore) Read(id string) Seat {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.seats[id]
}

// CompareAndDecrement mirrors:
//   UPDATE seats SET available = available - 1, version = version + 1
//   WHERE id = ? AND version = ? AND available > 0
// — one atomic operation, guarded by both version AND positive-availability,
// so it can never go negative under concurrency.
func (s *SeatStore) CompareAndDecrement(id string, expectedVersion int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	seat, ok := s.seats[id]
	if !ok {
		return errors.New("seat not found")
	}
	if seat.Version != expectedVersion {
		return ErrVersionConflict
	}
	if seat.Available <= 0 {
		return ErrOutOfStock
	}
	seat.Available--
	seat.Version++
	return nil
}

func BuySeat(store *SeatStore, seatID, buyerName string, successCount *int32) {
	for attempt := 1; attempt <= 5; attempt++ {
		current := store.Read(seatID)
		err := store.CompareAndDecrement(seatID, current.Version)
		if err == nil {
			fmt.Printf("  %s bought seat %s (attempt %d)\n", buyerName, seatID, attempt)
			atomic.AddInt32(successCount, 1)
			return
		}
		if errors.Is(err, ErrOutOfStock) {
			fmt.Printf("  %s failed: seat %s is sold out (attempt %d)\n", buyerName, seatID, attempt)
			return
		}
		// version conflict — someone else won the race, retry against fresh state
	}
	fmt.Printf("  %s gave up on seat %s after 5 retries\n", buyerName, seatID)
}

func main() {
	store := NewSeatStore()
	store.Init("12A", 1)

	var wg sync.WaitGroup
	var successCount int32
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			BuySeat(store, "12A", fmt.Sprintf("buyer-%d", n), &successCount)
		}(i)
	}
	wg.Wait()

	final := store.Read("12A")
	fmt.Printf("\nfinal available=%d (must be 0, not negative), successful buyers=%d (must be exactly 1)\n",
		final.Available, successCount)
}
```

**Verified output:**
```
  buyer-5 bought seat 12A (attempt 1)
  buyer-1 failed: seat 12A is sold out (attempt 1)
  buyer-2 failed: seat 12A is sold out (attempt 1)
  buyer-3 failed: seat 12A is sold out (attempt 1)
  buyer-4 failed: seat 12A is sold out (attempt 1)

final available=0 (must be 0, not negative), successful buyers=1 (must be exactly 1)
```
5 concurrent buyers, exactly 1 winner, availability never goes negative.

## Why it matters for the interview
- This is the direct fix for the overbooking scenario from the earlier snippet list —
  bring it up whenever you spot a "check availability, then decrement" pattern.
- Know the tradeoff against **pessimistic locking** (a DB row lock held for the whole
  operation): optimistic is better under low contention (no lock held during network
  round trips or business logic); pessimistic is simpler to reason about under high
  contention where retries would be constant and wasteful.
- Follow-up to expect: "what if a buyer's retry loop never wins under heavy
  contention?" — worth mentioning a max-retry cap with a clear failure (as shown) or
  falling back to a short pessimistic lock only under detected high contention.

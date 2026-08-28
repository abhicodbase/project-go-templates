# Bulkhead Pattern

## Use case
The Loan Eligibility Engine uses a single shared DB connection pool across every
endpoint in the monolith. If the Fraud Scoring call spikes to 2-3s under load, it can
hold onto enough shared resources to starve completely unrelated endpoints (like KYC
checks) that have nothing to do with fraud scoring. The bulkhead pattern isolates
resource pools per dependency, so one saturated dependency can only exhaust its own
pool.

## Diagram

```
   ┌─────────────────────────┐      ┌─────────────────────────┐
   │   FraudService Bulkhead    │      │   KYCService Bulkhead      │
   │   capacity = 2               │      │   capacity = 5                │
   │   [busy][busy]                │      │   [free][free][free][free]... │
   │   4 more calls REJECTED       │      │   calls succeed normally,      │
   │   fast (bulkhead full)         │      │   totally unaffected            │
   └─────────────────────────┘      └─────────────────────────┘
        both draw from separate pools — saturating one never blocks the other
```

## Code

```go
package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrBulkheadFull = errors.New("bulkhead at capacity, rejecting call")

// Bulkhead isolates a resource pool (here, concurrency slots) per
// dependency. Named after ship bulkheads: one compartment flooding
// doesn't sink the ship.
type Bulkhead struct {
	name string
	sem  chan struct{} // buffered channel used as a counting semaphore
}

func NewBulkhead(name string, maxConcurrent int) *Bulkhead {
	return &Bulkhead{name: name, sem: make(chan struct{}, maxConcurrent)}
}

// Execute runs fn only if a slot is free in THIS bulkhead; otherwise it
// fails fast rather than queuing indefinitely.
func (b *Bulkhead) Execute(fn func() error) error {
	select {
	case b.sem <- struct{}{}:
		defer func() { <-b.sem }()
		return fn()
	default:
		return fmt.Errorf("%s: %w", b.name, ErrBulkheadFull)
	}
}

func main() {
	fraudBulkhead := NewBulkhead("FraudService", 2)
	kycBulkhead := NewBulkhead("KYCService", 5)

	slowFraudCall := func() error {
		time.Sleep(300 * time.Millisecond) // simulates the 2-3s graph traversal, shortened for the demo
		return nil
	}
	fastKYCCall := func() error {
		time.Sleep(20 * time.Millisecond)
		return nil
	}

	var wg sync.WaitGroup

	// Flood the fraud bulkhead: 6 concurrent calls against capacity 2.
	for i := 1; i <= 6; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			err := fraudBulkhead.Execute(slowFraudCall)
			fmt.Printf("fraud call %d: err=%v\n", n, err)
		}(i)
	}

	// Meanwhile, KYC calls keep succeeding on their own pool — unaffected.
	time.Sleep(50 * time.Millisecond)
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			err := kycBulkhead.Execute(fastKYCCall)
			fmt.Printf("  kyc call %d: err=%v (unaffected by fraud bulkhead saturation)\n", n, err)
		}(i)
	}

	wg.Wait()
}
```

**Verified output:**
```
fraud call 2: err=FraudService: bulkhead at capacity, rejecting call
fraud call 3: err=FraudService: bulkhead at capacity, rejecting call
fraud call 4: err=FraudService: bulkhead at capacity, rejecting call
fraud call 5: err=FraudService: bulkhead at capacity, rejecting call
  kyc call 2: err=<nil> (unaffected by fraud bulkhead saturation)
  kyc call 3: err=<nil> (unaffected by fraud bulkhead saturation)
  kyc call 1: err=<nil> (unaffected by fraud bulkhead saturation)
fraud call 1: err=<nil>
fraud call 6: err=<nil>
```
4 of 6 flooding fraud calls get rejected immediately (capacity 2), while all 3 KYC
calls succeed on their own separate pool the entire time.

## Why it matters for the interview
- This is the direct, named fix for "single shared DB connection pool across all
  endpoints" — one of the most repeated anti-patterns across your prep scenarios.
- Pairs naturally with the circuit breaker: bulkheads limit **concurrency**, circuit
  breakers stop calling a **known-failing** dependency — they solve different
  problems and are often used together.
- Follow-up to expect: "how do you size each bulkhead?" — answer in terms of the
  dependency's own capacity and your latency budget, e.g., "if Fraud Scoring can
  handle 2 concurrent graph traversals without degrading further, I size the bulkhead
  there and let excess calls fail fast rather than queue and add latency everywhere."

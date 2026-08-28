# Rate Limiter (Token Bucket)

## Use case
Each flight-pricing supplier's API is metered — "Supplier A allows 500 calls/min."
You need to enforce that budget centrally (not per-pod, which would silently multiply
your effective limit by the number of pods) and allow short bursts up to a cap while
smoothing out sustained rate.

## Diagram

```
   ┌─────────────────────────────┐
   │        Token Bucket           │   refills continuously
   │   capacity = 3, refill=1/sec  │   at refillRate, capped
   │   [●][●][ ]                   │   at capacity
   └─────────────────────────────┘
         │ each call consumes
         │ 1 token if available
         v
   call allowed / rejected

   Per-supplier: map[supplierID] -> its own bucket,
   held in ONE centralized limiter (e.g., backed by Redis in prod)
   so all pods share the same budget.
```

## Code

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// TokenBucket rate limiter. Tokens refill continuously at `refillRate`
// per second, up to `capacity`. Centralized (single struct, single mutex)
// specifically to avoid the "in-memory limiter per pod" bug — in a real
// horizontally-scaled service this state would live in Redis (INCR +
// EXPIRE, or a Lua script) so all pods share the same budget.
type TokenBucket struct {
	mu         sync.Mutex
	capacity   float64
	tokens     float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func NewTokenBucket(capacity, refillRate float64) *TokenBucket {
	return &TokenBucket{capacity: capacity, tokens: capacity, refillRate: refillRate, lastRefill: time.Now()}
}

func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	tb.lastRefill = now

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

// PerSupplierLimiter maps each supplier to its own bucket — the
// per-dependency budget from the flight-pricing design, enforced
// centrally rather than per-pod.
type PerSupplierLimiter struct {
	mu       sync.Mutex
	limiters map[string]*TokenBucket
}

func NewPerSupplierLimiter() *PerSupplierLimiter {
	return &PerSupplierLimiter{limiters: make(map[string]*TokenBucket)}
}

func (p *PerSupplierLimiter) Allow(supplierID string, capacity, refillRate float64) bool {
	p.mu.Lock()
	tb, ok := p.limiters[supplierID]
	if !ok {
		tb = NewTokenBucket(capacity, refillRate)
		p.limiters[supplierID] = tb
	}
	p.mu.Unlock()
	return tb.Allow()
}

func main() {
	limiter := NewPerSupplierLimiter()
	for i := 1; i <= 10; i++ {
		allowed := limiter.Allow("supplier-A", 3, 1)
		fmt.Printf("call %2d: allowed=%v\n", i, allowed)
		if i == 5 {
			fmt.Println("   (sleeping 2s to let bucket refill...)")
			time.Sleep(2 * time.Second)
		}
	}
}
```

**Verified output:**
```
call  1: allowed=true
call  2: allowed=true
call  3: allowed=true
call  4: allowed=false
call  5: allowed=false
   (sleeping 2s to let bucket refill...)
call  6: allowed=true
call  7: allowed=true
call  8: allowed=false
call  9: allowed=false
call 10: allowed=false
```
Note calls 6-7 succeed (2 seconds of refill at 1/sec ≈ 2 tokens), then it's exhausted
again — exactly the "burst then smooth" behavior token bucket is designed for.

## Why it matters for the interview
- Have the **comparison ready**: token bucket allows bursts up to capacity; **leaky
  bucket** smooths output to a constant rate (no bursts); **sliding window** counts
  requests in a rolling time window, more precise but more expensive to compute.
- The critical follow-up: "does this work correctly across multiple pods?" — the
  honest answer for this in-memory version is **no**, and that's exactly why
  production versions centralize state in Redis with an atomic Lua script (INCR +
  TTL) rather than each pod holding its own bucket.

# Circuit Breaker Pattern

## Use case
Your Fraud Scoring service (graph traversal, can spike to 2-3s under load) starts
timing out. Without a circuit breaker, every request keeps calling it, piling up
latency and connection usage on a dependency that's already struggling — you can
cascade a slow dependency into a full outage. A circuit breaker detects repeated
failures, stops calling the dependency for a cooldown window, and periodically tests
whether it has recovered.

## Diagram

```
        failures >= threshold
   ┌───────────────────────────────┐
   │                                 v
┌────────┐                      ┌────────┐
│ CLOSED │  -- success reset -- │  OPEN  │
│ (calls  │ <------------------- │ (fails  │
│  pass)  │                      │  fast)  │
└────────┘                      └────────┘
     ^                                │
     │        success on trial call    │ cooldown elapsed
     │                                 v
     │                          ┌───────────┐
     └───────- fail again ------│ HALF-OPEN │
                                 │ (1 trial  │
                                 │  call)    │
                                 └───────────┘
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

// State of the breaker.
type State int

const (
	Closed State = iota // normal operation, calls pass through
	Open                // tripped, calls fail fast without hitting the dependency
	HalfOpen            // trial period, allow one call through to test recovery
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type CircuitBreaker struct {
	mu               sync.Mutex
	state            State
	failureThreshold int
	consecutiveFails int
	cooldown         time.Duration
	openedAt         time.Time
}

func NewCircuitBreaker(failureThreshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{failureThreshold: failureThreshold, cooldown: cooldown}
}

// Call wraps a dependency call. If the breaker is Open and cooldown hasn't
// elapsed, it fails fast without ever invoking fn — this is what protects
// a struggling downstream from being hammered further, and protects the
// caller from wasting its own resources waiting on a known-bad dependency.
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	if cb.state == Open {
		if time.Since(cb.openedAt) < cb.cooldown {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
		cb.state = HalfOpen // cooldown elapsed — allow a single trial call
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.consecutiveFails++
		if cb.state == HalfOpen || cb.consecutiveFails >= cb.failureThreshold {
			cb.state = Open
			cb.openedAt = time.Now()
		}
		return err
	}

	cb.consecutiveFails = 0
	cb.state = Closed
	return nil
}

func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

func main() {
	cb := NewCircuitBreaker(3, 300*time.Millisecond)

	callCount := 0
	flakyDependency := func() error {
		callCount++
		if callCount <= 3 {
			return errors.New("downstream timeout")
		}
		return nil
	}

	for i := 1; i <= 6; i++ {
		err := cb.Call(flakyDependency)
		fmt.Printf("call %d: state=%v err=%v\n", i, cb.State(), err)
	}

	fmt.Println("waiting for cooldown...")
	time.Sleep(350 * time.Millisecond)

	err := cb.Call(flakyDependency)
	fmt.Printf("call after cooldown: state=%v err=%v\n", cb.State(), err)
}
```

**Verified output:**
```
call 1: state=0 err=downstream timeout
call 2: state=0 err=downstream timeout
call 3: state=1 err=downstream timeout      <- 3rd failure trips the breaker (state 1 = Open)
call 4: state=1 err=circuit breaker is open <- fails fast, doesn't even call flakyDependency
call 5: state=1 err=circuit breaker is open
call 6: state=1 err=circuit breaker is open
waiting for cooldown...
call after cooldown: state=0 err=<nil>      <- half-open trial succeeded, back to Closed
```

## Why it matters for the interview
- Directly answers "what happens when a dependency is slow/down" in any system design.
- The half-open trial mechanism is the detail most candidates forget — without it,
  a breaker either never recovers automatically or thrashes open/closed on every call
  once cooldown ends.
- Follow-up to expect: "how do you choose the failure threshold and cooldown?" — answer
  in terms of the dependency's own recovery characteristics and your latency SLA, not
  an arbitrary number.

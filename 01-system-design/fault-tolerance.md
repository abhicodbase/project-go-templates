# Fault Tolerance

## What Is Fault Tolerance?
The system continues operating (possibly at reduced capacity) when some components fail.

**Key principle**: *Assume failure will happen. Design for it.*

---

## Circuit Breaker Pattern

Prevents cascading failures when a downstream service is failing.

```
         ┌─────────────────────────────────────────┐
         │              CLOSED (normal)             │
         │  Requests flow through freely            │
         │  Count failures                          │
         └────────────────┬────────────────────────┘
                          │ failure threshold exceeded
                          ▼
         ┌─────────────────────────────────────────┐
         │              OPEN (tripped)             │
         │  Fail fast — return error immediately   │
         │  No calls to downstream                 │
         └────────────────┬────────────────────────┘
                          │ after timeout period
                          ▼
         ┌─────────────────────────────────────────┐
         │           HALF-OPEN (probing)           │
         │  Allow limited requests through         │
         │  Success → back to CLOSED               │
         │  Failure → back to OPEN                 │
         └─────────────────────────────────────────┘
```

### Go Implementation
```go
package circuitbreaker

import (
    "errors"
    "sync"
    "time"
)

type State int

const (
    StateClosed   State = iota // Normal operation
    StateOpen                  // Failing fast
    StateHalfOpen              // Testing recovery
)

type CircuitBreaker struct {
    mu              sync.Mutex
    state           State
    failureCount    int
    failureThreshold int
    successCount    int
    successThreshold int
    openUntil       time.Time
    timeout         time.Duration
}

var ErrCircuitOpen = errors.New("circuit breaker is open")

func New(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        state:            StateClosed,
        failureThreshold: failureThreshold,
        successThreshold: successThreshold,
        timeout:          timeout,
    }
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    switch cb.state {
    case StateOpen:
        if time.Now().Before(cb.openUntil) {
            return ErrCircuitOpen // Fail fast
        }
        // Timeout elapsed — try half-open
        cb.state = StateHalfOpen
        cb.successCount = 0

    case StateClosed, StateHalfOpen:
        // proceed
    }

    err := fn()

    if err != nil {
        cb.onFailure()
        return err
    }
    cb.onSuccess()
    return nil
}

func (cb *CircuitBreaker) onFailure() {
    cb.failureCount++
    if cb.state == StateHalfOpen || cb.failureCount >= cb.failureThreshold {
        cb.state = StateOpen
        cb.openUntil = time.Now().Add(cb.timeout)
        cb.failureCount = 0
    }
}

func (cb *CircuitBreaker) onSuccess() {
    if cb.state == StateHalfOpen {
        cb.successCount++
        if cb.successCount >= cb.successThreshold {
            cb.state = StateClosed
            cb.failureCount = 0
        }
    } else {
        cb.failureCount = 0 // Reset on success in closed state
    }
}

// Usage
func callHotelSupplier(cb *CircuitBreaker) error {
    return cb.Execute(func() error {
        // call external hotel API
        return fetchFromHotelAPI()
    })
}
```

---

## Retry with Exponential Backoff

```go
package retry

import (
    "context"
    "math"
    "time"
)

type RetryConfig struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
    Multiplier  float64
}

func WithRetry(ctx context.Context, cfg RetryConfig, fn func() error) error {
    var lastErr error
    for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
        if err := ctx.Err(); err != nil {
            return err // Context cancelled
        }

        lastErr = fn()
        if lastErr == nil {
            return nil
        }

        if attempt == cfg.MaxAttempts-1 {
            break // Last attempt, don't sleep
        }

        // Exponential backoff with jitter
        delay := time.Duration(float64(cfg.BaseDelay) * math.Pow(cfg.Multiplier, float64(attempt)))
        if delay > cfg.MaxDelay {
            delay = cfg.MaxDelay
        }
        // Add jitter: ±10% of delay
        jitter := time.Duration(float64(delay) * 0.1)
        delay = delay - jitter/2 + time.Duration(float64(jitter)*rand.Float64())

        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(delay):
        }
    }
    return lastErr
}

// Usage
err := WithRetry(ctx, RetryConfig{
    MaxAttempts: 3,
    BaseDelay:   100 * time.Millisecond,
    MaxDelay:    5 * time.Second,
    Multiplier:  2.0,
}, func() error {
    return callExternalAPI()
})
```

**Jitter is critical**: Without jitter, all retrying clients hit the server at the same time (thundering herd). Jitter spreads them out.

---

## Bulkhead Pattern

Isolate failures to prevent one component from consuming all resources.

```go
// Bulkhead using semaphores (limit concurrent calls per service)
type Bulkhead struct {
    sem chan struct{}
}

func NewBulkhead(maxConcurrent int) *Bulkhead {
    return &Bulkhead{
        sem: make(chan struct{}, maxConcurrent),
    }
}

func (b *Bulkhead) Execute(fn func() error) error {
    select {
    case b.sem <- struct{}{}: // acquire slot
        defer func() { <-b.sem }() // release slot
        return fn()
    default:
        return errors.New("bulkhead: too many concurrent requests")
    }
}

// Separate bulkheads per downstream service
var (
    hotelAPIBulkhead   = NewBulkhead(20)  // max 20 concurrent hotel API calls
    paymentBulkhead    = NewBulkhead(10)  // max 10 concurrent payment calls
    notificationBulkhead = NewBulkhead(50) // notifications can have more
)
```

---

## Timeout

Every external call must have a timeout. No exceptions.

```go
func callWithTimeout(url string) ([]byte, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            return nil, fmt.Errorf("upstream timeout after 2s")
        }
        return nil, err
    }
    defer resp.Body.Close()
    return io.ReadAll(resp.Body)
}
```

---

## Graceful Degradation

Return partial/cached results when a dependency fails.

```go
func getHotelPrices(ctx context.Context, hotelID string) ([]Price, error) {
    prices, err := liveSupplierAPI.GetPrices(ctx, hotelID)
    if err != nil {
        // Graceful degradation — return cached prices with warning
        log.Warn("Live prices unavailable, using cache", "hotel_id", hotelID, "err", err)
        return cache.GetLastKnownPrices(hotelID) // stale but better than nothing
    }
    cache.Set(hotelID, prices)
    return prices, nil
}
```

---

## Interview Q&A

**Q: How do you prevent cascading failures in a microservices system?**
> A: Three key patterns:
> 1. **Circuit Breaker** — detect failures and fail fast instead of waiting for timeout
> 2. **Bulkhead** — limit concurrent calls to each downstream, so one failing service can't consume all your goroutines/threads
> 3. **Timeout** — always set timeouts on outbound calls; without them, goroutines pile up waiting forever
> Plus **retry with exponential backoff + jitter** for transient failures, and **graceful degradation** to serve stale/partial data when live data is unavailable.

**Q: What's the difference between a retry and a circuit breaker?**
> A: Retry is optimistic — it assumes the failure is transient and tries again. Circuit breaker is pessimistic — it detects that the downstream is consistently failing and stops trying entirely for a period, giving the downstream time to recover. They complement each other: retry handles individual request failures, circuit breaker handles systemic downstream failures. Using retry without a circuit breaker during an outage can make things worse (thundering herd on a struggling service).

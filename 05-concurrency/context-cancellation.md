# Context & Cancellation in Go

## Why context.Context?

Context carries deadlines, cancellation signals, and request-scoped values across API boundaries and goroutine boundaries.

```go
// Context hierarchy — cancelling parent cancels all children
ctx := context.Background()                           // root
ctx, cancel := context.WithCancel(ctx)                // cancellable
ctx, cancel := context.WithTimeout(ctx, 5*time.Second) // timeout
ctx, cancel := context.WithDeadline(ctx, time.Now().Add(5*time.Second))
```

**Rule**: Always defer cancel() to prevent context leaks.

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel() // ← Always! Even if context expired naturally
```

---

## Propagating Context

```go
// Pass context as FIRST argument to every function that does I/O
func (s *HotelService) GetHotel(ctx context.Context, id string) (*Hotel, error) {
    // DB query respects context cancellation
    row := s.db.QueryRowContext(ctx, "SELECT * FROM hotels WHERE id = $1", id)
    
    // HTTP call respects context
    req, _ := http.NewRequestWithContext(ctx, "GET", supplierURL+id, nil)
    resp, err := http.DefaultClient.Do(req)
    
    return hotel, err
}

// ❌ Never ignore context
func badFunc(id string) (*Hotel, error) {
    // No context — can't be cancelled, no timeout
    row := db.QueryRow("SELECT * FROM hotels WHERE id = $1", id)
    ...
}
```

---

## Checking for Cancellation

```go
func longRunningJob(ctx context.Context, items []Item) error {
    for _, item := range items {
        // Check cancellation at each iteration
        select {
        case <-ctx.Done():
            return ctx.Err() // context.Canceled or context.DeadlineExceeded
        default:
        }
        
        if err := processItem(ctx, item); err != nil {
            return err
        }
    }
    return nil
}

// In goroutines:
go func() {
    for {
        select {
        case work := <-workCh:
            doWork(work)
        case <-ctx.Done():
            cleanup()
            return
        }
    }
}()
```

---

## Context Values (Use Sparingly!)

```go
type contextKey string

const (
    requestIDKey contextKey = "request_id"
    userIDKey    contextKey = "user_id"
)

// Store a value
ctx = context.WithValue(ctx, requestIDKey, "req-abc123")

// Retrieve a value
if reqID, ok := ctx.Value(requestIDKey).(string); ok {
    log.Info("handling request", "request_id", reqID)
}

// ✅ Good use cases for context values:
//   - Request ID (for distributed tracing)
//   - Authenticated user (from auth middleware)
//   - Tenant ID (for multi-tenancy)

// ❌ Don't store:
//   - Business data (pass as function params)
//   - Optional parameters (use functional options pattern)
//   - Mutable state
```

---

## Graceful Shutdown Pattern

```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())

    // Listen for OS signals
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

    // Start server
    srv := &http.Server{Addr: ":8080", Handler: router}
    
    go func() {
        if err := srv.ListenAndServe(); err != http.ErrServerClosed {
            log.Fatal("server error", "err", err)
        }
    }()

    // Wait for shutdown signal
    <-sigCh
    log.Info("shutting down...")
    cancel() // Cancel all in-flight operations

    // Give server 30 seconds to finish in-flight requests
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()
    
    if err := srv.Shutdown(shutdownCtx); err != nil {
        log.Error("shutdown error", "err", err)
    }
    log.Info("shutdown complete")
}
```

---

## Context Timeout Cascade

```go
// Parent timeout: 5s for entire request
func handleSearchRequest(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    // DB query gets 2s of the 5s budget
    dbCtx, dbCancel := context.WithTimeout(ctx, 2*time.Second)
    defer dbCancel()
    hotels, err := searchDB(dbCtx, r.URL.Query())

    // External API gets 3s
    apiCtx, apiCancel := context.WithTimeout(ctx, 3*time.Second)
    defer apiCancel()
    prices, err := fetchPrices(apiCtx, hotels)
}
```

---

## Interview Q&A

**Q: Why does Go use context.Context instead of just passing timeouts as parameters?**
> A: Context solves three problems in one: (1) **Cancellation propagation** — when a parent operation is cancelled (e.g., client disconnects), all child operations can be cancelled automatically through the context tree; (2) **Deadline/timeout propagation** — a single deadline can cascade to all downstream calls without each function needing to manage its own timer; (3) **Request-scoped values** — data like request IDs and auth info flows through the call chain without polluting function signatures. Passing just a timeout wouldn't handle the cancellation signal or values.

**Q: What is the difference between context.WithCancel, WithTimeout, and WithDeadline?**
> A: WithCancel gives you a cancel function — you explicitly call cancel() to signal cancellation. WithTimeout takes a duration and automatically cancels after that duration (internally it calls WithDeadline). WithDeadline takes an absolute time.Time. In practice: use WithTimeout for "this operation should complete within 2 seconds"; use WithCancel for "cancel this when the user disconnects" (you signal cancellation via the cancel function); use WithDeadline when you have an absolute cutoff time.

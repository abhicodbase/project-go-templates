# Goroutines & Channels

## Goroutines

A goroutine is a lightweight thread managed by the Go runtime.

```go
// Start a goroutine — just add 'go'
go func() {
    fmt.Println("running concurrently")
}()

// Goroutine vs OS Thread:
// OS Thread: 1-8 MB stack, scheduled by OS
// Goroutine:  2 KB initial stack (grows dynamically), scheduled by Go runtime
// You can have millions of goroutines; thousands of OS threads is problematic
```

### Goroutine Leak — Most Common Interview Topic!

```go
// ❌ GOROUTINE LEAK — goroutine blocks forever if nobody reads from ch
func leak() {
    ch := make(chan int)
    go func() {
        ch <- 42 // blocks forever if nobody reads
    }()
    // function returns, goroutine is stuck
}

// ✅ Fixed — use buffered channel or ensure reader exists
func noLeak(ctx context.Context) {
    ch := make(chan int, 1) // buffered — send won't block
    go func() {
        select {
        case ch <- 42:
        case <-ctx.Done(): // exit if context cancelled
            return
        }
    }()
}

// ✅ Or use WaitGroup to ensure goroutines complete
func withWaitGroup() {
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            process(i)
        }(i) // pass i as argument — avoid closure capture bug!
    }
    wg.Wait()
}
```

---

## Channels

```go
// Unbuffered — sender blocks until receiver is ready
ch := make(chan int)

// Buffered — sender blocks only when buffer is full
ch := make(chan int, 10)

// Directional channels (in function signatures)
func producer(out chan<- int) { out <- 42 }    // send-only
func consumer(in <-chan int)  { v := <-in }     // receive-only

// Close a channel — signals no more values
close(ch)

// Range over channel — reads until closed
for v := range ch {
    fmt.Println(v)
}

// Non-blocking receive
select {
case v := <-ch:
    fmt.Println("received", v)
default:
    fmt.Println("nothing ready")
}
```

### Channel vs Mutex: When to Use Which?

| Use Channel when... | Use Mutex when... |
|---------------------|-------------------|
| Passing data between goroutines | Protecting shared state |
| Signaling / coordination | Simple counter/map access |
| Pipeline patterns | Performance-critical sections |
| Ownership transfer | Caching |

```go
// ✅ Channel for data passing
func generate(nums ...int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for _, n := range nums {
            out <- n
        }
    }()
    return out
}

// ✅ Mutex for shared state
type Cache struct {
    mu    sync.RWMutex
    items map[string]string
}

func (c *Cache) Get(key string) (string, bool) {
    c.mu.RLock() // multiple readers OK
    defer c.mu.RUnlock()
    v, ok := c.items[key]
    return v, ok
}

func (c *Cache) Set(key, val string) {
    c.mu.Lock() // exclusive write
    defer c.mu.Unlock()
    c.items[key] = val
}
```

---

## Select Statement

Multiplexes over multiple channels.

```go
func fanIn(ch1, ch2 <-chan string) <-chan string {
    merged := make(chan string)
    go func() {
        defer close(merged)
        for {
            select {
            case v, ok := <-ch1:
                if !ok { ch1 = nil; continue }
                merged <- v
            case v, ok := <-ch2:
                if !ok { ch2 = nil; continue }
                merged <- v
            }
            if ch1 == nil && ch2 == nil {
                return
            }
        }
    }()
    return merged
}

// Timeout pattern
func callWithTimeout(ch <-chan Result) (Result, error) {
    select {
    case result := <-ch:
        return result, nil
    case <-time.After(2 * time.Second):
        return Result{}, errors.New("timeout")
    }
}

// Done pattern with context
func worker(ctx context.Context, jobs <-chan Job) {
    for {
        select {
        case job, ok := <-jobs:
            if !ok { return }
            process(job)
        case <-ctx.Done():
            return // graceful shutdown
        }
    }
}
```

---

## Common Closure Bug

```go
// ❌ Bug — all goroutines capture the SAME 'i' variable
for i := 0; i < 5; i++ {
    go func() {
        fmt.Println(i) // prints 5,5,5,5,5 (not 0,1,2,3,4)
    }()
}

// ✅ Fix 1 — pass as argument
for i := 0; i < 5; i++ {
    go func(i int) {
        fmt.Println(i) // correctly prints 0,1,2,3,4
    }(i)
}

// ✅ Fix 2 — shadow the variable (Go 1.22+: for-range vars are per-iteration)
for i := 0; i < 5; i++ {
    i := i // create new variable in inner scope
    go func() {
        fmt.Println(i)
    }()
}
```

---

## Race Detection

```bash
# Run tests with race detector
go test -race ./...

# Run program with race detector  
go run -race main.go
```

```go
// ❌ Race condition
var counter int
for i := 0; i < 1000; i++ {
    go func() {
        counter++ // DATA RACE — concurrent read-modify-write
    }()
}

// ✅ Fix with atomic
var counter int64
for i := 0; i < 1000; i++ {
    go func() {
        atomic.AddInt64(&counter, 1)
    }()
}
```

---

## Interview Q&A

**Q: What is a goroutine leak and how do you prevent it?**
> A: A goroutine leak happens when a goroutine is started but never terminates — it stays alive forever, consuming memory and potentially holding resources. Common causes: blocking channel send/receive with no consumer, blocking on a mutex that's never released, infinite loop without exit. Prevention: always give goroutines a way to exit via `ctx.Done()` or closed channels, use `go vet` and goroutine leak detectors in tests (like `goleak`), and always ensure for every goroutine you start, there's a clear termination path.

**Q: Explain buffered vs unbuffered channels. When would you use each?**
> A: Unbuffered: the sender blocks until the receiver is ready, and vice versa. This provides synchronization — you know the receiver got the message before you continue. Buffered: the sender can send up to the buffer capacity without blocking. Use unbuffered for synchronization and handoff patterns. Use buffered when you want to decouple producer/consumer speeds, implement a semaphore (make(chan struct{}, N)), or when you want to allow a goroutine to send and exit without waiting for a receiver.

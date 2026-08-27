# Sync Primitives in Go

## sync.Mutex

```go
type SafeMap struct {
    mu sync.Mutex
    m  map[string]int
}

func (sm *SafeMap) Increment(key string) {
    sm.mu.Lock()
    defer sm.mu.Unlock() // Always defer unlock!
    sm.m[key]++
}

// ⚠️ Never copy a Mutex — pass by pointer
// ⚠️ Never lock in one goroutine and unlock in another
// ⚠️ Avoid calling external functions while holding a lock
```

## sync.RWMutex

```go
// Multiple readers OR one writer at a time
type Config struct {
    mu     sync.RWMutex
    values map[string]string
}

func (c *Config) Get(key string) string {
    c.mu.RLock()         // Multiple goroutines can hold RLock simultaneously
    defer c.mu.RUnlock()
    return c.values[key]
}

func (c *Config) Set(key, val string) {
    c.mu.Lock()          // Exclusive — blocks all readers and writers
    defer c.mu.Unlock()
    c.values[key] = val
}
```

**When to use RWMutex over Mutex?**
- Many goroutines reading, few writing (read-heavy)
- Read critical section is relatively expensive
- If writes are frequent, RWMutex adds overhead vs plain Mutex

---

## sync.WaitGroup

```go
func processAll(items []Item) {
    var wg sync.WaitGroup
    for _, item := range items {
        wg.Add(1)
        go func(item Item) {
            defer wg.Done() // Must call Done even if panic
            process(item)
        }(item)
    }
    wg.Wait() // Blocks until all Done() calls received
}

// ⚠️ Add() must be called BEFORE the goroutine starts
// ⚠️ Add() and Done() must balance — more Done() causes panic
```

---

## sync.Once

```go
// Ensures a function runs exactly once, even across multiple goroutines
var (
    instance *Database
    once     sync.Once
)

func GetDB() *Database {
    once.Do(func() {
        instance = &Database{} // This runs only once
        instance.Connect()
    })
    return instance
}

// Useful for: singletons, lazy initialization, one-time setup
```

---

## sync/atomic

```go
import "sync/atomic"

// Atomic counter — no mutex needed for simple increments
var requestCount int64

func handleRequest() {
    atomic.AddInt64(&requestCount, 1)
}

func getCount() int64 {
    return atomic.LoadInt64(&requestCount)
}

// Compare-and-swap (CAS) — basis of lock-free algorithms
func updateIfZero(ptr *int64, newVal int64) bool {
    return atomic.CompareAndSwapInt64(ptr, 0, newVal)
}

// atomic.Value — for any type (store/load arbitrary values atomically)
var config atomic.Value

func updateConfig(c *Config) {
    config.Store(c)
}

func getConfig() *Config {
    return config.Load().(*Config)
}
```

**atomic vs Mutex**: Atomic operations are faster but only work on simple types. Mutex is more flexible for complex critical sections.

---

## sync.Pool

```go
// Reuse expensive objects to reduce GC pressure
var bufPool = sync.Pool{
    New: func() interface{} {
        return &bytes.Buffer{}
    },
}

func processRequest(data []byte) {
    buf := bufPool.Get().(*bytes.Buffer)
    defer func() {
        buf.Reset()
        bufPool.Put(buf) // Return to pool for reuse
    }()

    buf.Write(data)
    // ... use buf
}

// Use for: HTTP request/response buffers, JSON encoders, byte slices
// ⚠️ Pool contents may be GC'd at any time — never store state there
```

---

## sync.Map

```go
// Thread-safe map — optimized for append-only or mostly-read workloads
var cache sync.Map

cache.Store("key", "value")

if v, ok := cache.Load("key"); ok {
    fmt.Println(v.(string))
}

// LoadOrStore — atomic get-or-create
actual, loaded := cache.LoadOrStore("key", "new_value")

// Delete
cache.Delete("key")

// Range — iterate (snapshot semantics)
cache.Range(func(k, v interface{}) bool {
    fmt.Printf("%v: %v\n", k, v)
    return true // continue iteration
})
```

**sync.Map vs RWMutex map**: Use sync.Map when:
- Keys are written once and read many times
- Multiple goroutines write different keys
Otherwise, prefer `map + RWMutex` (more explicit, slightly faster for same-key patterns)

---

## Interview Q&A

**Q: What is the difference between sync.Mutex and sync.RWMutex?**
> A: Mutex allows only one goroutine at a time, whether reading or writing. RWMutex allows multiple concurrent readers but only one writer (and writers exclude readers). Use RWMutex for read-heavy workloads where concurrent reads are common and writes are rare. The cost of RWMutex: slightly more overhead per operation due to read-count tracking. If writes are frequent, plain Mutex may be faster.

**Q: What is sync.Once used for?**
> A: sync.Once ensures a function is executed exactly once, even if called from multiple goroutines concurrently. It's used for thread-safe singleton initialization, lazy loading of expensive resources (DB connections, config files), and one-time setup. Unlike simply checking a boolean, Once handles the race condition where multiple goroutines see the boolean as false simultaneously and all try to initialize.

**Q: How does sync.Pool help with performance?**
> A: sync.Pool reduces GC pressure by reusing objects instead of allocating new ones. In high-throughput services, frequent allocation of temporary objects (buffers, request structs) causes GC pauses. Pool maintains a per-P (processor) free list, making Get/Put very cheap. Important caveat: pool contents are cleared at GC, so don't rely on pool for persistent storage.

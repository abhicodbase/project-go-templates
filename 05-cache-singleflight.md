# Cache-Aside with Single-Flight (Thundering Herd Prevention)

## Use case
A hot cache key (e.g., a popular flight route) expires. In the next instant, 50
concurrent user searches all miss simultaneously. Naively, all 50 would each trigger
their own expensive fetch (DB query or metered supplier API call) — wasting budget and
hammering the backend at exactly the worst moment. Single-flight coalesces concurrent
misses on the same key into ONE fetch; everyone else waits on that shared result.

## Diagram

```
   50 concurrent Get("hot-key") calls arrive at once
              │
              v
   ┌─────────────────────────┐
   │  is "hot-key" already      │  NO  ┌──────────────────┐
   │  in-flight?                 │────> │ become the fetcher │
   └─────────────────────────┘      │ run expensive fn()   │
              │ YES                    └────────┬─────────┘
              v                                   │ store result,
   wait on the same in-flight                     │ notify waiters
   call's result channel  <────────────────────────┘
```

## Code

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type entry struct {
	value     string
	expiresAt time.Time
}

type Cache struct {
	mu       sync.Mutex
	data     map[string]entry
	inFlight map[string]*call
	ttl      time.Duration
}

type call struct {
	done chan struct{}
	val  string
}

func NewCache(ttl time.Duration) *Cache {
	return &Cache{data: make(map[string]entry), inFlight: make(map[string]*call), ttl: ttl}
}

// Get returns the cached value if fresh; otherwise fetches via fn, but
// coalesces concurrent misses on the same key into a single fetch.
func (c *Cache) Get(key string, fn func() string) string {
	c.mu.Lock()
	if e, ok := c.data[key]; ok && time.Now().Before(e.expiresAt) {
		c.mu.Unlock()
		return e.value
	}

	if inFlight, ok := c.inFlight[key]; ok {
		c.mu.Unlock()
		<-inFlight.done // wait for the other goroutine's fetch to finish
		return inFlight.val
	}

	// We're the one who fetches — register BEFORE releasing the lock so
	// concurrent callers see it and wait instead of also fetching.
	call := &call{done: make(chan struct{})}
	c.inFlight[key] = call
	c.mu.Unlock()

	val := fn() // the expensive call — DB or partner API

	c.mu.Lock()
	c.data[key] = entry{value: val, expiresAt: time.Now().Add(c.ttl)}
	delete(c.inFlight, key)
	c.mu.Unlock()

	call.val = val
	close(call.done)
	return val
}

func main() {
	cache := NewCache(2 * time.Second)

	var fetchCount int32
	expensiveFetch := func() string {
		atomic.AddInt32(&fetchCount, 1)
		time.Sleep(100 * time.Millisecond) // simulate a slow DB/API call
		return "fresh-value"
	}

	var wg sync.WaitGroup
	results := make([]string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = cache.Get("hot-key", expensiveFetch)
		}(i)
	}
	wg.Wait()

	fmt.Printf("20 concurrent Get() calls completed; actual fetches performed: %d\n", fetchCount)
	fmt.Printf("all results identical: %v (sample: %s)\n", allSame(results), results[0])
}

func allSame(s []string) bool {
	for _, v := range s {
		if v != s[0] {
			return false
		}
	}
	return true
}
```

**Verified output:**
```
20 concurrent Get() calls completed; actual fetches performed: 1
all results identical: true (sample: fresh-value)
```
20 concurrent goroutines missed on the same key; only 1 actual fetch happened.

## Why it matters for the interview
- This is the exact fix for the "thundering herd" / "dogpile" problem, which shows up
  in almost every caching discussion once you mention TTL expiry under load.
- Go's standard library has this exact pattern productionized as
  `golang.org/x/sync/singleflight` — worth naming if asked "would you build this
  yourself," since knowing not to reinvent it is itself a signal of experience.
- Follow-up to expect: "what if the fetch itself fails?" — every waiter should receive
  the same error, not just the fetcher; make sure `call` carries an error field too in
  a production version, not just a value.

# Case Study: Rate Limiter Design

> **This is a very common Agoda-style platform scenario. Practice explaining this end-to-end.**

## Problem Statement
Design a rate limiter for Agoda's API Gateway that:
- Limits each API key to 1000 requests/minute
- Works across multiple gateway instances (distributed)
- Latency overhead < 5ms per request
- Gracefully handles Redis failures

---

## Step 1: Clarify Requirements

**Before designing, ask:**
- Per user or per API key? → Per API key
- Global limit or per-endpoint? → Global for now
- Hard reject or soft throttle? → Reject with 429
- Distributed (multiple gateway nodes)? → Yes
- Acceptable burst? → Allow short bursts up to 1.5x limit
- What if rate limiter itself fails? → Fail open (allow traffic)

---

## Step 2: Estimate Scale

```
10,000 API keys × 1000 req/min = 10M req/min = ~167K req/sec
Each rate limit check: one Redis command (< 1ms)
Redis: can handle 100K+ ops/sec on a single node → need Redis cluster or striped keys
```

---

## Step 3: Algorithm Selection

### Token Bucket (Recommended)
```
Each API key has a bucket with capacity C tokens
New tokens added at rate R per second
Each request consumes 1 token
If empty → reject with 429

Pros: Allows bursts (up to bucket capacity), smooth average rate
Cons: Slightly complex distributed implementation
```

### Fixed Window Counter
```
Count requests in current minute window
If count > limit → reject

Cons: Boundary problem — 999 at 00:59 + 999 at 01:01 = 1998 in 2 seconds
```

### Sliding Window Log
```
Store timestamp of each request in sorted set
Count requests in last 60 seconds
Pros: Very accurate | Cons: High memory usage
```

### Sliding Window Counter (Best trade-off)
```
Approximate sliding window using two fixed windows
current_rate = current_window_count + (previous_window_count × elapsed_fraction)
```

---

## Step 4: Distributed Implementation (Redis + Lua)

Use Redis to store counters — atomic Lua script to avoid race conditions.

```go
package ratelimiter

import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

type RateLimiter struct {
    redis    *redis.Client
    limit    int           // max requests
    window   time.Duration // time window
    luaScript *redis.Script
}

// Atomic sliding window counter using Redis Lua script
const slidingWindowLua = `
local key = KEYS[1]
local prev_key = KEYS[2]
local limit = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local window = tonumber(ARGV[3])
local elapsed_fraction = tonumber(ARGV[4])

local prev_count = tonumber(redis.call('GET', prev_key) or 0)
local curr_count = tonumber(redis.call('GET', key) or 0)

-- Approximate current rate
local rate = curr_count + (prev_count * elapsed_fraction)

if rate >= limit then
    return 0 -- rejected
end

-- Increment current window counter
redis.call('INCR', key)
redis.call('EXPIRE', key, window * 2)
return 1 -- allowed
`

func New(redisClient *redis.Client, limit int, window time.Duration) *RateLimiter {
    return &RateLimiter{
        redis:     redisClient,
        limit:     limit,
        window:    window,
        luaScript: redis.NewScript(slidingWindowLua),
    }
}

func (rl *RateLimiter) Allow(ctx context.Context, apiKey string) (bool, error) {
    now := time.Now()
    windowSecs := int(rl.window.Seconds())

    // Current window: e.g., "ratelimit:key123:2024-01-15T10:30"
    currentWindow := now.Truncate(rl.window).Unix()
    prevWindow := currentWindow - int64(windowSecs)

    currentKey := fmt.Sprintf("ratelimit:%s:%d", apiKey, currentWindow)
    prevKey := fmt.Sprintf("ratelimit:%s:%d", apiKey, prevWindow)

    // Fraction of current window elapsed (for sliding calculation)
    elapsed := float64(now.Unix()-currentWindow) / float64(windowSecs)

    result, err := rl.luaScript.Run(ctx, rl.redis,
        []string{currentKey, prevKey},
        rl.limit,
        now.Unix(),
        windowSecs,
        1.0-elapsed, // how much of previous window still counts
    ).Int()

    if err != nil {
        // Redis failure — fail open (allow traffic)
        return true, fmt.Errorf("rate limiter redis error (failing open): %w", err)
    }
    return result == 1, nil
}

// HTTP Middleware
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        apiKey := r.Header.Get("X-API-Key")
        if apiKey == "" {
            http.Error(w, "missing API key", http.StatusUnauthorized)
            return
        }

        allowed, err := rl.Allow(r.Context(), apiKey)
        if err != nil {
            // Log but allow — fail open
            log.Error("rate limiter error", "err", err)
        }

        if !allowed {
            w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.limit))
            w.Header().Set("Retry-After", "60")
            http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

---

## Step 5: High-Level Architecture

```
Client Request
     │
     ▼
API Gateway (multiple instances)
     │
     ▼ check rate limit
Redis Cluster (sliding window counters)
     │
     ├── Key exists? Increment + check
     └── Key expired? Create new window

If Redis down → fail open → allow traffic
```

---

## Step 6: Trade-offs Discussion

| Concern | Decision | Reasoning |
|---------|----------|-----------|
| Algorithm | Sliding window counter | Good accuracy + reasonable memory vs sliding log |
| Storage | Redis | Sub-ms latency, atomic Lua, built-in TTL |
| Failure mode | Fail open | Better than denying legitimate traffic during Redis outage |
| Burst | Allow up to window size | Token bucket-like behavior at window boundaries |
| Multi-region | Per-region limits | Strong consistency across regions costs too much latency |

---

## Interview Answer Template

> "I'd design a distributed rate limiter using Redis with a sliding window counter algorithm. Each API key maps to a Redis key with the current minute as part of the key name. I use a Lua script for atomic check-and-increment to avoid race conditions. For the sliding window, I keep two counters — current and previous — and approximate the rate as `current_count + (previous_count × (1 - elapsed_fraction))`. This gives smooth rate limiting without the boundary problem of fixed windows. If Redis is unavailable, I fail open to maintain availability — rate limiting is a best-effort concern, not a hard blocker. Response headers include `X-RateLimit-Limit` and `Retry-After` for client guidance."

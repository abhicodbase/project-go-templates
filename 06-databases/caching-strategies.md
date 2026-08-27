# Caching Strategies

## When to Cache

```
Cache: hotel search results, hotel details, user preferences, session data
Don't Cache: real-time availability (critical), payment data, audit logs
```

---

## Redis in Go

```go
package cache

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache miss")

type RedisCache struct {
    client *redis.Client
}

func NewRedisCache(addr string) *RedisCache {
    client := redis.NewClient(&redis.Options{
        Addr:         addr,
        Password:     "",
        DB:           0,
        PoolSize:     20,
        MinIdleConns: 5,
        DialTimeout:  5 * time.Second,
        ReadTimeout:  2 * time.Second,
        WriteTimeout: 2 * time.Second,
    })
    return &RedisCache{client: client}
}

func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
    val, err := c.client.Get(ctx, key).Bytes()
    if err == redis.Nil {
        return ErrCacheMiss
    }
    if err != nil {
        return fmt.Errorf("redis get %s: %w", key, err)
    }
    return json.Unmarshal(val, dest)
}

func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
    data, err := json.Marshal(value)
    if err != nil {
        return fmt.Errorf("marshal value: %w", err)
    }
    return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
    return c.client.Del(ctx, keys...).Err()
}

// SetNX — set only if key doesn't exist (for distributed locks/dedup)
func (c *RedisCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
    data, _ := json.Marshal(value)
    return c.client.SetNX(ctx, key, data, ttl).Result()
}

// Increment — atomic counter (for rate limiting)
func (c *RedisCache) Increment(ctx context.Context, key string) (int64, error) {
    return c.client.Incr(ctx, key).Result()
}
```

---

## Cache Key Design

```go
// Good cache keys — structured, versioned, human-readable
const (
    HotelDetailKey    = "v1:hotel:%s"          // v1:hotel:123
    HotelSearchKey    = "v1:search:%s:%s:%s"   // v1:search:Bangkok:2024-01-15:2024-01-18
    UserSessionKey    = "v1:session:%s"         // v1:session:abc123
    RateLimitKey      = "v1:ratelimit:%s:%d"    // v1:ratelimit:apikey:20240115100
)

func hotelKey(id string) string {
    return fmt.Sprintf(HotelDetailKey, id)
}

func searchKey(city string, checkIn, checkOut time.Time) string {
    return fmt.Sprintf(HotelSearchKey, city, checkIn.Format("2006-01-02"), checkOut.Format("2006-01-02"))
}

// Version in key = easy invalidation: change v1 to v2 when schema changes
```

---

## Cache-Aside with Fallback

```go
type HotelServiceWithCache struct {
    cache *RedisCache
    repo  HotelRepository
}

func (s *HotelServiceWithCache) GetHotel(ctx context.Context, id string) (*Hotel, error) {
    var hotel Hotel
    
    // Try cache
    err := s.cache.Get(ctx, hotelKey(id), &hotel)
    if err == nil {
        return &hotel, nil // Cache hit!
    }
    if !errors.Is(err, ErrCacheMiss) {
        // Redis error — log and fallback to DB (fail open)
        log.Warn("cache error, falling back to DB", "err", err)
    }
    
    // Cache miss or error — fetch from DB
    h, err := s.repo.Get(ctx, id)
    if err != nil {
        return nil, err
    }
    
    // Populate cache (don't fail if cache write fails)
    if err := s.cache.Set(ctx, hotelKey(id), h, 10*time.Minute); err != nil {
        log.Warn("failed to populate cache", "key", hotelKey(id), "err", err)
    }
    
    return h, nil
}

func (s *HotelServiceWithCache) UpdateHotel(ctx context.Context, hotel *Hotel) error {
    if err := s.repo.Update(ctx, hotel); err != nil {
        return err
    }
    // Invalidate cache after successful update
    s.cache.Delete(ctx, hotelKey(hotel.ID))
    return nil
}
```

---

## Redis Data Structures

```go
// String — simple key-value, JSON
client.Set(ctx, "hotel:123", jsonData, 10*time.Minute)

// Hash — structured object fields (avoid full JSON parse for partial reads)
client.HSet(ctx, "hotel:123", map[string]interface{}{
    "name": "Grand Palace",
    "city": "Bangkok",
    "rating": 4.5,
})
client.HGet(ctx, "hotel:123", "rating") // get single field

// Sorted Set — leaderboard, rate limiting with timestamps
client.ZAdd(ctx, "hotel:ranking:Bangkok", redis.Z{Score: 4.8, Member: "hotel:123"})
client.ZRevRangeWithScores(ctx, "hotel:ranking:Bangkok", 0, 9) // top 10

// List — simple queue
client.LPush(ctx, "job-queue", jobJSON)
client.BRPop(ctx, 0, "job-queue") // blocking pop

// Set — unique members (deduplication)
client.SAdd(ctx, "processed-events", eventID)
client.SIsMember(ctx, "processed-events", eventID)
```

---

## Cache Eviction Policies

```
allkeys-lru    → Evict least recently used from ALL keys (good for general cache)
volatile-lru   → Evict least recently used from keys WITH expiry
allkeys-lfu    → Evict least frequently used (better for uneven access patterns)
noeviction     → Return error when memory full (for session stores)
```

---

## Interview Q&A

**Q: How do you handle cache invalidation?**
> A: "There are only two hard things in Computer Science: cache invalidation and naming things." Three approaches: (1) TTL — simplest, accept stale data for TTL duration; (2) Write-through invalidation — on update/delete, immediately invalidate or update the cache key; (3) Event-driven — publish a cache invalidation event via Kafka/Redis Pub-Sub, all instances clear the key. For hotel details (changes infrequently), TTL of 5-10 min is fine. For real-time availability, don't cache or cache with a very short TTL (10-30s) with explicit invalidation on booking.

**Q: What is a cache stampede and how do you prevent it?**
> A: When a popular cache key expires, many requests simultaneously find a miss and all try to populate the cache from the DB simultaneously, creating a spike. Prevention: (1) Mutex per cache key — only one goroutine fetches, others wait; (2) Probabilistic early expiration — start refreshing before expiry with some probability based on remaining TTL; (3) Background refresh — when TTL is running low, asynchronously refresh before expiry, always serve cached value. Redis has a "stale-while-revalidate" pattern that works well here.

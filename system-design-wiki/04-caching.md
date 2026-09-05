# 04 — Caching

## TL;DR
> Cache = store expensive computation results so you don't recompute. 
> Cache-aside is the most common pattern. Redis is the go-to distributed cache.  
> Always define eviction policy (LRU) and TTL. Cache invalidation is the hardest problem.

---

## 1. Why Cache?

```
Without Cache:
Client ──▶ App Server ──▶ Database (10–100ms) ──▶ Response

With Cache:
Client ──▶ App Server ──▶ Cache (0.1–1ms) ──▶ Response   ← 100x faster!
                           (miss) ──▶ Database ──▶ fill cache
```

### What to Cache?
- **Database query results** (most common)
- **Computed values** (aggregations, recommendations)
- **Session data** (user login state)
- **Static assets** (HTML, CSS, JS — CDN)
- **API responses** (downstream service responses)
- **HTML fragments** (rendered page sections)

### Cache Hit Rate
```
Hit Rate = Cache Hits / (Cache Hits + Cache Misses)

Target: > 80% hit rate for meaningful benefit
```

---

## 2. Cache Placement

### Client-Side Cache
- Browser cache (HTTP Cache-Control headers)
- Mobile app local cache
- No server load at all on cache hit

### CDN Cache
- Edge servers cache static/semi-static content
- Geo-distributed, serves users from nearest node

### Application-Level Cache (In-Process)
```
App Server
  ┌──────────────────────┐
  │  In-memory dict/map  │  (fastest, but lost on restart, not shared)
  └──────────────────────┘
```
- Examples: `sync.Map` in Go, `ConcurrentHashMap` in Java
- Fast but **not shared** across multiple app instances

### Distributed Cache
```
App Server 1 ──▶ ┌────────────────┐
App Server 2 ──▶ │  Redis Cluster │
App Server 3 ──▶ └────────────────┘
```
- Shared across all app instances
- Examples: Redis, Memcached
- Slightly slower than in-process (network hop ~0.5ms)

### Database-Level Cache
- MySQL query cache (deprecated in MySQL 8.0)
- PostgreSQL shared_buffers (buffer pool)

---

## 3. Caching Strategies (Write Patterns)

### Cache-Aside (Lazy Loading) — Most Common
```
Read:
1. Check cache → HIT: return
2. Cache MISS → query DB
3. Store in cache → return

Write:
1. Write to DB
2. Invalidate (delete) cache entry
```

```
App ──GET user:123──▶ Cache
                      MISS
App ──SELECT──────▶ Database ──▶ user data
App ──SET user:123──▶ Cache
App ──return data──▶ Client
```

**Pros**: Only caches what's actually needed (lazy)  
**Cons**: Cache miss penalty on first read; potential for stale data if invalidation misses

### Write-Through
```
Write:
1. Write to cache AND DB simultaneously (or cache first, then DB)

Read:
1. Always read from cache (always populated)
```

```
App ──write──▶ Cache ──write──▶ Database
                ↑
            Always fresh
```

**Pros**: Cache always consistent with DB  
**Cons**: Every write hits cache (even data that will never be read again); write latency increases

### Write-Behind (Write-Back)
```
Write:
1. Write to cache immediately (fast!)
2. Asynchronously flush to DB later

Read:
1. Read from cache
```

**Pros**: Very fast writes (don't wait for DB)  
**Cons**: Risk of data loss if cache crashes before DB flush; complexity

### Write-Around
```
Write:
1. Write directly to DB (bypass cache)
2. Cache filled on reads (cache-aside style)
```

**Pros**: Good for infrequently-read data (don't pollute cache)  
**Cons**: Cache miss on first read after write

### Which Strategy When?

| Strategy | Best For |
|----------|---------|
| Cache-aside | Read-heavy, unpredictable access patterns |
| Write-through | Write-heavy but need consistency |
| Write-behind | Write-heavy, can tolerate eventual durability |
| Write-around | Write-once-read-never or infrequent reads |

---

## 4. Cache Eviction Policies

When cache is full, what do you remove?

| Policy | Algorithm | Best For |
|--------|-----------|---------|
| **LRU** (Least Recently Used) | Evict item not accessed longest | General use — most popular |
| **LFU** (Least Frequently Used) | Evict item accessed fewest times | Long-lived cache with clear hotspots |
| **FIFO** (First In, First Out) | Evict oldest item | Simple, not usually optimal |
| **Random** | Evict random item | Simple approximation |
| **TTL** (Time To Live) | Expire after set duration | Time-sensitive data |
| **MRU** (Most Recently Used) | Evict most recent item | Cyclic access patterns |

> **LRU** is the default choice for most caches (Redis default).

### LRU Implementation
```
Uses: HashMap + Doubly Linked List

HashMap: key → node (O(1) lookup)
Doubly Linked List: maintains access order (head = MRU, tail = LRU)

On access: move node to head
On eviction: remove tail node
```

---

## 5. Cache Invalidation

> "There are only two hard things in Computer Science: cache invalidation and naming things." — Phil Karlton

### Strategies

**Time-Based (TTL)**
```
SET user:123 {data} EX 3600   # expire in 1 hour
```
- Simple, predictable
- Data can be stale up to TTL duration

**Event-Based Invalidation**
```
On user update:
  1. DB write
  2. DEL user:123 from cache (or publish invalidation event)
```
- Immediately consistent
- Must not miss any update events

**Cache Tagging**
```
Cache entry tagged with: ["user:123", "account:456"]
On user update: invalidate all entries tagged with "user:123"
```
- Invalidate groups of related entries
- Used in Varnish, Laravel Cache

**Write-Through Invalidation**
- Write to DB and cache simultaneously → cache always fresh

### The Stampede Problem (Cache Thundering Herd)
```
Cache entry expires at T=0:
  ─── 1000 concurrent requests hit cache (MISS!) ───▶ All hit DB simultaneously!
```

**Solutions:**
1. **Mutex/Lock**: first requester fetches, others wait
2. **Probabilistic early expiration**: stochastically refresh before TTL expires
3. **Background refresh**: async task refreshes cache before expiry
4. **Jitter on TTL**: randomize TTL slightly so entries don't all expire at once
   ```
   TTL = base_ttl + random(0, base_ttl * 0.1)
   ```

---

## 6. Redis Deep Dive

Redis (Remote Dictionary Server) is an in-memory data structure store.

### Redis Data Structures

| Structure | Commands | Use Case |
|-----------|---------|---------|
| **String** | GET, SET, INCR, APPEND | Simple key-value, counters |
| **Hash** | HGET, HSET, HGETALL | User profiles, objects |
| **List** | LPUSH, RPOP, LRANGE | Queues, activity feeds |
| **Set** | SADD, SMEMBERS, SINTER | Tags, unique visitors |
| **Sorted Set** | ZADD, ZRANGE, ZRANK | Leaderboards, rate limiting |
| **Bitmap** | SETBIT, GETBIT, BITCOUNT | Feature flags, user tracking |
| **HyperLogLog** | PFADD, PFCOUNT | Approximate unique count |
| **Stream** | XADD, XREAD | Event logs, message queues |

### Common Redis Patterns

**Leaderboard (Sorted Set):**
```redis
ZADD leaderboard 1500 "user:alice"
ZADD leaderboard 2000 "user:bob"
ZADD leaderboard 1800 "user:charlie"

ZREVRANGE leaderboard 0 9 WITHSCORES
# Returns top 10 players with scores
```

**Rate Limiting (Sliding Window):**
```redis
-- Lua script (atomic)
local key = "ratelimit:" .. user_id
local now = redis.call("TIME")[1]
local window = 60  -- 1 minute
local limit = 100

redis.call("ZREMRANGEBYSCORE", key, 0, now - window)
local count = redis.call("ZCARD", key)
if count < limit then
  redis.call("ZADD", key, now, now)
  return 1  -- allowed
else
  return 0  -- rejected
end
```

**Distributed Lock:**
```redis
SET lock:resource123 client-uuid NX EX 30
# NX = only set if not exists (atomic)
# EX 30 = expire in 30s (auto-release on crash)

# Release:
if GET lock:resource123 == client-uuid:
  DEL lock:resource123
```

### Redis Persistence
| Mode | How | Trade-off |
|------|-----|-----------|
| **No persistence** | Pure in-memory | Fastest; data lost on restart |
| **RDB** (Snapshot) | Periodic dump to disk | Fast restart; may lose recent data |
| **AOF** (Append Only File) | Log every write | Most durable; larger files, slower |
| **RDB + AOF** | Both | Best durability + reasonable performance |

### Redis Replication
```
Primary ──▶ Replica 1
        ──▶ Replica 2
        ──▶ Replica 3
```
- Reads can go to replicas (scale reads)
- Writes go to primary only
- **Sentinel**: monitors primary, promotes replica on failure

### Redis Cluster (Sharding)
```
Slot range 0-5460    → Node A (Primary A + Replica A)
Slot range 5461-10922 → Node B (Primary B + Replica B)
Slot range 10923-16383 → Node C (Primary C + Replica C)

Key maps to slot: HASH_SLOT = CRC16(key) % 16384
```
- 16,384 hash slots distributed across nodes
- Enables horizontal scaling of both reads and writes

---

## 7. Memcached vs Redis

| Feature | Redis | Memcached |
|---------|-------|-----------|
| Data structures | Rich (string, hash, list, set, zset) | Simple string only |
| Persistence | Yes (RDB/AOF) | No |
| Replication | Yes (master-replica) | No |
| Clustering | Yes (Redis Cluster) | Yes (client-side) |
| Pub/Sub | Yes | No |
| Lua scripting | Yes | No |
| Memory efficiency | Good | Slightly better (simpler) |
| Multi-threading | Single-threaded (+ I/O threads) | Multi-threaded |

> **Use Redis** for almost everything. **Memcached** only if you need pure multi-threaded key-value with no other features.

---

## 8. Cache-Related Failure Modes

### Cache Penetration
```
Attacker sends requests for keys that NEVER exist:
GET user:999999999 → cache MISS → DB query → not found → cache MISS (again on retry)
Result: DB overwhelmed with useless queries
```

**Solutions:**
- Cache "null" result with short TTL: `SET user:999999999 NULL EX 60`
- **Bloom Filter**: probabilistic check if key could exist before querying DB

### Cache Avalanche
```
Many cache entries expire at the same time:
  T=0: 10,000 keys expire simultaneously
       → 10,000 DB queries hit at once
       → DB overloaded → system down
```

**Solutions:**
- Add random jitter to TTL: `TTL = base + rand(0, base*0.1)`
- Circuit breaker to prevent DB flooding
- Warm up cache before traffic switches over

### Cache Hotspot (Hot Key)
```
One key accessed millions of times/second:
  "trending:post:123" → 1M reads/sec → single Redis node bottleneck
```

**Solutions:**
- Local in-process cache for hot keys (with very short TTL)
- Key replication: `trending:post:123:replica-1`, `:replica-2`, `:replica-N` → distribute reads
- Read-through with local L1 cache in front of Redis (L2)

---

## Key Takeaways

1. **Cache-aside** is the default — only cache what's read
2. **Redis** for distributed cache; in-process map for single-node
3. **LRU** eviction + **TTL** expiry on every cache entry
4. **Invalidate on write**, not read — keeps cache fresh
5. Use **jitter on TTL** to prevent cache avalanche
6. Use **Bloom filter** to prevent cache penetration
7. Local L1 cache in front of Redis for hotspot mitigation
8. Redis **Sorted Sets** for leaderboards; **Atomic Lua** for rate limiting

# 16 — Design Primer Supplements
> **Source**: [system-design-primer](https://github.com/donnemartin/system-design-primer) by Donne Martin  
> These are the high-value topics from the primer **not already covered** in files 01–15. Read this alongside the wiki.

---

## 1. Availability in Parallel vs Sequence

This is a critical formula for any multi-component system.

### In Sequence (chain — one failure kills all)
```
[Service A] → [Service B] → [Service C]

Availability_total = Avail_A × Avail_B × Avail_C
                   = 0.999 × 0.999 × 0.999
                   = 99.7%  ← worse than any single component!
```

### In Parallel (redundancy — all must fail)
```
[Service A] ─┐
[Service B] ─┼─→ (any one serves)
[Service C] ─┘

Availability_total = 1 - (1 - Avail_A) × (1 - Avail_B)
                   = 1 - (0.001 × 0.001)
                   = 99.9999%  ← dramatically better!
```

### Real-World Implication
```
3 sequential services at 99.9% each → overall 99.7%  (that's 26 hours downtime/year!)
2 parallel load-balanced servers at 99.9% → 99.9999% (33 seconds downtime/year!)

→ Design critical paths with parallel redundancy, not sequential dependencies
→ Every added sequential hop reduces availability
```

### Minimizing Sequential Hops
```
❌ API Gateway → Auth → RateLimit → Router → Service → DB (5 sequential hops)
   Availability = 0.999^5 = 99.5%

✅ Collapse cross-cutting concerns into single middleware layer
   API Gateway (Auth + RateLimit built-in) → Service → DB (3 hops)
   Availability = 0.999^3 = 99.7%
```

---

## 2. Database Federation (Functional Partitioning)

Split database by **function/domain** instead of sharding by key.

```
Monolith DB:
  ┌──────────────────────────────┐
  │  users + orders + products   │
  │  + reviews + inventory + ... │
  └──────────────────────────────┘
         (single bottleneck)

Federated DBs:
  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
  │  Users DB    │  │  Orders DB   │  │ Products DB  │
  └──────────────┘  └──────────────┘  └──────────────┘
  (each domain has its own server — no cross-domain contention)
```

### Federation vs Sharding
| Approach | Split By | Good For |
|----------|---------|---------|
| **Federation** | Domain/function | Microservices, different teams |
| **Sharding** | Row key (user_id, etc.) | Same-domain horizontal scale |

### Pros
- Smaller write/read throughput per DB → less replication lag
- Less data per DB → more fits in memory (better cache hit rate)
- Enables independent scaling per domain
- Write to multiple DBs in parallel (no write bottleneck)

### Cons
- Application must determine which DB to query
- Cross-domain JOINs now require application-level merge
- More hardware, more connections to manage

---

## 3. Denormalization

Deliberately introduce redundancy to improve read performance.

```
Normalized (3NF):
  orders: { order_id, user_id, product_id, qty }
  users:  { user_id, name, email }
  products: { product_id, name, price }

  To show order summary: JOIN all 3 tables (expensive at scale)

Denormalized:
  orders: { order_id, user_id, user_name, user_email,  ← copied from users
             product_id, product_name, product_price,  ← copied from products
             qty, total }

  To show order summary: single SELECT on orders (no join)
```

### When to Denormalize
| Scenario | Action |
|----------|--------|
| Read-heavy workload with heavy JOINs | Denormalize |
| Sharded DB (cross-shard JOINs impossible) | Must denormalize |
| Data changes infrequently | Safe to denormalize |
| Data changes frequently | Dangerous — update anomalies |
| CQRS read model | Always denormalize the read side |

### Strategies
1. **Embed related data** (MongoDB sub-documents)
2. **Duplicate columns** (store `user_name` in orders table)
3. **Pre-computed aggregates** (store `comment_count` on posts table, update on every comment)
4. **Materialized views** (DB maintains pre-computed JOIN results)

```sql
-- Materialized view example (PostgreSQL)
CREATE MATERIALIZED VIEW order_summary AS
  SELECT o.id, u.name, p.name, o.qty, (o.qty * p.price) AS total
  FROM orders o
  JOIN users u ON o.user_id = u.id
  JOIN products p ON o.product_id = p.id;

REFRESH MATERIALIZED VIEW order_summary;  -- refresh on schedule
```

### Denormalization Dangers
- **Update anomaly**: update `user_name` in users → must update all orders too
- **Inconsistency risk**: if update is partial (failure mid-way) → stale data
- **Storage cost**: duplicate data uses more disk

---

## 4. SQL Tuning

Key optimizations every engineer should know.

### Index Design Rules
```sql
-- Rule 1: Index columns you filter/sort by
SELECT * FROM orders WHERE user_id = 123 AND status = 'PENDING';
CREATE INDEX idx_orders_user_status ON orders(user_id, status);  -- composite, leftmost first

-- Rule 2: Covering index (include all columns needed by query)
SELECT user_id, status, created_at FROM orders WHERE user_id = 123;
CREATE INDEX idx_covering ON orders(user_id, status, created_at);  -- index-only scan!

-- Rule 3: Avoid functions on indexed columns (breaks index)
❌ WHERE YEAR(created_at) = 2026         -- can't use index on created_at
✅ WHERE created_at BETWEEN '2026-01-01' AND '2026-12-31'  -- range scan on index

-- Rule 4: Avoid leading wildcards (can't use index)
❌ WHERE name LIKE '%smith'    -- full scan
✅ WHERE name LIKE 'smith%'    -- index range scan
```

### CHAR vs VARCHAR
| Type | Storage | Best For |
|------|---------|---------|
| `CHAR(n)` | Fixed n bytes (padded) | Fixed-length: country codes, UUIDs, status |
| `VARCHAR(n)` | Variable (1-2 byte overhead) | Variable-length: names, emails |

> `CHAR` is slightly faster for fixed-length lookups (no length calculation). `VARCHAR` saves space for variable-length.

### Query Execution Plan
```sql
EXPLAIN ANALYZE SELECT * FROM orders WHERE user_id = 123;
-- Look for: "Seq Scan" (bad for large tables) vs "Index Scan" (good)
-- Look for: rows estimates vs actual rows (large gap = stale statistics)

-- Update statistics:
ANALYZE orders;
```

### Common SQL Anti-Patterns
```sql
-- N+1 query (fetch user, then fetch each user's orders separately)
❌ for each user:
     SELECT * FROM orders WHERE user_id = user.id

✅ SELECT u.*, o.* FROM users u JOIN orders o ON u.id = o.user_id WHERE u.id IN (...)

-- SELECT * (fetches unnecessary columns, can't use covering index)
❌ SELECT * FROM users WHERE email = 'alice@example.com'
✅ SELECT id, name FROM users WHERE email = 'alice@example.com'

-- COUNT(*) on large tables without index
❌ SELECT COUNT(*) FROM events    -- full scan
✅ Keep a counter table updated via triggers, or use approximate count (PostgreSQL)
   SELECT reltuples FROM pg_class WHERE relname = 'events';

-- Missing pagination (fetch all rows)
❌ SELECT * FROM posts ORDER BY created_at DESC
✅ SELECT * FROM posts ORDER BY created_at DESC LIMIT 20 OFFSET 0
```

### Partitioning
Split one large table into smaller physical pieces while appearing as one logical table:
```sql
-- Range partitioning by date (PostgreSQL)
CREATE TABLE events (
  id BIGINT, event_date DATE, data JSONB
) PARTITION BY RANGE (event_date);

CREATE TABLE events_2025 PARTITION OF events
  FOR VALUES FROM ('2025-01-01') TO ('2026-01-01');
CREATE TABLE events_2026 PARTITION OF events
  FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');

-- Query on event_date only scans relevant partition (partition pruning)
SELECT * FROM events WHERE event_date = '2026-09-05';  -- only hits events_2026
```

---

## 5. Back Pressure

Mechanism to prevent a fast producer from overwhelming a slow consumer.

```
Without back pressure:
  Producer (10,000 msg/sec) → Queue → Consumer (1,000 msg/sec)
  Queue grows by 9,000 msg/sec → OOM → system crash

With back pressure:
  Consumer signals: "I'm at capacity"
  Producer slows down to match consumer rate
  OR: Producer drops/rejects new work (with 429 response)
```

### Strategies

**Queue Depth Limit (Bounded Queue)**
```
Max queue size = 10,000 messages
When full:
  - Reject new messages (producer gets error → retries with backoff)
  - Drop oldest messages (for real-time data where staleness > loss)
  - Block producer (for batch systems)
```

**Rate Limiting the Producer**
```
Consumer exposes capacity signal:
  GET /capacity → { "capacity": 0.85 }  (85% busy)

Producer reads this and throttles:
  if capacity > 0.8: slow down sends
  if capacity > 0.95: pause entirely
```

**Token Bucket on Ingestion**
```
Each consumer has token bucket.
Producer must acquire token before sending.
Consumer refills tokens as it processes.
```

### Back Pressure in Go (Channel-Based)
```go
// Bounded channel = natural back pressure
jobs := make(chan Job, 1000)  // buffer of 1000

// Producer blocks when channel is full:
go func() {
    for _, job := range allJobs {
        jobs <- job  // blocks when buffer is full
    }
    close(jobs)
}()

// Consumer processes at its own pace:
for job := range jobs {
    process(job)  // slow is fine — producer waits
}
```

### Kafka Back Pressure
```
Consumer lag (offset gap) indicates back pressure needed:
  Lag = latest_offset - consumer_offset

High lag → scale up consumers (add more instances in consumer group)
         → or reduce producer throughput temporarily
```

---

## 6. Refresh-Ahead Caching

Proactively refresh cache before it expires, so users never see a cache miss.

```
Normal TTL expiry:
  T=0:   Cache SET with TTL=60s
  T=60s: Cache EXPIRES → next request sees MISS → DB query → refill (slow!)

Refresh-ahead:
  T=0:   Cache SET with TTL=60s
  T=45s: Background worker detects "expiring soon" → proactively fetches from DB → updates cache
  T=60s: Cache "expires" but was already refreshed at T=45s → still a HIT!
```

### Implementation
```go
// On cache read: check if entry is in "refresh zone"
func GetWithRefreshAhead(key string) (value interface{}, err error) {
    entry := cache.Get(key)
    
    // If entry exists but approaching expiry (< 20% TTL remaining):
    if entry != nil && entry.TTLRemaining() < 0.2 * entry.OriginalTTL {
        go func() {
            // Background refresh (don't block the current request)
            fresh := fetchFromDB(key)
            cache.Set(key, fresh, originalTTL)
        }()
    }
    
    if entry != nil {
        return entry.Value, nil  // return current (possibly stale but very recent) value
    }
    
    // True miss — fetch and set
    value = fetchFromDB(key)
    cache.Set(key, value, originalTTL)
    return value, nil
}
```

### Comparison with Other Strategies
| Strategy | Stale Risk | Miss Penalty | Complexity |
|----------|-----------|-------------|------------|
| Cache-aside (lazy) | Medium (on miss) | High (blocks user) | Low |
| Write-through | Low | Low (always populated) | Medium |
| Refresh-ahead | Very Low | Very Low (pre-warmed) | High |
| Write-behind | Medium | Low | High |

> **Refresh-ahead** is ideal for data that is frequently read and expensive to compute (e.g., home page trending content, popular product listings).

---

## 7. RPC vs REST (Deep Comparison from Primer)

### RPC (Remote Procedure Call) Characteristics
```
Client calls functions, not resources:
  createUser(name, email)       → not POST /users
  getUserById(123)              → not GET /users/123
  transferMoney(from, to, amt)  → not PATCH /accounts/xxx
```

```
// RPC style request
POST /rpc
{
  "method": "createUser",
  "params": { "name": "Alice", "email": "alice@example.com" }
}
```

### RPC Disadvantages
- Tight coupling: client knows server function signatures
- Hard to cache (all POST — no HTTP caching semantics)
- Difficult to evolve API without breaking clients

### REST Disadvantages
- Awkward for complex operations (what's the REST endpoint for "send email to all users who haven't logged in for 30 days"?)
- Over-fetching / under-fetching (solved by GraphQL)
- Stateless design sometimes forces repetitive data in each request

### Practical Rule
```
Internal service-to-service: gRPC (typed, performant)
External / public APIs:      REST (standard, cacheable, browser-friendly)
Client-defined queries:      GraphQL (flexible, avoids over/under-fetching)
Action-based operations:     RPC or REST with action verbs in body
```

---

## 8. Weak Consistency

The third consistency model (between strong and eventual), often overlooked.

```
Consistency spectrum:
  Strong ──────────────── Eventual ──────────── Weak
  (always sees latest)  (converges)      (best-effort, no guarantee)
```

### Weak Consistency
- After a write, reads **may or may not see it** — no promise
- Best-effort approach
- Used when real-time accuracy is irrelevant: VoIP calls, live video, multiplayer games

```
Example: Phone call drops for 3 seconds
  You do NOT hear what was said during those 3 seconds
  The system does NOT replay the missed audio
  → Weak consistency: real-time data that is stale is useless

Example: Live video stream drops frames
  Missed frames are NOT retransmitted
  → Weak consistency: better to skip than to delay

Example: Multiplayer game position updates
  If you miss a position update → use last known position (extrapolate)
  → Weak consistency: low latency > perfect accuracy
```

### When to Use Each
| Model | Use When |
|-------|---------|
| **Strong** | Financial transactions, inventory, auth |
| **Eventual** | Social feeds, shopping carts, DNS |
| **Weak** | Real-time media, gaming, IoT sensor streams |

---

## 9. Powers of Two Table (Estimation Reference)

Essential for back-of-the-envelope calculations.

```
Power   Exact Value    Approx     Name
2^10    1,024          1 thousand  1 KB
2^20    1,048,576      1 million   1 MB
2^30    1,073,741,824  1 billion   1 GB
2^32    4,294,967,296  4 billion   (max 32-bit unsigned int)
2^40    ~1 trillion    1 trillion  1 TB
2^64    ~18 quintillion            (max 64-bit unsigned int)
```

### Quick Estimation Formulas
```
KB = 2^10 ≈ 10^3
MB = 2^20 ≈ 10^6
GB = 2^30 ≈ 10^9
TB = 2^40 ≈ 10^12
PB = 2^50 ≈ 10^15

1 day = 86,400 seconds ≈ 10^5 seconds
1 month = 2.5 × 10^6 seconds
1 year = 3.15 × 10^7 seconds ≈ π × 10^7

1 billion requests/day ÷ 10^5 sec/day = 10,000 RPS
```

### Data Size Quick Reference
```
1 ASCII char = 1 byte
1 UTF-8 char = 1–4 bytes (avg ~2 for most languages)
1 UUID = 16 bytes (128 bits)
1 URL = ~100 bytes
1 tweet (text) = ~280 bytes
1 photo (compressed) = ~300 KB
1 minute HD video (compressed) = ~50 MB
1 minute 4K video = ~375 MB
```

---

## 10. Additional Classic System Designs (from Primer)

### Design Mint.com (Personal Finance Dashboard)

**Requirements**: aggregate transactions from multiple bank accounts, categorize spending, show trends.

**Architecture:**
```
User ──▶ Mint Frontend
           ──▶ Account Service (store bank credentials encrypted)
           ──▶ Transaction Sync Worker (scheduled, runs nightly)
                 ──▶ Bank APIs (scrape/API fetch transactions)
                 ──▶ Categorization Service (ML-based: "Starbucks" → "Coffee")
                 ──▶ Transactions DB (PostgreSQL, partitioned by user_id + date)
           ──▶ Analytics Service (spending trends, budgets)
                 ──▶ Time-series DB or pre-aggregated summary tables
```

**Key Design Points:**
- **Bank credential security**: never store plain passwords; use OAuth2 (Plaid API) or encrypted vault (HSM)
- **Transaction sync**: cron job per user, staggered to avoid thundering herd; use Celery/queue
- **Categorization**: rule-based first (exact merchant match), ML second (fuzzy)
- **Aggregation**: pre-compute monthly/weekly aggregates on each sync (not at query time)

---

### Design Social Graph (Facebook-scale)

**Requirements**: users follow/friend each other; find shortest path between two users; suggest mutual friends.

**Storage:**
```
Option 1: SQL adjacency list
  edges: (from_user_id, to_user_id, created_at)
  Finding all friends: SELECT to_user_id FROM edges WHERE from_user_id = X
  Finding mutual friends: JOIN (expensive at scale)

Option 2: Graph DB (Neo4j)
  (Alice)-[:FRIENDS_WITH]->(Bob)
  MATCH (a)-[:FRIENDS_WITH]->(mutual)-[:FRIENDS_WITH]->(b)
  WHERE a.id = 'alice' AND b.id = 'charlie'
  → Efficient graph traversal

Option 3: Adjacency list in Redis (for hot data)
  SADD friends:alice bob charlie dave
  SINTERSTORE mutual:alice:charlie friends:alice friends:charlie
  → Instant set intersection for mutual friends
```

**BFS for Shortest Path** (6 degrees of separation):
```
Start with user A → BFS level by level
Level 1: A's friends (fetch from cache/DB)
Level 2: friends-of-friends
...stop when B found or max depth (6) reached

Optimization: bidirectional BFS (start from both A and B) → much faster
```

---

### Design Amazon Sales Ranking

**Requirements**: show top-selling products per category, updated frequently.

```
Architecture:
  Order placed → Order Service → Kafka "order.placed" event
  
  Sales Counter Service (Kafka consumer):
    ZINCRBY leaderboard:electronics 1 product_id_123  (Redis Sorted Set)
    ZINCRBY leaderboard:books 1 product_id_456

  Rankings API:
    GET /rankings/electronics?limit=10
    → ZREVRANGE leaderboard:electronics 0 9 WITHSCORES
    → Instant! (Redis sorted set is O(log N + M))
```

**Time-windowed rankings** (last 24 hours, last 7 days):
```
Approach 1: Rolling window with Redis Sorted Set (score = timestamp)
  ZADD sales:electronics timestamp product_id
  ZRANGEBYSCORE sales:electronics (now-86400) now  → last 24h orders
  → Count per product, sort

Approach 2: Pre-aggregate with Kafka Streams
  Tumbling window (1 hour) → count per product per window
  Merge last 24 windows → 24-hour total
  Store results in Redis for fast read
```

---

### Design a System that Scales to Millions on AWS

**Progression (start simple, scale incrementally):**

```
Stage 1 (< 1K users):
  Single EC2 instance + RDS PostgreSQL
  Route 53 → EC2 → RDS

Stage 2 (1K–10K users):
  Add CDN (CloudFront) for static assets
  Add ElastiCache (Redis) in front of RDS
  Route 53 → ALB → EC2 (2 instances, AZ spread)

Stage 3 (10K–100K users):
  Auto Scaling Group (EC2)
  RDS Multi-AZ (automatic failover)
  Separate read replicas for reporting
  SQS for async background jobs

Stage 4 (100K–1M users):
  Aurora (MySQL-compatible, 6-way replication)
  ElastiCache Redis Cluster
  Separate microservices (user, order, catalog)
  S3 + CloudFront for all media

Stage 5 (1M+ users):
  Multi-region active-active (Route53 geolocation)
  Aurora Global Database
  DynamoDB for high-throughput K/V (sessions, carts)
  Kafka (MSK) for event streaming
  Elasticsearch for search
```

---

## 11. Object-Oriented Design Classics

These come up in "low-level design" rounds alongside system design.

### LRU Cache

```go
type Node struct {
    key, val    int
    prev, next *Node
}

type LRUCache struct {
    cap        int
    cache      map[int]*Node
    head, tail *Node  // dummy head (MRU) and tail (LRU)
}

func NewLRU(cap int) *LRUCache {
    h, t := &Node{}, &Node{}
    h.next, t.prev = t, h
    return &LRUCache{cap: cap, cache: make(map[int]*Node), head: h, tail: t}
}

func (l *LRUCache) Get(key int) int {
    if n, ok := l.cache[key]; ok {
        l.remove(n)
        l.insertFront(n)
        return n.val
    }
    return -1
}

func (l *LRUCache) Put(key, val int) {
    if n, ok := l.cache[key]; ok {
        l.remove(n)
    } else if len(l.cache) == l.cap {
        evict := l.tail.prev    // LRU = node before dummy tail
        l.remove(evict)
        delete(l.cache, evict.key)
    }
    n := &Node{key: key, val: val}
    l.cache[key] = n
    l.insertFront(n)
}

func (l *LRUCache) remove(n *Node) {
    n.prev.next, n.next.prev = n.next, n.prev
}

func (l *LRUCache) insertFront(n *Node) {
    n.next, n.prev = l.head.next, l.head
    l.head.next.prev, l.head.next = n, n
}
```

### Parking Lot System
```
Classes:
  ParkingLot: floors, entry/exit gates, fee calculator
  Floor: spots[]
  Spot: type (compact/large/motorcycle), status (free/occupied), vehicle
  Vehicle: plate, type, entryTime
  Ticket: id, vehicle, spot, entryTime, exitTime, fee
  Gate (Entry/Exit): issue ticket, process exit + collect fee

Key methods:
  ParkingLot.findSpot(vehicleType) → Spot
  ParkingLot.park(vehicle) → Ticket
  ParkingLot.exit(ticket) → fee
  FeeCalculator.calculate(entryTime, exitTime, vehicleType) → fee

Storage: Map<ticketId, Ticket>, Map<spotId, Spot>
Thread safety: lock on individual spot (not whole lot)
```

### Design a Chat Server (OO)
```
Classes:
  ChatServer: users, rooms, connections
  User: id, name, connection, rooms[]
  Room: id, members[], messageHistory[]
  Message: id, sender, content, timestamp
  Connection: userId, websocket

Key operations:
  User.sendMessage(room, text)
  Room.broadcast(message)   → iterate members → push via Connection
  ChatServer.createRoom()
  ChatServer.join(user, room)
  ChatServer.leave(user, room)

Concurrency: 
  Room.broadcast() must be thread-safe (sync.RWMutex on members list)
  Message history: bounded circular buffer (last 100 messages)
```

---

## 12. Coverage Gap Summary

Here's what the primer adds vs our AlgoMaster wiki:

| Topic | Primer | Our Wiki | Gap? |
|-------|--------|---------|------|
| Availability math (parallel/series) | ✅ | Partial | **Add** ✅ (this file) |
| Federation | ✅ | ❌ | **Added** ✅ |
| Denormalization | ✅ | Partial | **Added** ✅ |
| SQL Tuning | ✅ | ❌ | **Added** ✅ |
| Back pressure | ✅ | ❌ | **Added** ✅ |
| Refresh-ahead cache | ✅ | ❌ | **Added** ✅ |
| RPC deep dive | ✅ | Partial | **Added** ✅ |
| Weak consistency | ✅ | ❌ | **Added** ✅ |
| Powers of two table | ✅ | Partial | **Added** ✅ |
| Mint.com design | ✅ | ❌ | **Added** ✅ |
| Social Graph design | ✅ | ❌ | **Added** ✅ |
| Sales Ranking design | ✅ | ❌ | **Added** ✅ |
| Scaling on AWS progression | ✅ | ❌ | **Added** ✅ |
| LRU Cache (OO) | ✅ | ❌ | **Added** ✅ |
| Parking Lot (OO) | ✅ | ❌ | **Added** ✅ |
| Chat Server (OO) | ✅ | ❌ | **Added** ✅ |
| CAP theorem | ✅ | ✅ | Covered |
| Caching strategies | ✅ | ✅ | Covered |
| Load balancing | ✅ | ✅ | Covered |
| DNS/CDN | ✅ | ✅ | Covered |
| Sharding | ✅ | ✅ | Covered |
| Message queues | ✅ | ✅ | Covered |
| Microservices | ✅ | ✅ | Covered |

**Nothing critical is missing. The wiki is now comprehensive.**

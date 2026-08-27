# Scalability

## What Is Scalability?
**Scalability** is the system's ability to handle growing load by adding resources.

---

## Vertical vs Horizontal Scaling

| | Vertical (Scale Up) | Horizontal (Scale Out) |
|---|---|---|
| **Approach** | Add more CPU/RAM to one server | Add more servers |
| **Limit** | Hardware ceiling | Virtually unlimited |
| **Cost** | Expensive at high end | Commodity hardware |
| **Failure** | Single point of failure | Resilient (redundancy) |
| **Complexity** | Simple | Needs load balancing, distributed state |
| **Use when** | DB that can't shard easily | Stateless services |

**Interview answer**: *"For Agoda's search service, I'd prefer horizontal scaling since it's stateless and request volume is unpredictable. We scale out web servers and use a shared cache layer (Redis) for session state."*

---

## Load Balancing

### Algorithms
```
Round Robin      → Distribute evenly in sequence
Weighted RR      → More powerful servers get more traffic
Least Connections → Route to server with fewest active connections
IP Hash          → Same client always hits same server (sticky sessions)
Random           → Randomly pick a server
```

### Layer 4 vs Layer 7
- **L4 (TCP/IP)**: Faster, no content inspection — for raw throughput
- **L7 (HTTP)**: Can route by URL path, headers, cookies — for microservices

### Health Checks
```
Active:  LB pings /health endpoint every N seconds
Passive: LB monitors response errors — removes on threshold
```

---

## Statelessness — Key to Horizontal Scaling

A **stateless service** stores no client session data locally.

```go
// ❌ Stateful — can't scale horizontally easily
var sessions = map[string]User{} // in-memory, lost on restart

// ✅ Stateless — state in Redis, any instance can serve
func getUser(ctx context.Context, sessionID string) (User, error) {
    data, err := redisClient.Get(ctx, "session:"+sessionID).Bytes()
    if err != nil {
        return User{}, err
    }
    var user User
    return user, json.Unmarshal(data, &user)
}
```

---

## Database Scaling

### Read Replicas
```
Primary (writes) ──┬──► Replica 1 (reads)
                   ├──► Replica 2 (reads)
                   └──► Replica 3 (reads)
```
- **Replication lag** is the trade-off (eventual consistency for reads)
- **Use for**: read-heavy workloads (Agoda hotel search)

### Connection Pooling
```go
// Database connection pool in Go
db, err := sql.Open("postgres", dsn)
db.SetMaxOpenConns(25)       // Max connections to DB
db.SetMaxIdleConns(25)       // Keep alive connections
db.SetConnMaxLifetime(5 * time.Minute)
```

---

## Caching for Scalability

```
Client → CDN → Load Balancer → App Server → Cache (Redis) → DB
                                              ↑
                                    Hit: < 1ms, no DB call
                                    Miss: fetch from DB, populate cache
```

### Cache Hit Rate Matters
- 90% hit rate → 10% of requests reach DB
- 99% hit rate → 1% of requests reach DB (10x improvement!)

---

## CDN (Content Delivery Network)
- Serve static assets (images, JS, CSS) from edge nodes near users
- For Agoda: hotel photos served from regional CDN nodes
- Reduces latency from 200ms → 10ms for assets

---

## Interview Q&A

**Q: How would you design the Agoda hotel search to handle 1M requests/day?**

> A: First, clarify QPS: 1M/day ≈ ~12 QPS avg, but peak could be 10x = 120 QPS. 
> 1. **Cache** search results in Redis with TTL (most searches are repeated)
> 2. **Read replicas** for hotel inventory DB
> 3. **CDN** for hotel images and static content
> 4. **Horizontal scaling** of search service — stateless, behind load balancer
> 5. **Elasticsearch** for full-text hotel search (better than SQL LIKE queries)
> 6. **Circuit breaker** to external hotel supplier APIs
> Trade-off: Caching means slightly stale data (eventual consistency), acceptable for search.

**Q: When would you NOT use horizontal scaling?**

> A: When you have a monolithic stateful service (like a traditional RDBMS with complex joins), or when the bottleneck is in a shared resource (like a single DB) that can't be easily distributed. In that case, vertical scaling or sharding the DB is more appropriate.

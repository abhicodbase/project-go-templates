# Database Sharding & NoSQL Patterns

## Sharding

**Sharding** = horizontal partitioning of data across multiple DB instances.

```
Without sharding:
  DB Server: all 100M hotels → 500GB, single point

With sharding:
  Shard 0: hotels 0-24M  (A-F cities)
  Shard 1: hotels 25-50M (G-M cities)
  Shard 2: hotels 51-75M (N-S cities)
  Shard 3: hotels 76-100M (T-Z cities)
```

### Sharding Strategies

```
1. Range-based: partition by value range (hotel_id 0-1M → shard 1)
   ✅ Simple, good for range queries
   ❌ Hot spots (new data always to last shard)

2. Hash-based: hash(shard_key) % num_shards
   ✅ Even distribution
   ❌ Range queries require scatter-gather across all shards

3. Geographic: route by user/data location
   ✅ Low latency for region-local queries
   ✅ Data residency compliance
   ❌ Cross-region queries are expensive
```

### What Problems Sharding Creates
- **Cross-shard queries**: JOINs across shards require application-level aggregation
- **Distributed transactions**: Booking that spans shards needs 2PC or Saga
- **Resharding**: Adding a shard requires rebalancing data (expensive)
- **Hotspots**: Popular hotels/cities may overload one shard

---

## NoSQL Patterns

### Redis — When to Use
```
✅ Session storage (fast, TTL built-in)
✅ Caching (sub-millisecond reads)
✅ Rate limiting (atomic INCR)
✅ Pub/Sub messaging
✅ Leaderboards (sorted sets)
✅ Distributed locks (SET NX EX)
❌ Complex queries, JOINs
❌ ACID transactions across multiple keys
❌ Primary store for important data
```

### Cassandra — When to Use
```
✅ Write-heavy workloads (hotel price updates from thousands of suppliers)
✅ Time-series data (booking history, search events)
✅ Multi-region with no single point of failure
✅ Linear horizontal scalability
❌ Complex queries (no JOINs, limited WHERE clauses)
❌ Frequent updates to same record
❌ Strong consistency required

Data model rule: Design tables around query patterns, not data relationships
```

### Elasticsearch — When to Use
```
✅ Full-text search (hotel name, description, amenities)
✅ Faceted search (filter by city, rating, price range, amenities)
✅ Geo-spatial search (hotels within 5km of airport)
✅ Analytics dashboards
❌ Primary write store (not ACID)
❌ Real-time updates (replication lag)

Agoda use: Hotel search index — updated via Kafka events from main DB
```

### MongoDB — When to Use
```
✅ Flexible schema (hotel content varies widely per supplier)
✅ Hierarchical data (hotel → rooms → rates)
✅ Rapid development (schema evolves without migrations)
❌ Complex transactions across multiple documents
❌ Strong consistency across replicas
```

---

## Read Replicas vs Sharding

```
Read Replicas:
  Primary (writes) → async replication → Multiple Replicas (reads)
  
  ✅ Simple to set up
  ✅ Good for read-heavy workloads (Agoda: hotel search is 90% reads)
  ❌ Replication lag (eventual consistency)
  ❌ Write throughput still limited to one primary

Sharding:
  ✅ Scales both reads AND writes
  ✅ No single bottleneck
  ❌ Complex to manage
  ❌ Cross-shard operations expensive

Rule of thumb:
  Start with read replicas (simpler)
  Add sharding only when write throughput or storage is the bottleneck
```

---

## Interview Q&A

**Q: How would you choose between SQL and NoSQL for storing hotel bookings?**
> A: SQL (PostgreSQL) for bookings. Reasons: (1) **ACID** — booking involves inventory decrement + booking record + payment record; these must all succeed or all fail — transactions are essential; (2) **Relational queries** — "show all bookings for user X in the last 30 days with hotel details" is trivial with JOINs; (3) **Consistency** — can't have overbooking due to eventual consistency. I'd use NoSQL complementarily: Redis for caching/rate limiting, Elasticsearch for hotel search, Cassandra for write-heavy hotel availability feeds. "Use the right tool for the right job" rather than defaulting to one.

**Q: What is eventual consistency and when is it acceptable?**
> A: Eventual consistency means replicas will eventually converge to the same state, but there's a window where they might differ. Acceptable for: hotel search results (showing slightly stale prices/availability in search is fine — confirm at booking time), hotel content (descriptions, photos — doesn't change often), analytics dashboards. NOT acceptable for: booking confirmation (can't show "booking confirmed" based on stale data), payment processing, inventory (must not overbook). At Agoda, search can be eventually consistent, but the final booking step must use strong consistency against the primary DB.

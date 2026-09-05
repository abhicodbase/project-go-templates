# 05 — Databases

## TL;DR
> SQL = ACID, structured, great for relational data. NoSQL = scale, flexibility, BASE.  
> Sharding splits data horizontally. Replication copies data for HA/read scale.  
> Index everything you query. Use the right DB for the job.

---

## 1. SQL (Relational Databases)

### Core Concepts
- Data organized in **tables** (rows + columns)
- **Schema-on-write**: structure defined upfront
- Relationships via **foreign keys**
- Query via **SQL** (Structured Query Language)
- ACID compliance

### Popular SQL Databases
| DB | Strengths | Use For |
|----|-----------|---------|
| **PostgreSQL** | Feature-rich, JSON support, ACID | General purpose, complex queries |
| **MySQL** | Widely used, good performance | Web apps, read-heavy workloads |
| **SQLite** | Embedded, zero config | Mobile apps, local dev |
| **CockroachDB** | Distributed SQL, strong consistency | Global ACID transactions |

### ACID Properties (Deep Dive)

**Atomicity** — All or nothing:
```sql
BEGIN;
  UPDATE accounts SET balance = balance - 100 WHERE id = 1;  -- debit
  UPDATE accounts SET balance = balance + 100 WHERE id = 2;  -- credit
COMMIT;  -- both succeed, or both roll back
```

**Consistency** — DB constraints always satisfied:
```sql
ALTER TABLE orders ADD CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES users(id);
-- Can't insert order with user_id that doesn't exist → constraint enforced
```

**Isolation** — Concurrent transactions don't interfere:

| Isolation Level | Dirty Read | Non-Repeatable Read | Phantom Read |
|----------------|-----------|-------------------|-------------|
| READ UNCOMMITTED | Possible | Possible | Possible |
| READ COMMITTED | ✅ Prevented | Possible | Possible |
| REPEATABLE READ | ✅ Prevented | ✅ Prevented | Possible |
| SERIALIZABLE | ✅ Prevented | ✅ Prevented | ✅ Prevented |

**Durability** — Committed data survives crashes:
- Write-Ahead Log (WAL): writes logged to durable storage before applying
- fsync: ensure data written to disk, not just OS buffer

---

## 2. NoSQL Databases

### Why NoSQL?
- Need to scale beyond single-server SQL
- Schema flexibility (evolving data models)
- Specific data access patterns (document, graph, time-series)
- High write throughput

### NoSQL Categories

#### Document Stores
```json
// MongoDB document
{
  "_id": "user_123",
  "name": "Alice",
  "email": "alice@example.com",
  "address": {
    "city": "Mumbai",
    "zip": "400001"
  },
  "tags": ["premium", "early-adopter"]
}
```
- Nested, flexible schema
- Query on any field
- Examples: **MongoDB**, **CouchDB**, **Firestore**
- Use for: user profiles, product catalogs, CMS

#### Key-Value Stores
```
SET session:abc123 → {"user_id": "123", "role": "admin"}
GET session:abc123 → {"user_id": "123", "role": "admin"}
```
- Simplest model, extremely fast
- No query on values (only by key)
- Examples: **Redis**, **DynamoDB** (also supports more), **Riak**
- Use for: sessions, caching, shopping carts

#### Wide-Column Stores (Column Family)
```
Row Key: user_123
  Column Family "profile": { name: "Alice", email: "alice@..." }
  Column Family "activity": { last_login: "...", login_count: 42 }
```
- Rows have variable columns
- Optimized for time-series and event data
- Examples: **Cassandra**, **HBase**, **Bigtable**
- Use for: IoT sensor data, event logs, time-series

#### Graph Databases
```
(Alice)-[:FOLLOWS]->(Bob)
(Bob)-[:LIKES]->(Post#123)
(Post#123)-[:TAGGED]->(Topic:Tech)
```
- Nodes + Edges with properties
- Optimized for traversing relationships
- Examples: **Neo4j**, **Amazon Neptune**, **JanusGraph**
- Use for: social networks, fraud detection, recommendation engines

### SQL vs NoSQL Comparison

| Feature | SQL | NoSQL |
|---------|-----|-------|
| Schema | Fixed, upfront | Flexible, dynamic |
| Scaling | Vertical (+ some horizontal) | Horizontal |
| Consistency | Strong (ACID) | Eventual (BASE) |
| Transactions | Multi-table ACID | Limited (usually per-document) |
| Query language | SQL (powerful) | DB-specific |
| Joins | Efficient | Hard (denormalize instead) |
| Use cases | Financial, ERP, complex relations | Social, IoT, real-time, big data |

---

## 3. Database Indexing

An index is a data structure that makes queries faster at the cost of write overhead and disk space.

### How Indexes Work
```
Without index:
SELECT * FROM users WHERE email = 'alice@example.com'
→ Full table scan: read every row (O(n))

With index on email:
→ B-tree lookup: O(log n) — find position in index, then row
```

### B-Tree Index (Default)
```
                    [50]
                  /      \
           [20, 30]       [70, 90]
          /   |   \      /   |   \
        [10] [25] [35] [60] [80] [95]
```
- Balanced tree structure
- O(log n) for point queries, range queries, ORDER BY
- Default index type in PostgreSQL, MySQL

### Hash Index
```
email → hash → bucket → row
"alice@example.com" → hash(.) → 0x4A2F → row pointer
```
- O(1) lookup for exact matches
- **No range queries** (no ordering)
- Used in-memory or for hash partitioning

### Composite Index
```sql
CREATE INDEX idx_user_status_created ON orders(user_id, status, created_at);

-- Uses index:
SELECT * FROM orders WHERE user_id = 123 AND status = 'PENDING';

-- Doesn't use index:
SELECT * FROM orders WHERE status = 'PENDING';  -- leftmost column missing
```
**Leftmost prefix rule**: a composite index on (A, B, C) supports queries on A, A+B, A+B+C — but NOT B alone, C alone, or B+C.

### Covering Index
```sql
-- Query only needs user_id and email
SELECT email FROM users WHERE user_id = 123;

-- If index covers all needed columns:
CREATE INDEX idx_user_email ON users(user_id, email);
-- → Index-only scan (no table lookup needed!) — extremely fast
```

### Index Trade-offs
| Pros | Cons |
|------|------|
| Fast reads (SELECT) | Slower writes (INSERT/UPDATE/DELETE must update index) |
| Enable efficient sorting | Extra disk space |
| Enable efficient range scans | More memory for index pages |

**Index everything you regularly filter/sort by, but don't over-index write-heavy tables.**

---

## 4. Database Replication

Copying data from one database server (primary) to one or more servers (replicas).

### Primary-Replica (Master-Slave) Replication
```
                ┌─────────────┐
Writes ────────▶│   Primary   │──▶ Binary Log
                └─────────────┘
                      │ replication
          ┌───────────┼───────────┐
          ▼           ▼           ▼
       Replica 1   Replica 2   Replica 3
          │
          └──▶ Reads distributed to replicas
```

**Benefits:**
- Reads scale horizontally (more replicas = more read throughput)
- High availability (failover to replica on primary failure)
- Backup without impacting primary

**Replication Lag:**
- Replicas are **eventually consistent** with primary
- Reads from replica may be slightly stale (milliseconds to seconds)
- Solution: read critical data from primary; use replica for non-critical reads

### Synchronous vs Asynchronous Replication
| Type | How | Trade-off |
|------|-----|-----------|
| **Synchronous** | Write confirmed only after replica acknowledges | No data loss, but slower writes |
| **Asynchronous** | Write confirmed immediately; replica lags | Faster writes, risk of data loss on failover |
| **Semi-sync** | At least one replica must acknowledge | Balance of speed and safety |

### Multi-Primary (Multi-Master) Replication
```
Primary A ◀──▶ Primary B (both accept writes)
    │                │
 Replica A1      Replica B1
```
- Both primaries accept writes
- **Conflict resolution** needed when same row written to both simultaneously
- Complex to implement correctly
- Examples: MySQL Group Replication, Galera Cluster, CockroachDB

---

## 5. Database Sharding (Horizontal Partitioning)

Splitting a large database into smaller pieces (shards) distributed across multiple servers.

```
Without sharding:
  All data → Single DB server → Bottleneck at scale

With sharding:
  user_id 1-10M  → Shard A (Server 1)
  user_id 10M-20M → Shard B (Server 2)
  user_id 20M-30M → Shard C (Server 3)
```

### Sharding Strategies

**Range-Based Sharding**
```
Shard A: user_id 1–1,000,000
Shard B: user_id 1,000,001–2,000,000
```
- Simple to understand
- **Hot spots**: recent data (latest IDs) may hit same shard

**Hash-Based Sharding**
```
shard = hash(user_id) % num_shards
user_id=123: hash(123) % 4 = 1 → Shard 1
user_id=456: hash(456) % 4 = 2 → Shard 2
```
- Even distribution
- **Resharding problem**: adding shard changes all mappings

**Directory-Based Sharding**
```
Lookup Service / Shard Map:
  user_id 123 → Shard C
  user_id 456 → Shard A
```
- Flexible (can move records between shards)
- Lookup service is a bottleneck/SPOF

**Consistent Hashing** (Best for resharding)
```
Hash ring 0─────────────────────────360°
  Shard A at 90°, Shard B at 180°, Shard C at 270°
  Key maps to nearest shard clockwise
  Adding shard D at 135°: only keys between 90°–135° move (A→D)
```
- Minimal data movement when adding/removing shards
- Used by: Amazon DynamoDB, Apache Cassandra, Memcached

### Sharding Challenges
| Challenge | Description | Solution |
|-----------|-------------|---------|
| **Cross-shard joins** | JOIN across shards is expensive | Denormalize, use application-level joins |
| **Distributed transactions** | ACID across shards is hard | Avoid or use 2PC/saga pattern |
| **Hotspot shards** | One shard gets disproportionate traffic | Better key design, virtual nodes |
| **Resharding** | Adding servers requires data migration | Consistent hashing, gradual migration |
| **Operational complexity** | More servers to manage | Use managed DBaaS |

---

## 6. Vertical Partitioning vs Horizontal Partitioning

| Type | What | Example |
|------|------|---------|
| **Vertical Partitioning** | Split by columns (some columns to different tables/DBs) | User table split: auth columns → users_auth, profile columns → users_profile |
| **Horizontal Partitioning (Sharding)** | Split by rows (data distributed by key) | Users with id 1-1M on server A, 1M-2M on server B |

---

## 7. Database Transactions & Isolation

### Transaction Anomalies
**Dirty Read**: read uncommitted data that later gets rolled back
**Non-Repeatable Read**: read same row twice, get different values (another tx updated it)
**Phantom Read**: read a range twice, get different number of rows (another tx inserted)

### Locking
**Pessimistic Locking**: lock the row before reading/writing
```sql
BEGIN;
SELECT * FROM inventory WHERE product_id = 123 FOR UPDATE;  -- lock row
UPDATE inventory SET quantity = quantity - 1 WHERE product_id = 123;
COMMIT;
```

**Optimistic Locking**: no lock; detect conflict at write time using version number
```sql
-- Read with version
SELECT *, version FROM products WHERE id = 123;  -- version = 5

-- Write only if version hasn't changed
UPDATE products SET price = 99, version = 6
WHERE id = 123 AND version = 5;
-- 0 rows updated = conflict, retry
```

---

## 8. NewSQL Databases

NewSQL combines SQL's ACID guarantees with NoSQL's horizontal scalability.

| DB | Description |
|----|-------------|
| **CockroachDB** | Distributed PostgreSQL-compatible, automatic sharding |
| **Google Spanner** | Globally distributed, external consistency, TrueTime |
| **TiDB** | MySQL-compatible, horizontal scale |
| **YugabyteDB** | PostgreSQL-compatible, geo-distribution |

---

## 9. Time-Series Databases

Optimized for data that arrives in time order and is queried by time ranges.

| DB | Use For |
|----|---------|
| **InfluxDB** | Metrics, monitoring, IoT |
| **TimescaleDB** | PostgreSQL extension for time-series |
| **Prometheus** | Metrics collection, alerting |
| **ClickHouse** | Analytics, event data |

**Key optimizations:**
- Columnar storage (compress repeated values)
- Automatic downsampling (aggregate old data)
- Retention policies (auto-delete old data)

---

## 10. Choosing the Right Database

| Use Case | Recommended DB |
|----------|---------------|
| Financial transactions, inventory | PostgreSQL, MySQL |
| User profiles, product catalog | MongoDB, PostgreSQL |
| Session management, caching | Redis |
| Real-time leaderboards, queues | Redis |
| Social graph, fraud detection | Neo4j, Amazon Neptune |
| IoT sensor data, metrics | InfluxDB, TimescaleDB |
| Analytics, reporting | ClickHouse, BigQuery, Redshift |
| Full-text search | Elasticsearch, OpenSearch |
| Event logs, audit trails | Cassandra, DynamoDB |
| Multi-region ACID | CockroachDB, Spanner |

---

## Key Takeaways

1. **SQL** for structured relational data + ACID. **NoSQL** for scale + flexibility
2. **Index** all columns you filter/sort by; avoid over-indexing write-heavy tables
3. **Replication** for HA + read scaling; know about replication lag
4. **Sharding** for write scaling; use **consistent hashing** to minimize resharding pain
5. **Optimistic locking** for low-contention; **pessimistic** for high-contention critical sections
6. **READ COMMITTED** isolation is the safe default for most workloads
7. Match DB to workload — time-series for metrics, graph for relationships, key-value for sessions

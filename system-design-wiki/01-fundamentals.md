# 01 — System Design Fundamentals

## TL;DR
> Scalability = handle more load. Availability = stay up. Reliability = do what it promises.  
> CAP theorem: you can only guarantee 2 of 3 (Consistency, Availability, Partition Tolerance).  
> Design always involves trade-offs — there is no free lunch.

---

## 1. Scalability

Scalability is the ability of a system to handle growing load without degrading performance.

### Vertical Scaling (Scale Up)
Add more power to a single machine (more CPU, RAM, faster disks).

```
Before:        After:
┌──────────┐   ┌──────────────┐
│ 4 CPU    │ → │ 64 CPU       │
│ 16 GB    │   │ 512 GB RAM   │
└──────────┘   └──────────────┘
```

| Pros | Cons |
|------|------|
| Simple, no code changes | Hard upper limit (hardware ceiling) |
| No network overhead | Single point of failure |
| Good for stateful apps | Very expensive at high end |

### Horizontal Scaling (Scale Out)
Add more machines and distribute the load.

```
              ┌────────────┐
Clients ───▶  │ Load       │ ──▶ Server A
              │ Balancer   │ ──▶ Server B
              └────────────┘ ──▶ Server C
```

| Pros | Cons |
|------|------|
| Near-infinite scale | More complex (state management) |
| No single point of failure | Network latency between nodes |
| Cost-effective (commodity hardware) | Distributed bugs are hard to debug |

### When to Scale?
- **CPU-bound**: vertical or horizontal scale compute
- **Memory-bound**: vertical scale or add caching
- **I/O-bound**: horizontal scale + async I/O
- **Network-bound**: CDN, edge caching, reduce payload sizes

---

## 2. Availability

Availability = the percentage of time a system is operational and accessible.

```
Availability = Uptime / (Uptime + Downtime)
```

### The Nines Table

| Availability | Downtime/Year | Downtime/Month | Downtime/Day |
|--------------|---------------|----------------|--------------|
| 99% (2 nines) | 3.65 days | 7.3 hours | 14.4 min |
| 99.9% (3 nines) | 8.76 hours | 43.8 min | 1.44 min |
| 99.99% (4 nines) | 52.6 min | 4.38 min | 8.64 sec |
| 99.999% (5 nines) | 5.26 min | 26.3 sec | 0.864 sec |

### How to Achieve High Availability
1. **Eliminate Single Points of Failure** — redundancy everywhere
2. **Replication** — data and services replicated across nodes/zones
3. **Failover** — automatic switching to backup when primary fails
4. **Health checks** — detect failures quickly
5. **Graceful degradation** — serve partial functionality when some components fail

### Active-Active vs Active-Passive Failover

```
Active-Active:               Active-Passive:
 Server A ◀──▶ Server B       Server A ──▶ Server B (standby)
  (both live traffic)          (B only serves if A dies)
```

- **Active-Active**: Better throughput, but state sync is hard
- **Active-Passive**: Simpler, but wastes resources on standby

---

## 3. Reliability

Reliability = the system does what it is supposed to do, correctly, over time.

- **Fault Tolerance**: system keeps working even when components fail
- **Resilience**: system recovers quickly from failures
- **Redundancy**: backup components ready to take over

### Failure Modes
| Type | Example | Mitigation |
|------|---------|------------|
| Hardware failure | Disk crash | RAID, replication |
| Software bugs | Memory leak | Circuit breakers, restarts |
| Network partition | DC split-brain | Consensus protocols |
| Human error | Wrong config deployed | CI/CD with rollback, feature flags |
| Cascading failures | One service takes down others | Bulkheads, timeouts |

---

## 4. CAP Theorem

> In a distributed system that experiences a **network partition**, you must choose between **Consistency** and **Availability**.

```
         Consistency
             /\
            /  \
           /    \
          / CA   \
         /        \
        ────────────
       /            \
      / CP        AP \
     /________________\
  Partition Tolerance
```

| Property | Meaning |
|----------|---------|
| **C**onsistency | Every read gets the most recent write (or an error) |
| **A**vailability | Every request gets a non-error response (may be stale) |
| **P**artition Tolerance | System works even if network drops messages between nodes |

> **P is always required** in real distributed systems — networks do fail. So the real choice is **CP vs AP**.

### CP Systems (Consistent + Partition Tolerant)
- Return error or timeout if data may be stale
- Examples: **HBase**, **MongoDB** (strong consistency mode), **Zookeeper**, **etcd**
- Use when: banking, inventory management — correctness > availability

### AP Systems (Available + Partition Tolerant)
- Return best-effort / possibly stale data
- Examples: **Cassandra**, **DynamoDB**, **CouchDB**, **DNS**
- Use when: social feeds, shopping carts — availability > perfect consistency

### PACELC Extension
Goes further: even without partition, there's a trade-off between Latency and Consistency.
- High consistency = higher latency (wait for all nodes to agree)
- Low latency = allow stale reads

---

## 5. Consistency Models

From strongest to weakest:

### Strong Consistency
- Every read reflects the latest write
- All nodes see the same data at the same time
- Cost: high latency, reduced availability

### Eventual Consistency
- Given enough time with no new writes, all nodes will converge to the same value
- Reads may return stale data temporarily
- Cost: risk of reading stale/conflicting data

```
Write → Node A ──propagates──▶ Node B (maybe 100ms later)
         │
         └──▶ Node C (maybe 200ms later)

Read from B immediately after write → may return old value
Read from B after propagation → returns new value ✓
```

### Read-Your-Writes Consistency
- A client always sees its own writes, even if others may not yet
- Common in user profile systems

### Causal Consistency
- Operations that are causally related are seen in order
- Unrelated operations may be seen in different order on different nodes

### Monotonic Read Consistency
- If a process reads a value, subsequent reads never return older values

---

## 6. ACID vs BASE

### ACID (Traditional Relational DBs)
| Property | Meaning |
|----------|---------|
| **A**tomicity | All operations succeed or all fail (no partial updates) |
| **C**onsistency | DB goes from one valid state to another |
| **I**solation | Concurrent transactions don't interfere |
| **D**urability | Committed data survives crashes |

### BASE (NoSQL / Distributed Systems)
| Property | Meaning |
|----------|---------|
| **B**asically **A**vailable | System guarantees availability (some staleness OK) |
| **S**oft state | State may change over time (even without input) due to eventual consistency |
| **E**ventually consistent | System will converge to consistent state given time |

```
ACID: Bank transfer — both debit and credit must happen atomically
BASE: Social media likes — eventual count is fine, real-time precision not needed
```

---

## 7. Latency vs Throughput

| Metric | Definition | Unit |
|--------|-----------|------|
| **Latency** | Time for one request to complete | ms, μs |
| **Throughput** | Requests processed per unit time | RPS, QPS |

### Latency Numbers Every Engineer Should Know

| Operation | Latency |
|-----------|---------|
| L1 cache reference | 0.5 ns |
| L2 cache reference | 7 ns |
| Main memory (RAM) access | 100 ns |
| SSD random read | 150 μs |
| HDD random read | 10 ms |
| Network round-trip (same DC) | 0.5 ms |
| Network round-trip (US to EU) | 150 ms |
| Read 1 MB from SSD | 1 ms |
| Read 1 MB from network | 10 ms |

### Little's Law
```
L = λ × W
```
- **L** = average number of items in the system (queue depth)
- **λ** = average arrival rate (RPS)
- **W** = average time an item spends in the system (latency)

> If latency doubles, either queue depth doubles or throughput halves.

### Optimizing Latency
- Caching (reduce DB hits)
- Async processing (don't block the request thread)
- Connection pooling (avoid TCP handshake overhead)
- CDN (serve from edge, close to user)
- Compression (reduce transfer size)

---

## 8. Back-of-the-Envelope Estimation

Essential for system design interviews. Always reason about scale.

### Common Assumptions
| Item | Value |
|------|-------|
| 1 server handles | ~1000 RPS (typical web) |
| 1 MySQL server | ~1000 QPS (complex queries) |
| 1 Redis instance | ~100,000 ops/sec |
| Average tweet size | ~280 bytes text + metadata |
| 1 photo (compressed) | ~300 KB |
| 1 HD video minute | ~50 MB |

### Example: Twitter-scale estimation
```
Daily Active Users: 300M
Tweets per user per day: 2
Total tweets/day: 600M
Tweets/sec (write): 600M / 86400 ≈ 7000 TPS

Reads >> Writes (read-heavy): assume 100:1 read/write ratio
Read QPS: 700,000 QPS

Storage (text only, 5 years):
600M tweets/day × 365 days × 5 years × 280 bytes ≈ ~300 TB
```

---

## 9. SLA, SLO, SLI

| Term | Meaning | Example |
|------|---------|---------|
| **SLA** (Agreement) | Contract with customers about uptime/performance | "99.9% uptime guaranteed" |
| **SLO** (Objective) | Internal target, more strict than SLA | "99.95% uptime internally" |
| **SLI** (Indicator) | The actual metric being measured | "Current uptime = 99.97%" |

> SLO should be **stricter** than SLA — leave an error budget.

### Error Budget
```
Error Budget = 100% - SLO
For 99.9% SLO: Error Budget = 0.1% = 8.76 hours/year

If depleted → freeze new deployments, focus on reliability
```

---

## 10. Key Design Principles

| Principle | Description |
|-----------|-------------|
| **Single Responsibility** | Each service does one thing well |
| **Loose Coupling** | Services communicate via well-defined interfaces |
| **High Cohesion** | Related functionality grouped together |
| **Idempotency** | Repeated operations have the same effect as one |
| **Statelessness** | Servers don't hold session state (enables horizontal scaling) |
| **Defense in Depth** | Multiple layers of protection |
| **Fail Fast** | Detect and surface errors immediately |
| **Design for Failure** | Assume any component can fail at any time |

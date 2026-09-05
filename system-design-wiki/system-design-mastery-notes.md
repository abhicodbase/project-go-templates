# System Design — Complete Reference Notes
*Based on the AlgoMaster.io "System Design Interviews" course structure (19 sections / 113 chapters). Condensed into a single deep-reference doc so every component is covered without needing to click through 113 pages.*

---

## Table of Contents
1. [Introduction](#1-introduction)
2. [Must-Know Topics](#2-must-know-topics)
3. [Concept Deep Dives](#3-concept-deep-dives)
4. [Technology Deep Dives](#4-technology-deep-dives)
5. [Interview Patterns](#5-interview-patterns)
6. [Interview Tips & Frameworks](#6-interview-tips--frameworks)
7. [Interview Question Catalog (45 designs, by category)](#7-interview-question-catalog)

---

## 1. Introduction

### 1.1 What are System Design Interviews?
Open-ended interviews where you architect a large-scale system (e.g., "design Twitter") on a whiteboard in 45–60 min. Unlike coding interviews (one correct answer), the interviewer evaluates **process**: how you gather requirements, make trade-offs, and justify decisions under ambiguity. There's no single "right" design — only defensible ones.

### 1.2 Types of System Design Questions
- **Product design** — design a real product (WhatsApp, Instagram, Uber). Requires functional + non-functional requirement gathering.
- **Infrastructure design** — design a generic building block (rate limiter, cache, message queue, key-value store). More about internal mechanics and CS fundamentals.
- **Object-Oriented / Low-Level Design (LLD)** — design classes/interfaces for a system (parking lot, elevator). Different skill: OOP + design patterns, not distributed systems.
- **Migration/scaling questions** — "how would you scale X from 1K to 100M users" — tests incremental evolution thinking.

### 1.3 Expectations by Level / YoE
| Level | Expectation |
|---|---|
| Junior/Mid (0–4 yrs) | Understand basic building blocks (LB, cache, DB, queue); can design a simple service with guidance; knows CRUD-level tradeoffs. |
| Senior (4–8 yrs) | Drives the interview independently; makes and defends non-trivial tradeoffs (consistency vs availability, SQL vs NoSQL); estimates capacity; identifies bottlenecks. |
| Staff/Principal (8+ yrs) | Thinks in terms of organizational and long-term architecture; discusses failure domains, cost, operability, migration paths; anticipates second-order effects; mentors interviewer-level depth on trade-offs. |

---

## 2. Must-Know Topics

### 2.1 Concepts
Core vocabulary every design must reference:
- **Scalability** — ability to handle growing load. *Vertical* (bigger machine) vs *Horizontal* (more machines).
- **Availability** — % of time system is operational (measured in "nines": 99.9% = ~8.7 hrs downtime/yr).
- **Reliability** — system performs correctly over time without failure.
- **Latency vs Throughput** — latency = time per request; throughput = requests handled per unit time. Often trade off against each other (batching improves throughput, hurts latency).
- **Consistency** — all nodes/reads see the same data at the same time (strong, eventual, causal).
- **Fault tolerance** — system keeps working despite component failures (via redundancy).
- **Load Balancing** — distributing traffic across servers.
- **Caching** — storing frequently accessed data closer to compute to cut latency/load.
- **Partitioning/Sharding** — splitting data across nodes to scale storage/throughput.
- **Replication** — copying data across nodes for durability & read scaling.
- **Idempotency** — repeating an operation produces the same result (critical for retries).
- **Statelessness** — servers don't hold session state locally, so any request can hit any server (enables easy horizontal scaling).

### 2.2 Technologies (landscape overview)
| Category | Examples |
|---|---|
| Relational DB | PostgreSQL, MySQL |
| NoSQL — Document | MongoDB |
| NoSQL — Key-Value | Redis, Memcached, DynamoDB |
| NoSQL — Wide-Column | Cassandra |
| Search | Elasticsearch |
| Messaging/Streaming | Kafka, RabbitMQ, SQS |
| Stream/Batch Processing | Flink, Spark |
| Object Storage | S3 |
| Compute | AWS Lambda |
| Reverse Proxy/LB | Nginx |
| Coordination | Zookeeper |
| Containers/Orchestration | Docker, Kubernetes |
| Observability | Prometheus |

Knowing *what each is for* (not implementation) is enough at interview level — pick the tool that matches the requirement, and justify it.

### 2.3 Tradeoffs (the heart of every interview)
- **Consistency vs Availability** (CAP theorem) — under a network partition, choose CP (reject requests to stay consistent, e.g. banking) or AP (stay up but risk stale reads, e.g. social feeds).
- **Latency vs Consistency** (PACELC extension) — even without partition, stronger consistency costs latency (extra coordination/quorum round-trips).
- **SQL vs NoSQL** — schema rigidity + transactions vs flexible schema + horizontal scalability.
- **Push vs Pull** — server pushes updates (WebSockets, notifications) vs client polls. Push = lower latency, higher server complexity; pull = simple, wasteful.
- **Normalization vs Denormalization** — normalized = less redundancy, more joins; denormalized = faster reads, storage/write overhead.
- **Batch vs Stream processing** — batch = simpler, higher latency, higher throughput; stream = near real-time, more operational complexity.
- **Strong vs Eventual consistency** — correctness guarantees vs scalability/latency.
- **Synchronous vs Asynchronous communication** — sync = simpler, tighter coupling, cascading failures; async (queues) = resilience, complexity, eventual completion.
- **Read-heavy vs Write-heavy optimizations** — caching/read-replicas for reads; sharding/write-buffers/log-structured storage for writes.
- **Cost vs Performance vs Simplicity** — the invisible fourth axis interviewers listen for.

### 2.4 Data Structures used in system design
- **Hash Table** — O(1) lookups; basis of caches, key-value stores, consistent hashing rings.
- **Trie** — prefix search (autocomplete, IP routing).
- **Heap / Priority Queue** — top-K problems, leaderboard, scheduling, load-balancer least-connections.
- **B-Tree / B+Tree** — disk-friendly, used inside relational DB indexes.
- **LSM Tree (Log-Structured Merge Tree)** — optimized for high write throughput (Cassandra, RocksDB, LevelDB).
- **Skip List** — used in Redis sorted sets for range queries with O(log n).
- **Bloom Filter** — probabilistic "definitely not present / maybe present" set membership test — cuts unnecessary DB lookups/cache misses (used in Cassandra, web crawlers, CDNs).
- **HyperLogLog** — approximate cardinality counting (e.g., unique visitors) with tiny memory footprint.
- **Merkle Tree** — hash tree for efficiently verifying/comparing large datasets across replicas (used in Cassandra, DynamoDB, Git, blockchains).
- **Consistent Hashing Ring** — a circular hash space data structure that minimizes re-shuffling when nodes are added/removed.
- **Quad-tree / Geohash** — spatial indexing for location-based queries (Uber, Google Maps).

---

## 3. Concept Deep Dives

### 3.1 Networking
- **OSI/TCP-IP basics**: IP addresses routing packets; TCP = reliable, ordered, connection-oriented (3-way handshake); UDP = fast, no delivery guarantee (used for video/voice/gaming).
- **DNS** — resolves domain → IP; hierarchical (root → TLD → authoritative); cached at multiple layers (browser, OS, resolver) with TTL.
- **HTTP/HTTPS** — request/response protocol; HTTPS adds TLS (encryption + certificate-based authentication).
- **HTTP/1.1 vs HTTP/2 vs HTTP/3** — HTTP/2 adds multiplexing over one TCP connection + header compression; HTTP/3 runs over QUIC (UDP-based) to avoid head-of-line blocking and speed up connection setup.
- **WebSockets** — full-duplex, persistent TCP connection for real-time bidirectional communication (chat, live dashboards) — contrast with request/response polling.
- **Long Polling / Server-Sent Events (SSE)** — cheaper alternatives to WebSockets when only server→client push is needed.
- **Load Balancers** — Layer 4 (TCP/IP level, fast, protocol-agnostic) vs Layer 7 (HTTP-aware, can route by URL/header, do SSL termination). Algorithms: round robin, least connections, IP hash, weighted, consistent hashing.
- **CDN (Content Delivery Network)** — geographically distributed edge caches for static (and increasingly dynamic) content; reduces latency + origin load. Push vs pull CDNs.
- **Reverse Proxy vs Forward Proxy** — reverse proxy (e.g., Nginx) sits in front of servers, hides internal topology, does LB/SSL termination/caching; forward proxy sits in front of clients (used for anonymity, filtering).
- **NAT** — Network Address Translation lets many private IPs share a public IP.

### 3.2 Caching
- **Why**: reduces latency, cuts DB load, absorbs read spikes.
- **Where to cache**: client-side, CDN, load balancer, application layer (in-process), distributed cache (Redis/Memcached), database's own buffer/query cache.
- **Cache-Aside (Lazy Loading)** — app checks cache, on miss reads DB then populates cache. Most common; simple; risk of stale data.
- **Read-Through** — cache library itself loads from DB on a miss transparently.
- **Write-Through** — write goes to cache and DB synchronously → cache always fresh, but write latency higher.
- **Write-Behind (Write-Back)** — write goes to cache first, flushed to DB asynchronously → fast writes, risk of data loss on cache crash.
- **Write-Around** — write goes directly to DB, bypassing cache → avoids cache pollution for write-heavy/rarely-read data.
- **Eviction policies** — LRU (least recently used), LFU (least frequently used), FIFO, TTL-based expiry, random.
- **Cache invalidation** ("one of the two hard problems in CS") — TTL expiry, explicit invalidation on write, versioned keys.
- **Cache stampede / thundering herd** — many requests miss simultaneously (e.g., popular key expires) and hammer the DB. Mitigations: locking/mutex on recompute, request coalescing, staggered TTLs, background refresh.
- **Hot key problem** — single key gets disproportionate traffic (celebrity post). Mitigate with local in-process caching, key replication across cache nodes, or sharding the hot key.

### 3.3 API Design
- **REST** — resource-oriented, uses HTTP verbs (GET/POST/PUT/PATCH/DELETE), stateless, cacheable. Good default for CRUD-style public APIs.
- **GraphQL** — client specifies exact fields needed in one query; avoids over-/under-fetching; single endpoint; more complex server & caching story.
- **gRPC** — binary protocol over HTTP/2 using Protobuf; very fast, strongly typed, great for internal service-to-service calls; supports streaming; poor browser support without a proxy.
- **Webhooks** — server pushes an HTTP callback to a client-registered URL on an event (contrast with client polling).
- **API Gateway** — single entry point handling routing, authentication, rate limiting, request/response transformation, and aggregation for microservices.
- **Versioning strategies** — URI (`/v1/...`), header-based, query param.
- **Pagination** — offset-based (simple, breaks on inserts/deletes) vs cursor/keyset-based (stable, scalable for large datasets).
- **Idempotency keys** — client-supplied unique key so retried POST/PUT requests don't double-apply (critical for payments).
- **Rate limiting at API layer** — token bucket, leaky bucket, fixed window, sliding window (see Interview Patterns section for full algorithm detail).
- **Authentication/Authorization** — API keys, OAuth2, JWT (stateless, self-contained token) vs session cookies (stateful, server-side lookup).

### 3.4 Database Design
- **Relational (SQL)** — strong schema, ACID transactions, joins; scale vertically or via read replicas/sharding. Best when data integrity & complex queries matter (finance, inventory).
- **NoSQL families**:
  - *Document* (MongoDB) — flexible JSON-like schema, good for evolving/nested data.
  - *Key-Value* (Redis, DynamoDB) — simplest model, blazing fast, great for caching/sessions/leaderboards.
  - *Wide-Column* (Cassandra, HBase) — optimized for massive write throughput and time-series-like access patterns.
  - *Graph* (Neo4j) — relationships are first-class; good for social graphs, recommendations, fraud detection.
- **Indexing** — B-Tree indexes speed up equality/range queries at the cost of extra write overhead and storage; composite indexes for multi-column filters; covering indexes to avoid table lookups.
- **ACID** — Atomicity, Consistency, Isolation, Durability — transactional guarantees (traditional RDBMS).
- **BASE** — Basically Available, Soft state, Eventual consistency — the NoSQL alternative philosophy favoring availability/scale over strict consistency.
- **Normalization (1NF–3NF)** vs **denormalization** — normalize to reduce redundancy/anomalies; denormalize (or use materialized views) to optimize read-heavy workloads.
- **Sharding strategies** — range-based, hash-based, geography-based, directory-based (lookup service). Challenges: hot shards, cross-shard joins/transactions, re-sharding.
- **Replication** — leader-follower (single write node, multiple read replicas), multi-leader (writes accepted at multiple nodes, needs conflict resolution), leaderless/quorum-based (Dynamo-style, reads/writes need W+R>N nodes to ack).
- **Database transactions across services** — see Distributed Transactions pattern (Saga, 2PC) below.

### 3.5 Distributed Systems
- **CAP Theorem** — under a network Partition, you can only guarantee Consistency **or** Availability, not both. In practice, partitions are rare but you must pick a default stance (CP vs AP).
- **PACELC** — extends CAP: even without a Partition, you trade Latency vs Consistency during normal operation.
- **Consensus algorithms** — Paxos, Raft — allow a cluster of nodes to agree on a single value/log order despite failures; underpin leader election and distributed logs (etcd, Zookeeper use these).
- **Leader Election** — a coordination pattern so one node becomes authoritative for a task (see Patterns section).
- **Quorum reads/writes** — `W + R > N` guarantees at least one overlapping node between a write and subsequent read set, giving tunable consistency (used by Dynamo, Cassandra).
- **Vector clocks / version vectors** — track causality between events across nodes to detect conflicting concurrent updates (used in Dynamo-style systems).
- **Gossip Protocol** — nodes periodically exchange state with random peers to eventually propagate cluster membership/health info without a central coordinator (Cassandra, DynamoDB use this).
- **Distributed transactions** — Two-Phase Commit (strong but blocking, SPOF risk on coordinator) vs Saga pattern (sequence of local transactions + compensating actions, eventually consistent, no locking).
- **Idempotency & exactly-once vs at-least-once delivery** — true exactly-once is nearly impossible across a network; systems typically do at-least-once delivery + idempotent consumers to *simulate* exactly-once effects.
- **CRDTs (Conflict-free Replicated Data Types)** — data structures that can be updated independently on different replicas and merged automatically without conflicts (used in collaborative editors, distributed counters).
- **Split-brain** — a cluster partitions into two groups, each thinking it's the sole leader — dangerous for consistency; solved with quorum-based leader election.

---

## 4. Technology Deep Dives

| Tech | Type | Core Idea | Best For | Watch-outs |
|---|---|---|---|---|
| **PostgreSQL** | Relational | ACID-compliant, MVCC concurrency, rich SQL & extensions (JSONB, PostGIS) | Complex queries, strong consistency, general-purpose OLTP | Vertical-scale-first; sharding needs external tooling (Citus) |
| **MySQL** | Relational | Simpler, extremely popular, InnoDB engine (row-level locking, ACID) | Web apps, read-replica-heavy scaling | Weaker native JSON/analytics support than Postgres |
| **MongoDB** | Document NoSQL | BSON documents, flexible schema, native sharding | Rapidly evolving schemas, content/catalog data | Joins are awkward; historically weaker multi-doc transactions (now supported but costly) |
| **Redis** | In-memory KV | Single-threaded, rich data types (strings, lists, sets, sorted sets, streams), sub-ms latency | Caching, session store, leaderboards, rate limiting, pub/sub | Data size limited by RAM; persistence (RDB/AOF) trade-offs |
| **Memcached** | In-memory KV | Simple, multi-threaded, pure cache (no persistence, no rich types) | Pure caching at massive scale | No persistence/replication built-in; simpler than Redis |
| **DynamoDB** | Managed KV/Document | Fully managed, single-digit ms latency, auto-scaling, partition-key based | Serverless apps, unpredictable/huge scale | Query flexibility limited by access-pattern-first schema design |
| **Cassandra** | Wide-column | Masterless, ring architecture, tunable consistency, LSM-tree storage | Massive write throughput, multi-datacenter, time-series | No joins/complex queries; data modeling must match query patterns upfront |
| **Elasticsearch** | Search engine | Inverted index, near-real-time full-text search, aggregations | Search, log analytics, autocomplete | Eventually consistent; not a system of record |
| **Kafka** | Distributed log | Append-only partitioned log, high throughput, consumer groups, retention-based (not delete-on-read) | Event streaming, activity logs, decoupling microservices, replay-able pipelines | Operational complexity; ordering only guaranteed per-partition |
| **RabbitMQ** | Message broker | Smart broker, flexible routing (exchanges: direct/topic/fanout), supports complex queuing patterns | Task queues, RPC-style messaging, priority queues | Lower raw throughput than Kafka; not designed for long-term log replay |
| **SQS** | Managed queue | Fully managed, simple, at-least-once delivery, standard (high-throughput) or FIFO queues | Decoupling AWS-based services, simple job queues | No native pub/sub fanout (pair with SNS); message size/visibility-timeout nuances |
| **Flink** | Stream processor | True event-at-a-time streaming, stateful, exactly-once semantics | Low-latency real-time analytics, CEP | Steeper learning curve, stateful ops need careful checkpointing |
| **Spark** | Batch/micro-batch | In-memory distributed compute (RDDs/DataFrames), batch + micro-batch streaming | Large-scale ETL, ML pipelines, batch analytics | Micro-batch = higher latency than Flink for true streaming |
| **S3** | Object storage | Durable (11 9's), infinitely scalable, flat key-namespace with prefixes, versioning/lifecycle rules | Storing blobs/files/backups/data lake | Not a filesystem (eventual consistency historically on overwrites; now strong); latency higher than block storage |
| **AWS Lambda** | Serverless compute | Event-triggered, auto-scales to zero, pay-per-invocation | Bursty/event-driven workloads, glue code | Cold starts, execution time limits, harder to debug/observe |
| **Nginx** | Reverse proxy / LB | Event-driven, non-blocking I/O; handles TLS termination, LB, static file serving | Front door to backend services, LB, API gateway building block | Config complexity at scale; not a full API gateway out of the box |
| **Zookeeper** | Coordination service | Consistent, ordered, hierarchical key store (znodes) using Zab consensus | Leader election, config management, distributed locks, service discovery | Itself a stateful cluster needing quorum (odd number of nodes) |
| **Docker** | Containerization | Packages app + dependencies into an isolated, portable image | Consistent dev/prod environments, microservice packaging | Not orchestration by itself; needs Kubernetes/ECS for scale |
| **Kubernetes** | Orchestration | Declarative desired-state management of containers: scheduling, self-healing, scaling, service discovery | Running microservices at scale | High operational complexity; steep learning curve |
| **Prometheus** | Monitoring | Pull-based metrics collection, time-series DB, PromQL, integrates with Grafana/Alertmanager | Metrics & alerting for infra/services | Not built for long-term storage or high-cardinality logs/traces (pair with Loki/Tempo/Thanos) |

---

## 5. Interview Patterns
*(These are the recurring "sub-problems" that show up inside almost every big design question — mastering these 19 patterns lets you solve most design questions by composition.)*

1. **Realtime Updates** — deliver data to clients as it changes. Options: WebSockets (bidirectional persistent), SSE (server push only), long polling (fallback). Choose based on directionality and infra constraints.
2. **Fanout Pattern** — delivering one event to many recipients (news feed, notifications). *Fanout-on-write* (push to every follower's feed at post time — fast reads, expensive/slow for celebrities) vs *fanout-on-read* (compute feed at read time by pulling from followees — cheap writes, slower reads). Hybrid: fanout-on-write for normal users, fanout-on-read for celebrities.
3. **High Read Traffic** — mitigate via caching (CDN/edge/app cache), read replicas, denormalization, precomputed views, and horizontal scaling of stateless read services.
4. **High Write Traffic** — mitigate via sharding, write buffering/batching, async processing via queues, LSM-tree-based storage (Cassandra), write-optimized data models.
5. **Handling Hot Keys** — a single key/partition receives disproportionate load. Fixes: key salting/splitting (append random suffix and merge results), local caching, replicating the hot key across nodes, request coalescing.
6. **Handling Traffic Spikes** — auto-scaling, load shedding, rate limiting/backpressure, queueing to smooth bursts, pre-warming caches/capacity for known events (e.g., sales, live events).
7. **Handling Large Files** — chunked/multipart upload, resumable uploads, direct-to-object-storage via pre-signed URLs (bypassing app servers), streaming instead of buffering whole file in memory.
8. **Media Streaming** — adaptive bitrate streaming (HLS/DASH: video split into chunks at multiple resolutions, client picks based on bandwidth), CDN edge caching of segments, transcoding pipelines.
9. **Handling Location Data** — geospatial indexing via geohashing or quad-trees to make "find nearby" queries efficient; trade-off between precision and index size; used in Uber/Maps-style designs.
10. **Generating Unique IDs** — UUID (random, no coordination, larger, unordered), auto-increment (simple, single point, doesn't scale across shards), Snowflake-style IDs (timestamp + machine ID + sequence — sortable, distributed, no central coordination).
11. **Distributed Counting** — approximate/exact counters at scale via sharded counters (split counter across N keys, sum on read), or CRDT counters, or async aggregation via a stream processor.
12. **Leader Election** — pick one node to coordinate a task using consensus (Raft/Paxos) or a coordination service (Zookeeper/etcd) so exactly one node acts as leader at a time, with automatic failover.
13. **Failure Detection** — determine if a node is alive: heartbeats (periodic "I'm alive" pings) with timeout thresholds, gossip-based failure detection (Cassandra-style, more scalable than centralized heartbeats), Phi Accrual failure detectors (probabilistic, adaptive thresholds).
14. **Handling Failures** — build resilience via redundancy/replication, retries with exponential backoff + jitter, circuit breakers (stop calling a failing dependency to avoid cascading failure), timeouts, bulkheads (isolate failure domains), graceful degradation.
15. **Recommendations** — collaborative filtering (users who liked X also liked Y), content-based filtering (similar item attributes), hybrid approaches; typically an offline batch job (Spark) computes candidate recommendations, cached/served via a fast KV store.
16. **Multi-Tenancy** — supporting many customers on shared infrastructure. Isolation models: siloed (separate DB per tenant — strong isolation, costly), pooled/shared schema with a `tenant_id` column (cheap, needs careful query scoping), bridge/hybrid (shared infra, isolated schemas).
17. **Multi-Region Architecture** — deploy across geographic regions for latency and disaster recovery. Active-active (all regions serve writes, needs conflict resolution/CRDTs) vs active-passive (one primary region, others standby/failover). Data replication lag and consistency across regions is the core challenge.
18. **Deduplicating Data** — idempotency keys, content hashing (dedupe identical uploads), Bloom filters for fast "have I seen this before" checks, exactly-once processing patterns in stream pipelines.
19. **Distributed Transactions** — **2PC (Two-Phase Commit)**: coordinator asks all participants to "prepare," then commits only if all agree — strongly consistent but blocking and coordinator is a SPOF risk. **Saga pattern**: break a transaction into a sequence of local transactions, each with a compensating action to undo it on failure — no locking, eventually consistent, more resilient at scale (used widely in microservices/payments).
20. **Removing Single Points of Failure** — redundancy at every layer (multiple LB instances via DNS/anycast or floating IP, multi-AZ/region DB replicas, leader election for auto-failover, multiple message broker nodes), so no single component's death takes the system down.

---

## 6. Interview Tips & Frameworks

### 6.1 Answering Framework (structure your 45–60 min)
1. **Clarify requirements (5–10 min)** — functional requirements (what must it do) and non-functional requirements (scale, latency, availability, consistency needs). Ask about scale explicitly — don't assume.
2. **Capacity estimation (Back-of-envelope, ~5 min)** — estimate DAU/QPS, storage growth, bandwidth. Sets the scale the rest of the design must support.
3. **High-level design (10–15 min)** — draw major components (clients, LB, services, DB, cache, queue) and how data flows through them for the core use cases.
4. **Deep dive (15–20 min)** — pick 1–2 components the interviewer cares about most (usually database schema/sharding, or a tricky pattern like fanout/hot-keys) and go deep on trade-offs.
5. **Identify bottlenecks & scale further** — discuss single points of failure, hot spots, and how the design evolves as scale grows 10x/100x.
6. **Wrap-up** — summarize trade-offs made and what you'd revisit with more time/information.

### 6.2 Estimation Cheatsheet
- 1 day ≈ 86,400 s (round to ~100,000 for quick math).
- Daily Active Users → QPS: `QPS ≈ (DAU × actions/user/day) / 86,400`. Multiply by 2–3x for peak traffic.
- 1 KB × 1M ≈ 1 GB. 1M requests/day ≈ ~12 QPS average.
- Read:Write ratio commonly assumed 100:1 for social/content apps unless stated otherwise.
- Storage = data size per record × number of records × replication factor; always project 1–5 years out.
- Common latency numbers (memorize order of magnitude): L1 cache ~1 ns, RAM ~100 ns, SSD random read ~100 µs, network round trip same datacenter ~0.5 ms, cross-region ~100–150 ms, disk seek ~10 ms.

### 6.3 Diagramming Tips
- Draw client → LB → service layer → cache → DB → queue/async workers, left to right, in request flow order.
- Label arrows with protocol (HTTP, gRPC) and rough data (e.g., "write event, ~1KB").
- Box out failure-prone components and mark replication/redundancy explicitly (don't just draw one DB box if you mean a cluster).
- Keep it evolvable — start simple, then annotate what changes as you scale (e.g., "add read replicas here at 10x read traffic").

### 6.4 Choosing the Right Database (quick decision guide)
- Need ACID transactions + complex relational queries → **SQL (Postgres/MySQL)**.
- Need flexible/evolving schema, document-shaped data → **MongoDB**.
- Need sub-millisecond lookups, caching, counters, leaderboards → **Redis**.
- Need massive write throughput, multi-datacenter, tunable consistency → **Cassandra**.
- Need full-text search / log analytics → **Elasticsearch**.
- Need simple, fully-managed, huge-scale key-based access → **DynamoDB**.
- Need to store large blobs/files → **Object storage (S3)**, not a database at all.
- Need relationship-heavy queries (social graph, fraud rings) → **Graph DB (Neo4j)**.

---

## 7. Interview Question Catalog
*(45 classic system-design questions, grouped by category. Each is really just a composition of the concepts/patterns above — use this as a checklist of "which patterns apply" rather than a separate thing to memorize.)*

### Basic Questions
- **URL Shortener** — hashing/base62 encoding for short codes, KV store for mapping, redirect via 301/302, analytics as async pipeline.
- **Pastebin** — similar to URL shortener + object storage for large paste bodies + TTL-based expiry.

### Real-Time Communication
- **WhatsApp** — WebSockets, message queues per user, end-to-end encryption, message ordering & delivery receipts, offline message storage.
- **Slack** — channels/workspaces data model, WebSocket fanout per channel, search indexing (Elasticsearch), presence system.
- **Live Comments** — high fanout pattern, WebSocket/SSE push, rate limiting per user, moderation pipeline.
- **Google Docs** — Operational Transformation or CRDTs for concurrent editing, WebSocket sync, versioning/history.
- **Zoom** — WebRTC for peer/media routing, SFU (Selective Forwarding Unit) architecture, signaling server, adaptive bitrate.

### Social Media Systems
- **Instagram** — fanout patterns, CDN for media, feed ranking, follower graph storage.
- **FB News Feed** — feed ranking pipeline, fanout-on-write/read hybrid, precomputed feed cache.
- **TikTok** — recommendation engine, video transcoding pipeline, CDN delivery, engagement-based ranking.
- **Reddit** — voting/ranking algorithms (hot/best), comment tree storage, sharded post storage.
- **Tinder** — geospatial matching (geohash), swipe-based matching queue, mutual-match detection.

### Media Streaming & Delivery
- **Spotify** — audio streaming via CDN, recommendation pipeline, offline caching on client.
- **YouTube** — video upload/transcoding pipeline, adaptive bitrate streaming, metadata + search, view-count aggregation (distributed counting).
- **Netflix** — content delivery via Open Connect CDN, personalization/recommendation, encoding pipeline for multiple formats.
- **Google Drive** — chunked file upload/sync, file versioning, conflict resolution, metadata service + object storage.
- **Gmail** — email storage/indexing at scale, spam filtering pipeline, search, threading model.
- **Twitch** — low-latency live streaming (RTMP ingest → HLS/LL-HLS delivery), chat fanout at massive scale.

### Location-Based Services
- **Airbnb** — search/filter by location & availability, booking consistency (avoid double-booking), geospatial indexing.
- **Food Delivery Service** — real-time order tracking, matching drivers to orders, ETA computation, geospatial + queueing.
- **Uber** — real-time location tracking, driver-rider matching (geohash-based), surge pricing, trip state machine.
- **Google Maps** — map tile serving (quad-tree/geohash), routing algorithms (Dijkstra/A\* at scale), real-time traffic data ingestion.

### Search & Aggregation Systems
- **Search Autocomplete** — trie-based prefix matching, precomputed top-K suggestions per prefix, caching hot prefixes.
- **News Aggregator** — web crawling/ingestion pipeline, dedup (Bloom filters/hashing), ranking.
- **Web Crawler** — URL frontier (priority queue), politeness/rate limiting per domain, dedup via Bloom filter, distributed crawling.
- **Google Search** — inverted index at massive scale, ranking (PageRank-like), crawling + indexing pipeline, sharded index serving.
- **Ad Click Aggregator** — high-write-throughput event ingestion (Kafka), stream aggregation (Flink), exactly-once/dedup guarantees for billing accuracy.

### E-commerce & Marketplace
- **Amazon** — product catalog search, inventory management (consistency-critical), order processing pipeline, recommendation engine.
- **Shopify** — multi-tenancy architecture, per-store customization, payment integration.
- **Flash Sale** — handling massive traffic spikes on a hot key (single product), inventory consistency under high contention (avoid overselling), queue-based order admission.
- **Online Auction System** — real-time bid updates (WebSockets), strict consistency on current-highest-bid, distributed locking to prevent race conditions.
- **Movie Booking System** — seat-locking to avoid double booking (distributed locks/transactions), high read fanout for popular shows.

### Payment & Financial Systems
- **Payment System** — idempotency keys, distributed transactions (Saga), ledger design (append-only, immutable), reconciliation.
- **Digital Wallet** — strong consistency for balance, double-entry bookkeeping model, fraud detection pipeline.
- **Stock Exchange** — extremely low-latency matching engine, strict ordering guarantees, in-memory order book, strong consistency non-negotiable.

### Distributed Infrastructure (designing the building blocks themselves)
- **Load Balancer** — algorithms (round robin/least conn/consistent hashing), health checks, L4 vs L7.
- **API Gateway** — routing, auth, rate limiting, aggregation for microservices.
- **Rate Limiter** — token bucket / leaky bucket / fixed window / sliding window log or counter — distributed via Redis with atomic operations (Lua scripts) for correctness across nodes.
- **Key-Value Store** — consistent hashing for partitioning, replication with quorum reads/writes, vector clocks for conflict detection (Dynamo-style).
- **Distributed Cache** — sharding cache nodes via consistent hashing, replication for availability, eviction policy, client-side vs server-side sharding logic.
- **CDN** — edge PoPs, cache invalidation propagation, pull vs push origin fetch strategy.
- **Object Storage (like S3)** — durability via erasure coding/replication, metadata service mapping keys to physical location, multipart upload.
- **Messaging Queue** — at-least-once vs exactly-once delivery, partitioning for parallelism, consumer group offset tracking, dead-letter queues.
- **Time Series Database** — write-optimized storage (append-heavy), downsampling/rollups for old data, efficient range queries by time.
- **Locking Service** — distributed locks via Zookeeper/etcd/Redis (Redlock), lease-based locks with TTL to avoid deadlocks on client crash.

### Counting & Ranking Systems
- **Likes Counting System** — sharded counters + async aggregation, eventual consistency acceptable, write-behind to persistent store.
- **Real-Time Leaderboard** — Redis Sorted Sets (skip-list based) for O(log n) rank updates/queries at scale.
- **Top K** — heap-based streaming top-K, or Count-Min Sketch for approximate frequency counting at massive scale with fixed memory.

### Asynchronous Systems
- **Notification Service** — fanout to multiple channels (push/SMS/email), template management, retry/backoff for failed deliveries, priority queues.
- **Job Scheduler** — delayed/cron-based job execution, distributed locking to avoid duplicate execution, priority queue for due jobs.
- **CI/CD Pipeline** — build/test/deploy stage orchestration, artifact storage, parallel job execution, rollback strategy.
- **Monitoring and Alerting System** — metrics ingestion (Prometheus-style pull or push), time-series storage, alerting rules engine, on-call routing.

### Specialized Systems
- **LeetCode** — code execution sandboxing (isolated containers), judge/grading pipeline, leaderboard.
- **Calendar System** — recurring event modeling, timezone handling, conflict/availability checks across users.
- **Online Chess** — real-time move sync (WebSockets), move validation, matchmaking, game-state persistence for reconnection.

---

## How to Use This Doc
- **First pass**: read Sections 2–3 (Must-Know Topics + Concept Deep Dives) fully — this is 80% of interview signal.
- **Second pass**: skim Section 4 (Technologies) as a lookup table — don't memorize, just know *which tool solves which problem*.
- **Third pass**: study Section 5 (Patterns) closely — almost every "Design X" question in Section 7 is just 3–5 of these patterns stitched together.
- **Before an interview**: re-read Section 6 (Framework + Estimation) and skim the relevant Section 7 category for the system you expect to be asked about.

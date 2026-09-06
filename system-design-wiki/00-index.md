# 📚 System Design Wiki — Complete Reference
> Sources: [AlgoMaster.io](https://algomaster.io/learn/system-design) + [system-design-primer](https://github.com/donnemartin/system-design-primer)

A self-contained, in-depth reference covering every topic in the AlgoMaster System Design curriculum, cross-referenced and gap-filled with the system-design-primer.

---

## 📂 Table of Contents

| # | File | Topics Covered |
|---|------|----------------|
| 01 | [Fundamentals](./01-fundamentals.md) | Scalability, Availability, Reliability, CAP theorem, ACID/BASE, Latency vs Throughput |
| 02 | [Networking & Protocols](./02-networking-protocols.md) | HTTP/2/3, TCP/UDP, DNS, CDN, WebSockets, SSE, gRPC |
| 03 | [APIs & Communication](./03-apis-communication.md) | REST, GraphQL, API Gateway, BFF, Rate Limiting |
| 04 | [Caching](./04-caching.md) | Cache strategies, Redis, LRU, Thundering herd, Cache invalidation |
| 05 | [Databases](./05-databases.md) | SQL vs NoSQL, Sharding, Replication, Indexing, ACID, Transactions |
| 06 | [Storage & File Systems](./06-storage-filesystems.md) | Object storage, Block storage, HDFS, S3, Pre-signed URLs |
| 07 | [Message Queues & Streaming](./07-message-queues-streaming.md) | Kafka, RabbitMQ, SQS, Event-driven, Pub/Sub, DLQ |
| 08 | [Distributed Systems](./08-distributed-systems.md) | Consistent hashing, Raft, Leader election, Distributed locks, Quorum |
| 09 | [Load Balancing & Proxies](./09-load-balancing-proxies.md) | L4/L7 LB, Algorithms, Nginx, GSLB, Service mesh |
| 10 | [Microservices & Patterns](./10-microservices-patterns.md) | Circuit breaker, Saga, CQRS, Event sourcing, Bulkhead |
| 11 | [Search & Indexing](./11-search-indexing.md) | Inverted index, Elasticsearch, Typeahead, Fuzzy search, CDC |
| 12 | [Real-time Systems](./12-realtime-systems.md) | Chat, Live feeds, Notifications, Gaming, WebRTC |
| 13 | [Security](./13-security.md) | TLS/mTLS, JWT, OAuth2, OWASP Top 10, Zero-trust |
| 14 | [Observability](./14-observability.md) | Logs, Metrics, Traces, SLO/SLI, Prometheus, Error budget |
| 15 | [System Design Interviews](./15-system-design-interviews.md) | RADIO framework, URL Shortener, Twitter, Netflix, Uber, WhatsApp, Drive, Crawler |
| 16 | [Design Primer Supplements ⭐](./16-design-primer-supplements.md) | Availability math, Federation, Denormalization, SQL tuning, Back pressure, Refresh-ahead, OO designs, Mint/Social Graph/AWS scaling |

---

## 🗺️ How to Use This Wiki

1. **Start with Fundamentals** (01) — everything else builds on it
2. **Read Networking & APIs** (02, 03) — understand how services communicate
3. **Deep-dive Databases & Caching** (04, 05) — the core of every design
4. **Study Distributed Systems** (08) — for senior/staff-level questions
5. **Practice with real designs** (15) — apply everything end-to-end
6. **Read Supplements** (16) — fills all gaps from system-design-primer

---

## 📖 Sources

| Source | Coverage |
|--------|---------|
| [AlgoMaster.io — System Design](https://algomaster.io/learn/system-design) | Core curriculum (files 01–15) |
| [system-design-primer](https://github.com/donnemartin/system-design-primer) | Gap-fill: Federation, Denormalization, SQL Tuning, Back Pressure, Refresh-ahead, OO Design, additional case studies (file 16) |

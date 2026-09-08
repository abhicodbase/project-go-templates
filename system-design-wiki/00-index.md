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
| 17 | [HLD Diagrams & Visual Reference 🖼️](./17-hld-diagrams.md) | Architecture diagrams for all designs: URL Shortener, Twitter, Netflix, Uber, WhatsApp, Web Crawler, Mint, AWS Scaling + deep-dive explanations |

---

### 🎯 Real Interview Questions — Design Solutions

| # | File | Company Context | Core Topics |
|---|------|----------------|-------------|
| 18 | [Hotel Booking & Proximity Search](./18-hotel-booking-proximity.md) | Booking.com | Geohash, Elasticsearch geo, optimistic locking, dynamic pricing |
| 19 | [Nearby Places Recommender](./19-nearby-places-yelp.md) | Yelp/Google Maps/Facebook | Geo search, ranking algorithm, reviews, fake detection |
| 20 | [Log & Media Ingestion System](./20-log-media-ingestion.md) | Any | Multi-source (API+CSV+events), CDC, Kafka, deduplication, Bloom filter |
| 21 | [Concert Ticket Booking (Flash Sale)](./21-concert-ticket-booking.md) | Ticketmaster | Virtual queue, seat hold, oversell prevention, GA vs numbered |
| 22 | [YouTube Design](./22-youtube-design.md) | Google | Pre-signed upload, HLS transcoding, CDN, view counter, ABR |
| 23 | [Music Streaming + Trending Songs](./23-music-streaming-trending.md) | Spotify | Flink/Kafka trending pipeline, Redis sorted set, DRM, Cassandra |
| 24 | [Event Ticket Booking](./24-event-ticket-booking.md) | General | Seat hold, concurrency patterns, idempotency, QR tickets |
| 25 | [Flight Aggregation System](./25-flight-aggregation.md) | MakeMyTrip/Goibibo | Fan-out with timeout, GDS vs direct, circuit breaker, price calendar |
| 26 | [Hotel Tag Management](./26-hotel-tag-management.md) | **Agoda** | CDC, NLP extraction, Redis sorted set, batch UPSERT, 500 reviews/sec |
| 27 | [Reconciliation System](./27-reconciliation-system.md) | **Agoda** | Data diff, discrepancy taxonomy, idempotent runs, auto-resolution |
| 28 | [Hotel Management System](./28-hotel-management-system.md) | Marriott/Hilton | Multi-property, pricing engine, check-in/out, RevPAR, analytics |
| 29 | [Figma / Excalidraw Design Tool](./29-figma-excalidraw.md) | Figma | CRDT vs OT, WebSocket, Canvas vs SVG, 10K concurrent users, Yjs |
| 30 | [Payment Gateway + Exactly-Once](./30-payment-gateway.md) | Any/Fintech | Idempotent consumer, outbox pattern, Kafka exactly-once, PCI-DSS |
| 31 | [Car Dealer Reconciliation](./31-car-dealer-reconciliation.md) | Agoda-style | VIN match key, 4-phase pipeline, auto-resolution rules, tutored |

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

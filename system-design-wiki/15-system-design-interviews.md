# 15 — System Design Interview Playbook

## TL;DR
> Use the RADIO framework: Requirements → API → Data model → Infrastructure → Deep dive.  
> Always clarify scale before designing. Draw the diagram, then add components one by one.  
> Know the top 10 classic designs cold — they appear in 90% of interviews.

---

## 1. Interview Framework (RADIO)

### Step 1: Requirements (5–7 min)
Clarify **what** you're building before drawing anything.

**Functional Requirements** (features):
```
"What exactly does the system need to do?"
- Which specific features? (e.g., for Twitter: post tweet, follow users, see timeline)
- Which client types? (web, mobile, API)
- Any special behaviors? (real-time? offline support?)
```

**Non-Functional Requirements** (scale + quality):
```
- How many daily active users (DAU)?
- Read-heavy or write-heavy?
- Latency requirements? (p99 < 200ms)
- Consistency requirements? (strong or eventual?)
- Storage requirements?
- Availability target? (99.9%?)
- Geographic distribution?
```

### Step 2: API Design (3–5 min)
Define the interface before the implementation.
```
POST /tweets          { content, media_ids }  → tweet_id
GET  /timeline        ?user_id&cursor&limit   → [tweets]
POST /users/{id}/follow                       → 200 OK
GET  /tweets/{id}                             → tweet
```

### Step 3: Data Model (5–7 min)
Define schemas for key entities.
```sql
users:  id, username, email, created_at
tweets: id, user_id, content, created_at
follows: follower_id, followee_id, created_at
```
Choose storage type and justify it.

### Step 4: Infrastructure Design (10–15 min)
Draw the high-level architecture:
- Clients → API Gateway → Services
- Storage choices
- Caching layer
- Message queues

### Step 5: Deep Dive (10 min)
Pick the hardest problems and go deep:
- How does the news feed generation work?
- How do you handle celebrities (hot key problem)?
- How do you scale the write path?

---

## 2. Classic System Designs

---

### Design 1: URL Shortener (e.g., bit.ly)

**Requirements:**
- Given long URL → generate short code (e.g., `bit.ly/xK3m9`)
- Redirect short URL → original URL
- Analytics (click count, referrer, geo)
- Scale: 100M URLs created/day, 10B redirects/day

**Key Design Decisions:**

**Short Code Generation:**
```
Option 1: MD5(longURL) → take first 7 chars
  Problem: collisions possible; same URL maps to same code

Option 2: Base62 encoding of auto-incrementing ID
  ID: 123456789 → base62 → "8M0kX"
  Base62 = [a-z][A-Z][0-9] = 62 chars, 7 chars = 62^7 = 3.5 trillion URLs

Option 3: Pre-generate random codes, store in DB
  Worker pre-generates 10M codes → stores in "available_codes" table
  On create: atomic pop from available_codes table
```

**Architecture:**
```
Client ──POST /shorten──▶ API Server ──▶ DB (url_mappings)
                          API Server ──▶ Cache (short → long)

Client ──GET /xK3m9──▶ API Server ──▶ Cache (hit? → 301 redirect)
                                       Cache miss → DB → cache → 301
```

**Caching:**
- Cache short→long URL mapping in Redis (LRU, TTL = 24h)
- Hit rate will be very high (80/20 rule: 20% of URLs = 80% of traffic)

**Analytics:**
- Log each redirect to Kafka → consumer counts per URL → store in time-series DB
- Don't block redirect for analytics (async)

**DB Schema:**
```sql
url_mappings:
  short_code VARCHAR(8) PRIMARY KEY
  long_url    TEXT
  user_id     BIGINT
  created_at  TIMESTAMP
  expires_at  TIMESTAMP

analytics:
  short_code  VARCHAR(8)
  clicked_at  TIMESTAMP
  referrer    TEXT
  country     VARCHAR(2)
  (partitioned by short_code, clustered by clicked_at)
```

---

### Design 2: Twitter / Social Feed

**Requirements:**
- Post tweets (text, images)
- Follow other users
- See home timeline (tweets from people you follow)
- Real-time updates
- Scale: 300M DAU, 500M tweets/day

**Core Challenge:** Home Timeline Generation

**Fan-out-on-write (Push):**
```
User A (1000 followers) posts:
  For each of A's followers, push tweet_id to their timeline cache

Timeline Cache (Redis):
  user:456:timeline → [tweet_99, tweet_88, tweet_77, ...] (sorted by time)

Read: GET user:456:timeline → instant! (pre-computed)
```

**Fan-out-on-read (Pull):**
```
On read, fetch from all followees' tweet stores and merge.
Too expensive at scale for users following 1000+ people.
```

**Hybrid (Twitter's actual approach):**
- Regular users → fan-out-on-write
- Celebrities (>1M followers) → fan-out-on-read at read time
- Home timeline = pre-computed cache + injected celebrity tweets

**Storage:**
```
tweets:       MySQL (sharded by user_id)
timeline:     Redis List per user (fan-out stored here)
media:        S3 + CloudFront CDN
user_follow:  Cassandra or MySQL (follower_id, followee_id)
```

---

### Design 3: Netflix / Video Streaming

**Requirements:**
- Upload videos (content creators/admin)
- Stream videos (users)
- Multiple quality levels (360p, 720p, 1080p, 4K)
- Global audience, low buffering
- Scale: 200M subscribers, 100M hours watched/day

**Upload Flow:**
```
1. Creator uploads raw video → S3 (raw bucket)
2. S3 event triggers Transcoding Service (AWS Elemental / FFmpeg cluster)
3. Transcoding creates: 360p, 720p, 1080p, 4K variants + thumbnails
4. Output segments (HLS .ts files) stored in S3 (delivery bucket)
5. CDN (CloudFront) pre-warms popular content at edge nodes
```

**Playback Flow:**
```
1. User opens video → App fetches manifest URL from API
2. API → DB: look up video metadata → returns CloudFront CDN URL
3. Player fetches HLS manifest from CDN (m3u8 file listing segments)
4. Player fetches video segments from CDN (nearest edge)
5. Player adapts quality based on bandwidth (ABR - Adaptive Bitrate)
```

**CDN Strategy:**
```
Popular movies: pre-pushed to all edge nodes
Long-tail content: fetched from S3 origin on first request, cached at edge
```

**Recommendation System:**
- Collaborative filtering (users who watched X also watched Y)
- Matrix factorization / deep learning
- Served from pre-computed recommendation store (Redis or DynamoDB)

---

### Design 4: Uber / Ride Sharing

**Requirements:**
- Rider requests ride; driver accepts
- Real-time location tracking of driver
- Match rider with nearest available driver
- Surge pricing during high demand
- Scale: 15M trips/day, drivers update location every 4s

**Location Storage:**
```
Driver sends GPS update every 4 seconds:
  POST /drivers/{id}/location { lat, lng, timestamp }

Storage: Redis Geo (Geospatial sorted set)
  GEOADD drivers lng lat driver_id

Nearby driver search:
  GEORADIUS drivers lat lng 5 km WITHDIST ASC LIMIT 10
  → Returns [driver_A: 0.3km, driver_B: 0.8km, driver_C: 1.2km]
```

**Matching Service:**
```
Rider requests ride at (lat, lng):
  1. Find available drivers in 5km radius (Redis Geo)
  2. Estimate ETA for each (routing engine, Google Maps API)
  3. Send ride request to nearest driver
  4. Driver accepts → assign to rider
  5. If declines → try next driver
```

**Real-time Tracking:**
```
Driver ──location update (every 4s)──▶ Location Service ──▶ Redis Geo
                                       Location Service ──▶ Publish to Kafka

Rider's app:
  WebSocket or Long Polling to Track Service
  Track Service subscribes to driver's Kafka channel
  → Pushes location updates to rider's app
```

**Surge Pricing:**
```
Supply/Demand calculator runs continuously:
  surge_multiplier = demand(area) / supply(area)
  if demand >> supply: surge_multiplier = 1.5–4×
  Cached in Redis per geohash zone
```

---

### Design 5: WhatsApp / Chat App

**Requirements:**
- 1-to-1 messaging
- Group messaging (up to 1000 members)
- Message delivery status (sent, delivered, read)
- Online presence
- End-to-end encryption
- Scale: 2B users, 100B messages/day

**Architecture:**
```
Client A ──WebSocket──▶ Chat Server 1
                         │
                         ├── Redis Pub/Sub (fan-out to server hosting recipient)
                         │
Client B ──WebSocket──▶ Chat Server 2 ──▶ Client B
```

**Message Flow (1-to-1):**
```
1. Client A sends: { to: user_B, text: "Hey!", client_msg_id: "abc" }
2. Chat Server 1 generates msg_id, stores in Cassandra
3. Looks up: user_B connected to Server 2 (via Redis)
4. Publishes to Redis channel "user:B"
5. Server 2 subscribed → delivers to Client B via WebSocket
6. Client B sends ACK → "delivered" status updated
7. Client B reads → "read" status sent to Client A
```

**Offline Delivery:**
```
User B is offline:
  Message stored in Cassandra + flagged as "pending"
  When B reconnects: server fetches all pending messages for B
```

**End-to-End Encryption (Signal Protocol):**
```
Client A and B exchange public keys
Each message encrypted with session key derived from key exchange
Server sees only: { from: A, to: B, ciphertext: "xxxxxxx", timestamp: ... }
→ WhatsApp cannot read messages
```

**Group Messages (up to 1000 members):**
```
User sends to group_id:xyz
Server fetches all 1000 member IDs
For each online member: deliver via WebSocket
For each offline member: queue in Cassandra
```

---

### Design 6: Google Drive / Dropbox

**Requirements:**
- Upload files of any size
- Download/sync files across devices
- File versioning
- Sharing with permissions
- Scale: 1B users, 50TB data uploaded/day

**File Upload (Large Files - Chunked):**
```
Client splits file into 4MB chunks:
  chunk_1 = file[0:4MB]       hash = SHA256(chunk_1)
  chunk_2 = file[4MB:8MB]     hash = SHA256(chunk_2)

1. POST /files/upload/init → upload_session_id
2. For each chunk: PUT /files/chunks/{hash} → S3 with pre-signed URL
3. POST /files/upload/complete { chunk_hashes[] } → server assembles
```

**Delta Sync (Efficient Sync):**
```
Client tracks file state: { filename, chunk_hashes[], last_modified }

On file change:
  Only re-upload modified chunks (not entire file)
  Send list of changed chunk hashes to server
  Server stores only changed chunks → deduplication

File deduplication:
  Content-addressed storage: if chunk_hash already exists in S3, skip upload
```

**File Versioning:**
```
files:
  file_id, name, owner_id, created_at
file_versions:
  version_id, file_id, chunk_hashes[], size, created_at, created_by
chunks:
  hash (PK), s3_key  (deduplicated storage)
```

---

### Design 7: Web Crawler

**Requirements:**
- Crawl billions of web pages
- Respect robots.txt
- Handle duplicate pages
- Politeness (don't overwhelm servers)
- Scale: 1B pages in 30 days = ~400 pages/sec

**Architecture:**
```
┌──────────────┐    URLs to crawl    ┌──────────────┐
│ URL Frontier │◀────────────────────│  URL Manager │
│  (Priority   │                     │  (dedup)     │
│   Queue)     │                     └──────────────┘
└──────────────┘                            ▲
       │                                    │ new URLs extracted
       ▼                                    │
┌──────────────┐  HTML content   ┌──────────────┐
│  Downloader  │────────────────▶│   Parser     │
│  (fetcher)   │                 │  (extract    │
└──────────────┘                 │   links)     │
                                 └──────────────┘
                                        │
                                        ▼
                                 ┌──────────────┐
                                 │  Content     │
                                 │  Storage     │
                                 │  (S3)        │
                                 └──────────────┘
```

**URL Deduplication:**
```
Bloom Filter: probabilistic check (is this URL already crawled?)
  - 99.9% accurate, very memory efficient
  - False positives possible (may skip some new URLs) — acceptable trade-off

Fingerprinting: for near-duplicate content detection (SimHash)
```

**Politeness:**
```
Per-domain queue: only send 1 request per domain per second
Respect: Crawl-delay in robots.txt
Respect: robots.txt disallowed paths
Set: User-Agent: MyBot/1.0 (contact: crawler@company.com)
```

---

### Design 8: Rate Limiter Service

See [03-apis-communication.md](./03-apis-communication.md) for detailed algorithm coverage.

**Key design points:**
```
Architecture:
  Request ──▶ Rate Limiter Middleware ──▶ Service
                       ↕
                    Redis Cluster
                    (sliding window counter per user/IP)

Multi-region:
  Each region has local Redis
  Slight over-counting across regions is acceptable
  (don't synchronize across regions for performance)
```

---

## 3. System Design Cheat Sheet

### When to Use What

| Need | Solution |
|------|---------|
| Scale reads | Read replicas, caching, CDN |
| Scale writes | Sharding, write-ahead log, CQRS |
| Low latency reads | Redis cache (in-memory) |
| Full-text search | Elasticsearch |
| Real-time updates | WebSocket + Redis Pub/Sub |
| Async processing | Kafka, RabbitMQ, SQS |
| Large file upload | S3 + pre-signed URL + chunking |
| Distributed lock | Redis SET NX EX |
| Service discovery | Consul, etcd, Kubernetes DNS |
| Rate limiting | Redis sliding window |
| Global availability | Multi-region active-active |

### Capacity Estimation Quick Reference
```
QPS = DAU × requests_per_user_per_day / 86400

Storage = DAU × avg_data_per_user_per_day × retention_days

Bandwidth = QPS × avg_response_size
```

### Common Back-of-Envelope Numbers
```
1M users     → likely need 1–5 app servers
10M users    → need horizontal scaling + caching
100M users   → multiple data centers, sharding
1B users     → global deployment, custom infrastructure
```

---

## 4. What Interviewers Look For

| Skill | How to Demonstrate |
|-------|--------------------|
| **Requirements gathering** | Ask clarifying questions before designing |
| **Prioritization** | Address core requirements first, extras later |
| **Bottleneck identification** | Know which component is the bottleneck at scale |
| **Trade-off awareness** | "We could do X but that adds complexity, so instead..." |
| **Depth of knowledge** | Know WHY you chose Redis over Memcached |
| **Communication** | Talk through your thinking out loud |
| **Humility** | "I'm not sure about X, but I'd research Y approach" |

---

## Key Takeaways

1. **RADIO framework**: Requirements → API → Data model → Infrastructure → Deep dive
2. Clarify **scale** upfront: DAU, RPS, data volume, read/write ratio
3. **Draw first** (high-level), then justify components, then deep dive bottlenecks
4. Know the **hot key / celebrity problem** and how to solve it (fan-out-on-read)
5. **Justify every choice**: why Redis? why Kafka? why sharding by this key?
6. Practice with a **timer**: 45 minutes is tight — don't over-index on one component
7. Study the 8 classic designs: URL shortener, Twitter, Netflix, Uber, WhatsApp, Drive, Crawler, Rate Limiter

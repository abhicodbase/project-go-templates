# 17 — HLD Diagrams & Visual Reference

> **Visual companion to the wiki.** All architecture diagrams in one place.  
> Sources: [system-design-primer](https://github.com/donnemartin/system-design-primer) screenshots + generated HLD diagrams.

---

## 1. URL Shortener (bit.ly)

![URL Shortener — High Level Design](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/url-shortener-hld.jpg)

### Key Design Decisions Explained

| Decision | Choice | Why |
|----------|--------|-----|
| Short code generation | Base62(auto-increment ID) | No collision, predictable length |
| Read path | Redis → MySQL | 80%+ cache hit rate, sub-ms reads |
| Redirect type | HTTP 301 (permanent) | Browser caches → less server load |
| Analytics | Async via Kafka | Don't block redirect for logging |
| Storage | MySQL for mappings, Cassandra for analytics | ACID for writes, wide-column for time-series |

### Missing Details (from primer)

**301 vs 302 redirect:**
```
301 Moved Permanently → browser caches, future requests skip server (less load, less analytics)
302 Found (Temporary) → browser always asks server (more load, accurate analytics)

→ Use 302 if click tracking is important (bit.ly uses this)
→ Use 301 if reducing server load is more important
```

**Hash collision handling:**
```
MD5 approach: take first 7 chars of MD5(longURL)
If collision detected (that short_code already exists):
  Append a predefined string to longURL and re-hash
  Repeat until no collision

Base62 approach: no collision (based on unique auto-increment ID)
→ Prefer Base62 for simplicity
```

**Custom aliases:**
```
POST /shorten { long_url: "...", custom_alias: "mylink" }
→ Check if custom_alias already taken in DB
→ If taken: return 409 Conflict
→ If free: insert with custom_alias as short_code
```

---

## 2. Twitter / Social Feed

![Twitter Social Feed — High Level Design](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/twitter-feed-hld.jpg)

**System Design Primer Original Diagram:**
![Twitter Architecture from system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/twitter-hld.png)

### Key Design Decisions

| Component | Design Choice | Reason |
|-----------|--------------|--------|
| Timeline storage | Redis List per user | O(1) read, pre-computed |
| Fan-out strategy | Write for regular users, Read for celebrities | Avoid 100M-follower write storm |
| Tweet storage | MySQL sharded by user_id | Relational, manageable with sharding |
| Media | S3 + CloudFront | Binary large objects, CDN delivery |
| Search | Elasticsearch | Inverted index for full-text tweet search |

### Fan-out Deep Dive (Missing from files)
```
Threshold: follower_count > 1,000,000 → "celebrity"

On tweet create:
  if user.followers < 1M:
    for each follower_id:
      LPUSH timeline:{follower_id} {tweet_id}
      LTRIM timeline:{follower_id} 0 999  ← keep only latest 1000 tweets in cache
  else:
    just store tweet in MySQL (no fan-out)

On timeline read:
  tweet_ids = LRANGE timeline:{user_id} 0 19          ← 20 most recent
  celebrity_tweets = merge from celebrity MySQL tables  ← at read time
  combined = merge(tweet_ids, celebrity_tweets)
  sort by timestamp, return top 20
```

### Retweet / Like Implementation
```
Retweet: create new tweet row with retweet_of = original_tweet_id
         fan-out the new tweet_id to followers

Like:    tweets_likes: { tweet_id, user_id }  (compound PK)
         likes_count on tweets table (denormalized, updated via counter)
         
Search:  on tweet create → async → index in Elasticsearch
```

---

## 3. Netflix / Video Streaming

![Netflix Video Streaming — High Level Design](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/netflix-hld.jpg)

### HLS (HTTP Live Streaming) Explained
```
Video uploaded → FFmpeg splits into 6-second segments:
  video_720p_001.ts  (6 sec, 720p)
  video_720p_002.ts  (6 sec, 720p)
  ...

Master playlist (manifest.m3u8):
  #EXTM3U
  #EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=1280x720
  video_720p.m3u8
  #EXT-X-STREAM-INF:BANDWIDTH=400000,RESOLUTION=640x360
  video_360p.m3u8

Per-quality playlist (video_720p.m3u8):
  #EXTM3U
  #EXTINF:6.0,
  video_720p_001.ts
  #EXTINF:6.0,
  video_720p_002.ts
```

**ABR (Adaptive Bitrate) Logic:**
```
Player measures download speed continuously:
  bandwidth > 5 Mbps  → switch to 1080p
  bandwidth 2-5 Mbps  → stay at 720p  
  bandwidth 1-2 Mbps  → drop to 360p
  bandwidth < 1 Mbps  → drop to 240p or buffer

Player pre-buffers 30s ahead → smooth switching
```

### Missing Detail: Video Upload Authentication
```
Large file upload with pre-signed S3 URL:
  1. Client → API: "I want to upload video.mp4 (10GB)"
  2. API → generates pre-signed S3 URL (valid 2 hours)
  3. Client → S3 pre-signed URL → uploads directly (bypasses API servers!)
  4. S3 → triggers Lambda/SQS → Transcoding Service
  5. Transcoding completes → S3 delivers bucket → CDN
  
  Benefits: API servers not bottlenecked by large uploads
            Client writes directly to S3 at full bandwidth
```

---

## 4. Uber / Ride-Sharing

![Uber Ride-Sharing — High Level Design](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/uber-hld.jpg)

### Geohashing vs Redis Geo
```
Geohash: encode lat/lng into a string prefix
  lat=37.7749, lng=-122.4194  →  "9q8yy" (San Francisco)
  Nearby areas share prefix → range scan on geohash prefix

Redis Geo: uses sorted set with geohash score internally
  GEOADD drivers -122.4194 37.7749 "driver_123"
  GEORADIUS drivers -122.4194 37.7749 5 km ASC LIMIT 10
  → Faster than manual geohash for small-scale
  → Redis Geo for < 100M points, custom geohash for larger scale
```

### Surge Pricing Algorithm
```
surge_zone = geohash(request_lat, request_lng, precision=5)  ← 5km grid

demand = count of RIDE_REQUESTED events in zone in last 5 min
supply = count of available drivers in zone (from Redis Geo)

surge_multiplier = demand / supply
  if surge_multiplier < 1.2: multiplier = 1.0x
  if surge_multiplier < 2.0: multiplier = 1.5x
  if surge_multiplier < 3.0: multiplier = 2.0x
  else:                       multiplier = min(surge_multiplier, 4.0x)

Stored in Redis: SET surge:9q8yy 1.5 EX 300  (refresh every 5 min)
```

### Driver Matching State Machine
```
DRIVER STATES:
  OFFLINE → AVAILABLE → ON_TRIP → AVAILABLE

RIDE STATES:
  REQUESTED → DRIVER_ASSIGNED → DRIVER_ARRIVED → IN_PROGRESS → COMPLETED

On ride request:
  1. Find top 5 nearby available drivers
  2. Send push notification to #1 closest driver
  3. Wait 15s for accept
  4. If no accept: skip to #2 driver
  5. If all 5 decline: no cars available (rare)
```

---

## 5. WhatsApp / Chat System

![WhatsApp Chat System — High Level Design](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/whatsapp-hld.jpg)

### Message Ordering & Deduplication
```
Each message has:
  client_msg_id: UUID generated by sender app (idempotency key)
  server_msg_id: server-assigned sequence number per conversation

Client re-sends on timeout:
  Server checks: is client_msg_id already in DB? → return existing (idempotent)
  If not: insert new message

Ordering in conversation:
  Cassandra: CLUSTER BY server_msg_id ASC
  → Messages always retrieved in server-assigned order
```

### Group Chat Fan-out (up to 256 members for WhatsApp)
```
User sends to group "family_chat" (50 members):
  1. Server stores message once in group_messages table
  2. Server fetches all 50 member_ids
  3. For each online member: push via WebSocket
  4. For each offline member: mark as pending in member_delivery_status
     { group_id, message_id, member_id, status: PENDING }

Member comes online:
  Fetch all PENDING messages for member → push in order → mark DELIVERED
```

### Read Receipts (✓✓ mechanism)
```
Single tick ✓ = sent to server (server ACK)
Double tick ✓✓ = delivered to recipient device (recipient ACK)  
Blue ✓✓ = read by recipient (recipient send READ event)

Implementation:
  Server → Client B (deliver) → Client B sends {message_id, event: "DELIVERED"} to server
  Server stores delivery_status → notifies Client A via WebSocket
  
  Client B opens chat → sends {message_id, event: "READ"} to server
  Server → notifies Client A → turns ticks blue
```

---

## 6. Caching Strategies — Visual Reference

![Caching Strategies Visual Reference](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/caching-hld.jpg)

### Cache Eviction Policy Deep Dive

| Policy | Algorithm | Best For | Problem |
|--------|-----------|---------|---------|
| **LRU** | Doubly linked list + HashMap | General use — removes least recently used | Not great for "scan" workloads (one-time large reads evict hot data) |
| **LFU** | Min-heap by frequency count | Workloads with stable hot data | Cold start problem (new popular items start at frequency=1) |
| **FIFO** | Simple queue | Simple, predictable | Ignores access patterns |
| **TTL** | Sorted set by expiry time | Time-sensitive data | Items evicted even if still popular |
| **ARC** | Adaptive (LRU + LFU combined) | Mixed workloads | More complex |

### Cache Stampede Prevention
```
Problem: Key expires → 10,000 simultaneous cache misses → DB overloaded

Solution 1: Mutex (Redis SETNX lock)
  if SETNX cache_lock:product_123 1 EX 5:
      # We won the lock — fetch from DB and fill cache
      data = DB.fetch(product_123)
      cache.set(product_123, data)
      cache.delete(cache_lock:product_123)
  else:
      # Another worker is filling it — wait briefly and retry
      time.sleep(50ms); return GetCached(product_123)

Solution 2: Probabilistic Early Expiration
  Read cache entry → if TTL < random()*TTL_threshold:
      background_refresh()  # fetch fresh before others see expiry
```

---

## 7. Distributed Systems — Core Concepts

![Distributed Systems Core Concepts](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/distributed-systems-hld.jpg)

### Consistent Hashing — Adding/Removing Nodes
```
Ring with 3 servers (A, B, C) each with 3 virtual nodes = 9 points on ring

Before: 
  Keys 0-120 → Server A
  Keys 121-240 → Server B  
  Keys 241-360 → Server C

Add Server D (gets position 60 on ring):
  Only keys 0-60 move from A to D  (33% of A's keys)
  B and C unaffected

Remove Server B (failure):
  Only B's keys (121-240) move to C (next clockwise)
  A and D unaffected

→ Without consistent hashing (simple modulo):
  Adding server D changes N from 3 to 4
  hash(key) % 3 → hash(key) % 4 → EVERYTHING remaps!
```

### Raft — Leader Election Timing
```
Election timeout: each follower waits random 150-300ms before starting election
  → Randomness prevents split votes (two followers start election simultaneously)

Term number: monotonically increasing election counter
  If a node sees a higher term → immediately becomes follower

Leader heartbeat: every 50ms (must be << election timeout)
  Followers that receive heartbeat reset their election timer
  → No election while leader is healthy
```

---

## 8. System Design Primer — Official Architecture Diagrams

These are the canonical HLD diagrams from the [system-design-primer](https://github.com/donnemartin/system-design-primer):

### Pastebin / URL Shortener (Primer Official)
![Pastebin HLD — system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/pastebin-hld.png)

### Twitter Timeline & Search (Primer Official)
![Twitter HLD — system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/twitter-hld.png)

### Web Crawler (Primer Official)
![Web Crawler HLD — system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/webcrawler-hld.png)

### Mint.com (Primer Official)
![Mint.com HLD — system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/mint-hld.png)

### Social Graph (Primer Official)
![Social Graph HLD — system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/social-graph-hld.png)

### Amazon Sales Ranking (Primer Official)
![Sales Ranking HLD — system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/sales-rank-hld.png)

### Scaling on AWS (Primer Official)
![AWS Scaling HLD — system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/aws-scaling-hld.png)

---

## 9. Web Crawler — Additional Details

![Web Crawler HLD — system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/webcrawler-hld.png)

### Prioritized URL Frontier
```
Not all URLs are equal. Prioritization factors:
  1. PageRank / domain authority (high-authority domains first)
  2. Update frequency (news sites → crawl every hour; static pages → once/month)
  3. Business priority (your own domain > partner domains > rest of web)

Implementation: multiple priority queues
  HIGH priority queue:  news.ycombinator.com, github.com (crawl every 1h)
  MED priority queue:   blogs, forums (crawl every 24h)
  LOW priority queue:   everything else (crawl every 30 days)

Queue selector: weighted random selection (70% high, 20% med, 10% low)
```

### Duplicate Detection
```
URL deduplication (same URL crawled twice):
  Bloom Filter: 1 bit per URL hash (very compact)
  False positive rate: 0.1% at 1 billion URLs = ~1.25 GB memory
  → Accept: 0.1% of new URLs incorrectly skipped

Content deduplication (same content, different URL):
  SimHash: locality-sensitive hash of page content
  If SimHash(page_A) ≈ SimHash(page_B) → near-duplicate → skip
  
  SimHash algorithm:
    1. Extract tokens (words) from page
    2. Hash each token to N-bit fingerprint
    3. Weighted sum of fingerprints
    4. Result: N-bit SimHash (similar pages → similar hash)
    5. Hamming distance < threshold → duplicate
```

### robots.txt Compliance
```
// Fetch and parse robots.txt before crawling any URL on a domain
GET https://example.com/robots.txt

User-agent: *          ← applies to all bots
Disallow: /admin/      ← don't crawl /admin/*
Disallow: /private/    ← don't crawl /private/*
Allow: /public/        ← explicitly allowed
Crawl-delay: 2         ← wait 2 seconds between requests

// Cache robots.txt per domain with TTL = 24h
// If robots.txt is missing → treat as "allow all"
```

---

## 10. Mint.com — Additional Details

![Mint.com HLD — system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/mint-hld.png)

### Transaction Sync Architecture
```
For each user with connected bank accounts:
  Cron job (staggered): trigger every night at different offsets to avoid thundering herd
    user_id % 24 = hour_offset  → user 123 syncs at 3AM, user 456 at 7AM

Sync Worker:
  1. Fetch OAuth token for bank (stored encrypted in secrets vault)
  2. Call bank API (Plaid/Yodlee): GET /transactions?since=last_sync_timestamp
  3. For each new transaction:
     a. Store in transactions table
     b. Send to Categorization Service (async via Kafka)
  4. Update last_sync_timestamp

Categorization Service (Kafka consumer):
  Receives: { merchant_name: "STARBUCKS #1234", amount: 5.75 }
  Step 1: Exact match: "STARBUCKS" → "Food & Drink: Coffee"  (lookup table)
  Step 2: If no match: ML classifier (trained on labeled transactions)
  Step 3: Store category → update transaction record
```

### Budget Alert System
```
User sets budget: { category: "Dining", monthly_limit: $300 }

After each transaction categorized:
  spent_this_month = SUM(amount) WHERE category='Dining' AND month=current
  
  if spent_this_month >= monthly_limit * 0.8:
      send notification: "You've used 80% of your Dining budget"
  if spent_this_month >= monthly_limit:
      send notification: "You've exceeded your Dining budget by $X"

Budget evaluation: triggered by Kafka "transaction.categorized" events
```

---

## 11. AWS Scaling — Step-by-Step

![AWS Scaling — system-design-primer](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/aws-scaling-hld.png)

### Detailed Progression

```
Stage 1: Single box (< 1K users)
  Route53 → EC2 (t3.medium) → RDS MySQL (db.t3.micro)
  Cost: ~$50/month
  Failure point: single EC2 = SPOF

Stage 2: Separate web & DB (1K–10K users)
  Route53 → EC2 Web (2 instances) → RDS MySQL (Multi-AZ) 
  + ElastiCache Redis (cache queries)
  + CloudFront (static assets)
  Cost: ~$300/month

Stage 3: Scale horizontally (10K–100K users)  
  Route53 → ALB → Auto Scaling Group (EC2)
  + RDS Read Replicas (for reporting/analytics)
  + SQS (async jobs: email, resize images)
  + S3 for user uploads
  Cost: ~$1,500/month

Stage 4: Microservices (100K–1M users)
  Route53 → ALB → API Gateway
  → User Service (ECS containers)
  → Order Service (ECS containers)
  → Catalog Service (ECS containers)
  + Aurora (replaces RDS — 6 copies across 3 AZs, auto-scales storage)
  + ElastiCache Cluster (Redis cluster mode)
  + OpenSearch (Elasticsearch for search)
  Cost: ~$10,000/month

Stage 5: Multi-region (1M+ users)
  Route53 with Geolocation routing
  → US-East: ALB → ECS → Aurora Global (primary writer)
  → EU-West: ALB → ECS → Aurora Global (replica writer, 1s lag)
  → AP-Southeast: ALB → ECS → Aurora Global (replica)
  + DynamoDB Global Tables (sessions, carts — <10ms globally)
  + CloudFront (content from 400+ edge locations)
  Cost: ~$100,000/month
```

---

## Quick Reference: Which Diagram to Study for Which Interview

| Interview Question | Diagrams to Study |
|-------------------|-------------------|
| "Design a URL shortener" | §1 URL Shortener |
| "Design Twitter / Instagram" | §2 Twitter Feed + Caching (§6) |
| "Design Netflix / YouTube" | §3 Netflix |
| "Design Uber / Lyft" | §4 Uber |
| "Design WhatsApp / Slack" | §5 WhatsApp |
| "Design Google Search" | §9 Web Crawler + Search indexing (file 11) |
| "Design a key-value store" | §7 Distributed Systems |
| "How does Redis work?" | §6 Caching + §7 Distributed Systems |
| "How do you scale to 1M users?" | §11 AWS Scaling |
| "Design a notification system" | §5 WhatsApp (notification section) |
| "Design a rate limiter" | File 03 (APIs) |
| "Design a payment system" | File 10 (Saga pattern) + File 13 (Security) |

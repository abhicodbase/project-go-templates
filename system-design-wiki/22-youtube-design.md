# 22 — YouTube Design

> **Interview context**: Registered user can upload a video. Any user can search and view a video.

---

## 1. Requirements Clarification

### Functional Requirements
- Registered users can **upload videos** (any size/format)
- Any user (guest or registered) can **search** videos by keyword
- Any user can **view/stream** videos
- Videos available in multiple resolutions (360p, 720p, 1080p, 4K)
- Like, dislike, comment on videos (registered users)
- Subscribe to channels
- Recommended videos sidebar

### Non-Functional Requirements
```
DAU: 2B users, 500M videos viewed/day
Uploads: 500 hours of video uploaded per minute
Storage: 500hr/min × 60min × 24hr × 365 days × ~50MB/min ≈ 13 Exabytes/year
Streaming: 500M views/day ÷ 86,400 = ~6,000 streams/sec (peak 10×: 60,000/sec)
Search: 10M searches/day = 115 searches/sec

Latency: video starts playing within 2 seconds of click
Availability: 99.99% (streaming must work globally)
Durability: uploaded video never lost
```

---

## 2. Core Entities & DB Schema

```sql
-- Users / Channels
users (
  user_id      UUID PRIMARY KEY,
  username     VARCHAR(50) UNIQUE,
  email        VARCHAR(200) UNIQUE,
  channel_name VARCHAR(100),
  avatar_url   TEXT,
  subscriber_count INT DEFAULT 0,  ← denormalized
  created_at   TIMESTAMP
)

-- Videos
videos (
  video_id     UUID PRIMARY KEY,
  user_id      UUID REFERENCES users,
  title        VARCHAR(200),
  description  TEXT,
  tags         TEXT[],
  status       ENUM('PROCESSING','READY','FAILED','DELETED'),
  raw_s3_key   TEXT,              ← original uploaded file
  duration_sec INT,
  view_count   BIGINT DEFAULT 0, ← denormalized, updated async
  like_count   INT DEFAULT 0,    ← denormalized
  thumbnail_url TEXT,
  category     VARCHAR(50),
  visibility   ENUM('PUBLIC','UNLISTED','PRIVATE'),
  created_at   TIMESTAMP
)

-- Video Resolutions (transcoded variants)
video_variants (
  variant_id   UUID PRIMARY KEY,
  video_id     UUID REFERENCES videos,
  resolution   ENUM('360p','480p','720p','1080p','4K'),
  bitrate_kbps INT,
  s3_key       TEXT,             ← transcoded HLS segments location
  size_bytes   BIGINT,
  status       ENUM('READY','PROCESSING','FAILED')
)

-- Comments (Cassandra — write-heavy, time-ordered)
-- partition by video_id, cluster by created_at DESC
comments: { video_id, comment_id (TimeUUID), user_id, text, likes, created_at }

-- Subscriptions
subscriptions (
  subscriber_id  UUID,
  channel_id     UUID,
  subscribed_at  TIMESTAMP,
  PRIMARY KEY (subscriber_id, channel_id)
)
```

---

## 3. API Design

```
# Upload
POST /videos/upload/init
  { title, description, category, tags, file_size }
  Response: { video_id, upload_url (S3 pre-signed), upload_id }

PUT {s3_presigned_url}           ← client uploads directly to S3 (chunked multipart)
POST /videos/{video_id}/upload/complete
  Response: { status: "PROCESSING" }

# Video
GET /videos/{video_id}
  Response: { title, description, view_count, like_count, variants: [{resolution, hls_url}] }

POST /videos/{video_id}/view     ← increment view counter (async)
POST /videos/{video_id}/like
DELETE /videos/{video_id}/like

# Search
GET /search?q=funny+cats&category=entertainment&sort=relevance|views|date&page=1

# Comments
GET  /videos/{video_id}/comments?sort=top|newest&page=1
POST /videos/{video_id}/comments { text }

# Subscriptions
POST   /channels/{channel_id}/subscribe
DELETE /channels/{channel_id}/subscribe
GET    /subscriptions/feed      ← new videos from subscribed channels
```

---

## 4. High-Level Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│                     UPLOAD PIPELINE                                  │
│                                                                       │
│ Creator ──HTTPS──▶ Upload Service ──▶ S3 (raw video bucket)         │
│                    (pre-signed URL)       │                          │
│                                           │ S3 event trigger         │
│                               ┌───────────▼──────────┐              │
│                               │  Transcoding Queue    │              │
│                               │  (SQS / Kafka)        │              │
│                               └───────────┬──────────┘              │
│                                           │                          │
│                               ┌───────────▼──────────┐              │
│                               │  Transcoding Workers  │              │
│                               │  (EC2 GPU fleet /     │              │
│                               │   AWS Elastic Trans.) │              │
│                               │  → 360p, 720p, 1080p  │              │
│                               │  → HLS segments        │              │
│                               │  → Thumbnail extract   │              │
│                               └───────────┬──────────┘              │
│                                           │                          │
│                               S3 Delivery Bucket + CloudFront CDN   │
└────────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────────┐
│                     STREAMING PIPELINE                               │
│                                                                       │
│  Viewer ──DNS──▶ CloudFront Edge (nearest) ──▶ HLS segments         │
│                    (cache hit: < 10ms)                               │
│                    (cache miss: CloudFront → S3 Delivery Bucket)    │
│                                                                       │
│  Player: fetches manifest.m3u8 → adaptive bitrate (360p→1080p)      │
└────────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────────┐
│                     SEARCH PIPELINE                                  │
│                                                                       │
│  Video uploaded/ready → Kafka → Indexer → Elasticsearch             │
│  User searches → API → Elasticsearch → ranked results               │
└────────────────────────────────────────────────────────────────────┘

API Layer:
  Client → API Gateway → Load Balancer → Microservices
    ├── Video Service    (MySQL: video metadata)
    ├── Search Service   (Elasticsearch)
    ├── Comment Service  (Cassandra)
    ├── View Counter     (Redis HLL → async flush to MySQL)
    └── Notification Svc (Kafka: new video from subscribed channel)
```

---

## 5. Video Upload — Deep Dive

### Multi-part S3 Upload (Large Files)
```
Video file = 10GB

Step 1: Client → API: "I want to upload 10GB file"
Step 2: API → AWS S3: CreateMultipartUpload → upload_id
Step 3: API → return to client: { upload_id, presigned_urls: [part1_url, part2_url, ...] }
         Each URL = 100MB chunk (100 parts total for 10GB)

Step 4: Client uploads each 100MB chunk DIRECTLY to S3 in parallel
         PUT part1_url (100MB) → etag1
         PUT part2_url (100MB) → etag2
         ...

Step 5: Client → API: "Upload complete" + [etag1, etag2, ...]
Step 6: API → S3: CompleteMultipartUpload(etags)
Step 7: S3 assembles final file → triggers SNS/SQS event

Benefits:
  Parallel chunk uploads → fast (10GB in ~60sec on good connection)
  Resumable: if part 5 fails → only re-upload part 5, not whole file
  API servers never touch video bytes (just presigned URLs)
```

### Transcoding Workers
```
Raw video arrives → worker picks up from SQS:

for each resolution [360p, 480p, 720p, 1080p]:
    ffmpeg -i input.mp4 \
           -vf scale=1280:720 \       ← resize
           -b:v 2500k \               ← video bitrate
           -hls_time 6 \              ← 6-second segments
           -hls_playlist_type vod \
           -f hls output_720p.m3u8

Generated files:
  output_720p.m3u8           ← playlist (index)
  output_720p_001.ts         ← segment 1 (6 sec)
  output_720p_002.ts         ← segment 2 (6 sec)
  ...

Upload all segments to S3 delivery bucket:
  s3://youtube-delivery/{video_id}/720p/
  
Update DB: video_variants SET status = 'READY', s3_key = 's3://...'
Publish Kafka event: "video.ready" → Elasticsearch indexer + notifications
```

---

## 6. View Counter — High Throughput Challenge

```
Problem: 60,000 video views/sec → 60,000 DB UPDATE view_count + 1 per second → DB melts

Solution 1: Redis HyperLogLog (approximate, unique views)
  PFADD views:video_123 {user_id}     ← add viewer
  PFCOUNT views:video_123             ← unique count (~0.81% error, very compact)
  
  Background job every 5 minutes:
    counts = PFCOUNT for all videos
    BULK UPDATE videos SET view_count = counts

Solution 2: Redis Counter + Periodic Flush
  INCR viewcount:video_123            ← atomic increment in Redis (in-memory, fast)
  
  Background job every 60 seconds:
    for each video: GET viewcount:video_123
    BULK UPDATE videos SET view_count = view_count + redis_count
    DEL viewcount:video_123           ← reset counter after flush

Solution 3: Kafka aggregation
  Each view → publish to Kafka "view" topic
  Kafka Streams: count by video_id, tumbling window 60s
  → Batch update DB every 60s

→ Use Redis counter flush for simplicity, Kafka for audit trail
```

---

## 7. Search — Deep Dive

```
Elasticsearch index for videos:
{
  "video_id": "abc123",
  "title": "Funny Cats Compilation 2026",
  "description": "...",
  "tags": ["cats", "funny", "compilation"],
  "category": "entertainment",
  "view_count": 5000000,
  "like_count": 150000,
  "published_at": "2026-09-01",
  "channel_name": "CatLovers"
}

Search query: "funny cats"
  Relevance score = BM25(title match) × 0.5
                  + BM25(description match) × 0.2
                  + BM25(tags match) × 0.3
                  + log(view_count) × 0.1   ← popularity boost
                  + recency_boost × 0.05

Sort options:
  "Relevance" → BM25 score
  "Upload date" → published_at DESC
  "View count" → view_count DESC
```

---

## 8. Recommendations

```
Simple:
  Redis Sorted Set per category:
    ZADD trending:entertainment {view_count} {video_id}
    ZREVRANGE trending:entertainment 0 9 → top 10 entertainment videos

Advanced (collaborative filtering):
  Spark ML: find users who watched same videos as user X
  → recommend videos those users watched that X hasn't seen

Near real-time:
  Video watch event → Kafka → Recommendation Engine
  → Update user's recommendation feed (stored in Redis or Cassandra)
  → Next time user opens app → feed is ready
```

---

## Key Talking Points

1. **Pre-signed S3 URL** for upload — never proxy large files through your servers
2. **Transcoding pipeline** — async, separate service, multiple resolutions + HLS
3. **CDN** is everything for streaming — 99% of stream requests served by CloudFront edge
4. **View counter** — can't do direct DB updates at 60K/sec → Redis buffer + periodic flush
5. **Adaptive bitrate (HLS)** — explain how player switches quality based on bandwidth
6. **Search** — Elasticsearch with popularity boost (not just text relevance)
7. **Scale storage**: ~13 Exabytes/year — use S3 tiering (standard → Glacier after 1 year)

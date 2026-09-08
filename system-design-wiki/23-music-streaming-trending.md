# 23 — Music Streaming + Trending Songs (Spotify-like)

> **Interview context**: Music streaming app. Trending songs globally + by region. Scalability, fault tolerance, millions of concurrent listeners. DB schema + storage choices.

---

## 1. Requirements Clarification

### Functional Requirements
- Users can **stream songs** (any device)
- **Search** songs, artists, albums
- **Playlists**: create, follow, share
- **Trending songs**: globally and by region (top 50 per country)
- Song recommendations based on listening history
- Offline playback (downloaded songs)

### Non-Functional Requirements
```
DAU: 500M users (Spotify scale)
Songs: 100M tracks
Streams: 500M × 5 songs/day = 2.5B plays/day = 29,000 plays/sec (peak 3×: 87,000/sec)
Search: 50M searches/day = 578/sec

Latency: song starts playing within 1 second of click
Availability: 99.99% (streaming service = 0 downtime tolerance)
Durability: licensed songs must never be lost
```

---

## 2. Core Entities & DB Schema

```sql
-- Songs
songs (
  song_id       UUID PRIMARY KEY,
  title         VARCHAR(200),
  artist_id     UUID REFERENCES artists,
  album_id      UUID REFERENCES albums,
  duration_sec  INT,
  genre         VARCHAR(50),
  language      VARCHAR(10),
  release_date  DATE,
  play_count    BIGINT DEFAULT 0,    ← denormalized, updated async
  lyrics        TEXT,
  storage_key   TEXT,                ← S3 key for audio file
  waveform_url  TEXT,                ← pre-generated waveform image
  is_explicit   BOOLEAN,
  is_licensed   BOOLEAN              ← regional licensing
)

-- Artists
artists (
  artist_id     UUID PRIMARY KEY,
  name          VARCHAR(200),
  bio           TEXT,
  photo_url     TEXT,
  follower_count INT DEFAULT 0       ← denormalized
)

-- Albums
albums (
  album_id      UUID PRIMARY KEY,
  title         VARCHAR(200),
  artist_id     UUID,
  release_date  DATE,
  cover_url     TEXT,
  genre         VARCHAR(50)
)

-- Playlists
playlists (
  playlist_id   UUID PRIMARY KEY,
  name          VARCHAR(200),
  user_id       UUID,
  is_public     BOOLEAN,
  follower_count INT DEFAULT 0,
  cover_url     TEXT,
  created_at    TIMESTAMP
)

playlist_songs (
  playlist_id   UUID,
  song_id       UUID,
  position      INT,                 ← ordering
  added_at      TIMESTAMP,
  PRIMARY KEY (playlist_id, position)
)

-- User Listening History (Cassandra — write-heavy, append-only)
-- Partition by user_id, cluster by played_at DESC
listening_history: {
  user_id, played_at (TimeUUID), song_id, duration_listened_sec,
  skipped (BOOLEAN), completed (BOOLEAN), country_code
}

-- Trending scores (Redis Sorted Sets — pre-computed)
-- trending:global:weekly → { song_id: play_count_this_week }
-- trending:IN:weekly     → { song_id: play_count_from_IN_this_week }
```

---

## 3. API Design

```
# Stream
GET /songs/{song_id}/stream           ← returns CDN presigned URL for audio
  Response: { stream_url, duration_sec, cdn_ttl: 3600 }

POST /songs/{song_id}/play-event      ← record play event (async, for analytics)
  { user_id, country, duration_listened, skipped }

# Search
GET /search?q=shape+of+you&type=song,artist,album&limit=20

# Trending
GET /trending/songs                   ← global top 50
GET /trending/songs?region=IN         ← India top 50
GET /trending/songs?region=US&timeframe=daily|weekly|monthly

# Recommendations
GET /users/me/recommendations?limit=20  ← personalized feed

# Playlists
GET /playlists/{playlist_id}
POST /playlists { name, is_public }
POST /playlists/{playlist_id}/songs { song_id, position }
DELETE /playlists/{playlist_id}/songs/{song_id}

# Download (offline)
GET /songs/{song_id}/download         ← encrypted download (DRM protected)
```

---

## 4. High-Level Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                     STREAMING PATH                            │
│                                                               │
│  Client ──▶ API → "get stream url"                           │
│  API: generate short-lived CDN pre-signed URL                │
│  Client ──▶ CloudFront (nearest edge) ──▶ S3 (audio files)  │
│  (99% cache hit for popular songs at edge)                   │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│                     SEARCH PATH                               │
│                                                               │
│  Client ──▶ Search Service ──▶ Elasticsearch                 │
│  (songs indexed by: title, artist, album, genre, tags)       │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│                     TRENDING PIPELINE                         │
│                                                               │
│  Play event ──▶ API ──▶ Kafka (play_events topic)            │
│                         │                                     │
│                 ┌───────▼────────┐                           │
│                 │ Flink / Kafka  │  Aggregates per song      │
│                 │ Streams        │  per region per time window│
│                 └───────┬────────┘                           │
│                         │                                     │
│            ┌────────────┼────────────────┐                   │
│            ▼            ▼                ▼                   │
│        Redis           Cassandra        Data Warehouse       │
│    trending:global  (raw aggregates)    (Spark, BI tools)   │
│    trending:IN      per-day per-region                       │
│    trending:US                                               │
└──────────────────────────────────────────────────────────────┘

Core Services:
  Song Service      ← MySQL (song metadata, artist, album)
  User Service      ← MySQL (users, subscriptions)
  Playlist Service  ← MySQL (playlists, playlist_songs)
  History Service   ← Cassandra (listening_history — write heavy)
  Search Service    ← Elasticsearch
  Trending Service  ← Redis Sorted Sets
  Recommendation    ← ML Pipeline (Spark + DynamoDB for results)
```

---

## 5. Trending Songs — Deep Dive

### Requirements
```
"Top 50 songs in India this week" needs to update in near real-time (every 5 min).
Play event → ranked list update within ~5 minutes.
```

### Architecture
```
Every song play → Kafka "play_events" topic
  { user_id, song_id, country_code, played_at, duration_sec, completed: true/false }

Flink Streaming Job (sliding window):
  Window: 1 hour, slides every 5 minutes
  For each window: COUNT plays by (song_id, country_code)
  Output: { song_id, country, play_count_1hr }

Daily aggregation (batch, runs every midnight):
  Spark job reads from Kafka/Cassandra:
    SELECT song_id, country_code, COUNT(*) FROM play_events
    WHERE played_at > NOW() - 7 days
    GROUP BY song_id, country_code
    ORDER BY COUNT DESC

Results stored in Redis Sorted Sets:
  Weekly global: ZADD trending:global:weekly {weekly_count} {song_id}
  Weekly India:  ZADD trending:IN:weekly     {weekly_count} {song_id}
  Weekly US:     ZADD trending:US:weekly     {weekly_count} {song_id}

Trending API:
  GET /trending/songs?region=IN
  → ZREVRANGE trending:IN:weekly 0 49 WITHSCORES
  → Return top 50 song_ids
  → Fetch metadata: MGET song:{id} from Redis or MySQL
  → Response in < 10ms
```

### Why Not Direct DB Count?
```
❌ Bad: SELECT song_id, COUNT(*) FROM plays WHERE week = ? GROUP BY song_id ORDER BY COUNT DESC
  → Billion-row table scan every request → too slow

✅ Good: Pre-compute with Flink → store in Redis Sorted Set → O(log N) read
  → Result in < 1ms from Redis
```

### Trending by Region
```
Country detection:
  From user profile.country OR IP geolocation (MaxMind)
  
Hierarchy:
  City → Country → Region → Global
  (can drill down: trending in Mumbai, trending in India, trending globally)

Data:
  Kafka event includes country_code from IP lookup
  Flink partitions by country_code → separate sorted sets per country
```

---

## 6. Audio Storage & Delivery

### Audio Formats
```
Master (storage): FLAC or WAV (lossless, ~30MB/song)
  → Never modified, permanent archive in S3 Glacier (cheap)

Delivery variants (transcoded on upload):
  320kbps MP3   ← Premium users, ~5MB/song/4min
  128kbps MP3   ← Free users, ~2MB/song/4min
  64kbps MP3    ← Low bandwidth mode, ~1MB/song/4min
  OGG Vorbis   ← Mobile optimized

Storage: 100M songs × 5MB (320kbps) = 500 TB → S3 Standard
         100M songs × 30MB (FLAC archive) = 3 PB → S3 Glacier
```

### DRM (Digital Rights Management)
```
Spotify uses Widevine DRM for offline downloads:
  1. App requests license key for song from License Server
  2. License Server checks: is user subscribed? Is this song licensed in their country?
  3. If yes: issue encrypted key (time-limited: valid 30 days offline)
  4. App decrypts audio with key → plays locally
  5. After 30 days: key expires → must re-validate online
  
  → Prevents sharing downloaded files (key is device-bound)
```

---

## 7. Fault Tolerance

```
"How to handle millions of requests + fault tolerance?"

CDN failure:
  CloudFront → fallback to origin S3 (slower but available)
  Multi-CDN: use both CloudFront + Fastly → DNS failover

Streaming service failure:
  Audio is served directly from CDN, not app servers
  App server only serves presigned URL → if app server down:
    User already has presigned URL → stream continues uninterrupted for TTL duration (1hr)
    
Kafka failure:
  Play events are fire-and-forget from client's perspective
  Client: send event → if 500 → drop (analytics are eventually consistent, not critical)
  
Redis failure (trending broken):
  Fallback: serve last known trending list from DB
  Redis Sentinel / Cluster: automatic failover < 30s

Cassandra failure (listening history):
  Multi-datacenter Cassandra with RF=3
  Writes: quorum (2/3 must ack) → survive 1 DC failure
  Reads: LOCAL_QUORUM → fast reads from nearest DC
```

---

## 8. DB Schema Deep Dive (What Interviewers Ask)

```
"Why Cassandra for listening history?"
  → Write-heavy (every song play = 1 write = 29K writes/sec)
  → Time-series (query: "last 50 songs for user X")
  → Append-only (history never updated, just appended)
  → Partitioned by user_id → perfect for user-specific queries
  → Scales horizontally without hot spots

"Why Redis for trending?"
  → Sorted Set = ordered by score automatically
  → ZADD is O(log N) write, ZREVRANGE is O(log N + M) read
  → In-memory = microsecond reads for a hot global endpoint
  → Small data: 100M songs × 8 bytes = ~800MB → fits in RAM easily

"Why Elasticsearch for search?"
  → Inverted index for full-text (song title: "shape of you")
  → Typo tolerance (fuzzy)
  → Faceted search (filter by genre, year, explicit)
  → Songs are small documents → 100M songs = ~100GB ES index (manageable)

"Why MySQL for song metadata?"
  → Song data is relational (song → album → artist → label)
  → Songs are rarely updated
  → Transactional (licensing changes, takedowns)
  → 100M songs × 1KB = 100GB → single sharded MySQL (or Aurora)
```

---

## Key Talking Points

1. **CDN** does all the heavy lifting for streaming — don't route audio through app servers
2. **Trending = pre-computed** (Flink → Redis) — never compute at query time
3. **Regional trending** = country tag on every play event → partition aggregation by country
4. **Cassandra** for history (write-heavy, append-only, time-series)
5. **Redis Sorted Set** for trending (ZADD = insert, ZREVRANGE = top-N)
6. **DRM** for offline — show you know about licensing and content protection
7. **Scale**: mention audio is 500TB → S3; trending data is small → Redis
8. **Fault tolerance**: CDN for streaming, Kafka for events (durable), Redis Sentinel for trending

# 19 — Nearby Places Recommender (Yelp / Google Maps Local)

> **Interview context**: Design a system to help users discover relevant places (restaurants, cafes, salons, etc.) near their current location, similar to Yelp, Google Maps, or Facebook Local.

![Nearby Places Recommender — High Level Design](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/nearby-places-hld.jpg)

---

## 1. Requirements Clarification

### Functional Requirements
- User enters location (GPS or city search) → get ranked list of nearby places
- Filter by: category (restaurant, cafe, salon), rating, price level, open now
- View place details: name, address, phone, hours, photos, reviews
- Search by keyword: "best biryani near me"
- Add reviews and ratings
- Business owners can add/claim/edit their listing

### Non-Functional Requirements
- Search results in < 100ms
- 500M DAU (Google Maps scale); 1B places in DB
- High read:write ratio (1000:1)
- Location data refreshed in near real-time (user moves)
- Availability: 99.99% (critical infrastructure)

---

## 2. Capacity Estimation

```
Searches:  500M users × 5 searches/day = 2.5B searches/day = 29,000 searches/sec
           Peak 3×: 90,000 searches/sec

Places:    1B places globally
Reviews:   10B reviews (avg 10 per place)
Photos:    50B photos (avg 50 per place)

Storage:
  Places:  1B × 1 KB = 1 TB (metadata)
  Reviews: 10B × 500B = 5 TB
  Photos:  50B photos × 100KB avg = 5 PB (on S3)
```

---

## 3. Core Entities & DB Schema

```sql
-- Places (core entity — must support geospatial queries)
places (
  place_id    UUID PRIMARY KEY,
  name        VARCHAR(200),
  category    VARCHAR(50),       ← "restaurant", "cafe", "salon"
  subcategory VARCHAR(50),       ← "Italian", "Sushi", "Nail Salon"
  lat         DECIMAL(9,6),
  lng         DECIMAL(9,6),
  geohash     VARCHAR(12),       ← pre-computed for fast prefix search
  address     TEXT,
  city        VARCHAR(100),
  country     VARCHAR(2),
  phone       VARCHAR(20),
  website     VARCHAR(300),
  price_level TINYINT,           ← 1=$, 2=$$, 3=$$$, 4=$$$$
  avg_rating  DECIMAL(3,2),      ← denormalized, updated after each review
  review_count INT,              ← denormalized
  is_verified BOOLEAN,
  is_open_now BOOLEAN,           ← updated by scheduler every hour
  hours       JSONB,             ← {"mon": "9:00-22:00", "tue": ...}
  created_at  TIMESTAMP
)
CREATE INDEX idx_places_geohash ON places(geohash);
CREATE INDEX idx_places_category ON places(category, avg_rating DESC);
-- PostGIS: CREATE INDEX idx_places_geo ON places USING GIST(ST_MakePoint(lng, lat));

-- Reviews
reviews (
  review_id   UUID PRIMARY KEY,
  place_id    UUID REFERENCES places,
  user_id     UUID,
  rating      TINYINT,          ← 1-5
  text        TEXT,
  photos      TEXT[],           ← S3 URLs
  helpful_votes INT DEFAULT 0,
  created_at  TIMESTAMP
)
CREATE INDEX idx_reviews_place ON reviews(place_id, created_at DESC);

-- Place Photos
place_photos (
  photo_id    UUID PRIMARY KEY,
  place_id    UUID REFERENCES places,
  s3_url      TEXT,
  caption     TEXT,
  uploaded_by UUID,             ← user or business owner
  sort_order  INT
)
```

---

## 4. API Design

```
# Nearby Search (CORE)
GET /places/nearby
  ?lat=40.7128&lng=-74.0060
  &radius=2                    ← km (default 5, max 50)
  &category=restaurant
  &min_rating=4.0
  &price_level=1,2             ← $ or $$
  &open_now=true
  &keyword=biryani             ← full-text search within results
  &sort=distance|rating|relevance
  &page=1&limit=20

Response: [{ place_id, name, distance_m, avg_rating, review_count, price_level, photo, is_open }]

# Place Detail
GET /places/{place_id}
Response: full place object + business hours + top 3 photos

# Reviews
GET /places/{place_id}/reviews?sort=newest|helpful&page=1
POST /places/{place_id}/reviews
  { rating: 4, text: "Great food!", photos: ["s3://..."] }

# Search by Keyword (city-level, not geo)
GET /places/search?q=best+sushi+nyc&category=restaurant

# Business Owner: Add/Update Place
POST /places               ← add new place
PUT  /places/{place_id}   ← update (owner only)
```

---

## 5. High-Level Design

```
Client (GPS) ──▶ API Gateway ──▶ Load Balancer
                                       │
            ┌──────────────────────────┼──────────────────────────┐
            │                          │                          │
    ┌───────▼──────┐          ┌────────▼──────┐         ┌───────▼───────┐
    │ Search Service│          │  Place Service │         │ Review Service│
    │(proximity +   │          │(CRUD, photos)  │         │(write reviews,│
    │ keyword)      │          │               │         │  vote helpful)│
    └───────┬──────┘          └────────┬──────┘         └───────┬───────┘
            │                          │                          │
    ┌───────▼──────┐          ┌────────▼──────┐         ┌───────▼───────┐
    │Elasticsearch  │          │  PostgreSQL    │         │ Cassandra     │
    │  (geo index  │          │  (place master │         │ (reviews,     │
    │  + full-text)│          │   data + hours)│         │  partitioned  │
    └──────────────┘          └───────────────┘         │  by place_id) │
                                                         └───────────────┘
    ┌──────────────────────────────────────────────────────────────┐
    │                     Redis Cluster                             │
    │  • Popular search cache: nearby:{geohash5}:{filters} → 5min  │
    │  • Place detail cache: place:{id} → 10min                    │
    │  • Trending places per city: trending:{city} → 1hr           │
    └──────────────────────────────────────────────────────────────┘
    
    Writes:
    Place added/updated → Kafka → ES indexer (sync ES within seconds)
    Review added → Kafka → Rating Aggregator → UPDATE places.avg_rating
```

---

## 6. Proximity Search — Three Approaches Compared

### Approach 1: SQL + PostGIS (good for < 10M places)
```sql
SELECT *, 
  ST_Distance(ST_SetSRID(ST_Point(lng, lat), 4326)::geography,
              ST_SetSRID(ST_Point(-74.006, 40.713), 4326)::geography) AS dist
FROM places
WHERE ST_DWithin(
  ST_SetSRID(ST_Point(lng, lat), 4326)::geography,
  ST_SetSRID(ST_Point(-74.006, 40.713), 4326)::geography,
  2000  ← 2km in meters
)
AND category = 'restaurant'
AND avg_rating >= 4.0
ORDER BY dist ASC
LIMIT 20;
```

### Approach 2: Geohash Prefix Search (fast, scalable)
```
User location → geohash: 40.7128, -74.0060 → "dr5ru"

Nearby cells (9 cells total including center):
  dr5ru, dr5rg, dr5rv, dr5ry, dr5rs, dr5rt, dr5rm, dr5rk, dr5rj

Query: SELECT * FROM places WHERE geohash LIKE 'dr5r%' AND category = 'restaurant'
Post-filter: compute exact distance, keep < 2km

Precision 5 = ~4.9km cell → covers 2km radius with 1-level expansion
Precision 6 = ~1.2km cell → for tight radius queries
```

### Approach 3: Elasticsearch geo_distance (production choice)
```json
{
  "query": {
    "bool": {
      "filter": [
        { "geo_distance": { "distance": "2km", "location": { "lat": 40.71, "lon": -74.01 } } },
        { "term": { "category": "restaurant" } },
        { "range": { "avg_rating": { "gte": 4.0 } } },
        { "term": { "is_open_now": true } }
      ],
      "must": [
        { "match": { "name_description": "biryani" } }  ← keyword search
      ]
    }
  },
  "sort": [
    { "_geo_distance": { "location": "40.71,-74.01", "order": "asc" } },
    { "avg_rating": "desc" }
  ]
}
```

> **Production choice: Elasticsearch** — combines geo + full-text + filtering in one query. Scale to 1B+ documents.

---

## 7. Ranking Algorithm

Pure distance is not enough. Relevance = distance × quality × personalization.

```
score = (1 / (distance_km + 0.1)) × 0.4     ← closer is better
      + (avg_rating / 5.0) × 0.3              ← higher rating is better
      + log(review_count + 1) × 0.1           ← more reviews = more trustworthy
      + recency_bonus × 0.1                   ← recently active places
      + personalization_score × 0.1           ← user's past category preferences

personalization_score:
  user_history = categories user searched/visited recently
  if place.category in user_history → boost 0.2
  if user rated similar places 4+ → boost 0.1
```

---

## 8. Real-time "Open Now" Feature

```
Approach 1: Query + compute (simple, always accurate)
  Place has: hours = { "mon": "09:00-22:00", ... }
  At query time: if current_time between open and close → is_open = true

Approach 2: Pre-computed column (faster queries)
  Scheduler runs every 30 min:
    SELECT * FROM places WHERE hours is not null
    For each place: compute is_open based on current time + hours + timezone
    UPDATE places SET is_open_now = true/false WHERE place_id = ?

  Query: ... AND is_open_now = true  ← simple indexed column
  Trade-off: up to 30min staleness on open/close status (acceptable)
```

---

## 9. Review System

### Handling Fake Reviews
```
Signals for fake review detection:
  1. New account (< 7 days old) posting many reviews quickly
  2. All reviews for same business from same IP/device subnet
  3. All 5-star reviews with same writing pattern (NLP similarity)
  4. Reviewer has never checked in or ordered from the place

Actions:
  Score each review with trust_score (0-1.0)
  low_trust_score reviews → hold for manual review
  Very low → auto-hide but keep in DB (appeals)
```

### Rating Aggregation (Eventual Consistency is OK)
```
Review created → Kafka "review.created" event
  → Rating Aggregator consumer:
      new_avg = (old_avg × old_count + new_rating) / (old_count + 1)
      UPDATE places SET avg_rating = new_avg, review_count = review_count + 1
      
  Delay: 1-5 seconds for avg_rating to update
  → Acceptable for ratings (not real-time critical)
```

---

## 10. Scale Trade-offs

| Challenge | Solution |
|-----------|---------|
| 90K searches/sec | ES cluster + Redis cache of popular geohash+filter combos (60% hit rate) |
| 1B places in ES | Horizontal sharding (10 shards × 3 replicas = 30 nodes) |
| Photo storage (5 PB) | S3 + CloudFront, thumbnails at multiple sizes |
| Review writes (global) | Cassandra (wide-column, partitioned by place_id) |
| Rating freshness | Denormalized avg_rating updated async via Kafka (< 5s lag) |
| "Open now" query | Pre-computed column updated by cron every 30min |
| Global scale | Multi-region ES clusters; DNS geolocation routing |

### Key Interview Talking Points
1. **Geo search** — start with geohash, mention ES geo_distance for production
2. **Ranking** — distance alone is bad UX; explain composite score
3. **Scale**: 90K QPS needs ES sharding + Redis caching — give numbers
4. **Denormalization**: avg_rating on place row (not computed at query) — critical for performance
5. **CAP**: search = AP (eventual), review writes = strong (once written, visible within seconds)
6. **Fake reviews** — always mention, shows product thinking

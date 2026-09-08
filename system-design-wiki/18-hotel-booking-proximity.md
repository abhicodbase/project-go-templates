# 18 — Hotel Booking & Proximity Search (Booking.com)

> **Interview context**: Hotel booking system with emphasis on proximity-based search, real-time availability, and pricing.

---

## 1. Requirements Clarification

### Functional Requirements
- Users can **search hotels** by location (lat/lng, city, radius)
- Filter by: check-in/out dates, guests, price range, amenities, rating
- View hotel details, photos, room types, availability, pricing
- **Book a room** — reserve specific room type for date range
- Manage bookings: view, cancel, modify
- Hotel operators: add/update rooms, manage inventory, set pricing

### Non-Functional Requirements
- Search results in < 200ms (proximity queries especially)
- Booking is **ACID** — no double booking
- Support **50M DAU**, 10M searches/day, 500K bookings/day
- High read-to-write ratio (~100:1 for search vs booking)
- Hotel inventory always **consistent** — overbooking is unacceptable

### Out of Scope
- Payment processing (just reserve; payment via 3rd party)
- Reviews/ratings system
- Hotel CRM features

---

## 2. Capacity Estimation

```
Searches:  10M searches/day ÷ 86,400 = ~115 searches/sec (peak 5×: 600/sec)
Bookings:  500K bookings/day ÷ 86,400 = ~6 bookings/sec (peak 5×: 30/sec)

Hotel count: 1M hotels globally
Rooms per hotel: avg 50 → 50M rooms total
Availability records: 50M rooms × 365 days = 18.25B rows/year

Storage (availability table):
  18.25B rows × 50 bytes/row ≈ 900 GB/year
  → Partition by date, shard by hotel_id
```

---

## 3. Core Entities & DB Schema

```sql
-- Hotels
hotels (
  hotel_id     UUID PRIMARY KEY,
  name         VARCHAR(200),
  description  TEXT,
  lat          DECIMAL(9,6),    ← indexed with PostGIS / geospatial index
  lng          DECIMAL(9,6),
  address      TEXT,
  city         VARCHAR(100),
  country      VARCHAR(2),
  star_rating  TINYINT,
  amenities    JSONB,           ← ["wifi", "pool", "gym", "parking"]
  created_at   TIMESTAMP
)

-- Room Types (per hotel)
room_types (
  room_type_id UUID PRIMARY KEY,
  hotel_id     UUID REFERENCES hotels,
  name         VARCHAR(100),    ← "Deluxe King", "Standard Twin"
  capacity     TINYINT,         ← max guests
  base_price   DECIMAL(10,2),
  amenities    JSONB,
  total_count  INT              ← how many rooms of this type exist
)

-- Room Inventory (availability per date per room type)
-- KEY TABLE: must be consistent under concurrent bookings
room_inventory (
  hotel_id     UUID,
  room_type_id UUID,
  date         DATE,
  total_rooms  INT,
  booked_rooms INT,             ← use atomic increment/decrement
  price        DECIMAL(10,2),   ← dynamic pricing per date
  PRIMARY KEY (hotel_id, room_type_id, date)
)

-- Bookings
bookings (
  booking_id    UUID PRIMARY KEY,
  user_id       UUID,
  hotel_id      UUID,
  room_type_id  UUID,
  check_in      DATE,
  check_out     DATE,
  guests        TINYINT,
  total_price   DECIMAL(10,2),
  status        ENUM('PENDING','CONFIRMED','CANCELLED'),
  created_at    TIMESTAMP,
  idempotency_key VARCHAR(64) UNIQUE   ← prevent double booking on retry
)

-- Photos
hotel_photos (
  photo_id   UUID PRIMARY KEY,
  hotel_id   UUID,
  url        TEXT,              ← S3 URL
  type       ENUM('exterior','room','amenity'),
  sort_order INT
)
```

---

## 4. API Design

```
# Search
GET /hotels/search
  ?lat=40.7128&lng=-74.0060
  &radius=5                    ← km
  &check_in=2026-10-01
  &check_out=2026-10-05
  &guests=2
  &min_price=50&max_price=300
  &amenities=pool,wifi
  &sort=price_asc|distance|rating
  &page=1&limit=20

Response: [{ hotel_id, name, distance_km, price_from, rating, photos[0], availability_count }]

# Hotel Detail
GET /hotels/{hotel_id}
  ?check_in=2026-10-01&check_out=2026-10-05&guests=2
Response: hotel details + available room types with pricing

# Check Availability
GET /hotels/{hotel_id}/rooms/{room_type_id}/availability
  ?check_in=2026-10-01&check_out=2026-10-05

# Create Booking
POST /bookings
  { hotel_id, room_type_id, check_in, check_out, guests, idempotency_key }
Response: { booking_id, status: "CONFIRMED", total_price }

# Get Bookings
GET /bookings?user_id={id}&status=CONFIRMED

# Cancel Booking
DELETE /bookings/{booking_id}
```

---

## 5. High-Level Design

```
                    ┌─────────────────────────────────────────────────┐
                    │           CDN (CloudFront)                       │
                    │         Static: photos, JS, CSS                  │
                    └──────────────────┬──────────────────────────────┘
                                       │
Client ──────────────────▶ API Gateway / Load Balancer (L7)
                                       │
          ┌────────────────────────────┼─────────────────────────┐
          │                            │                          │
   ┌──────▼──────┐            ┌───────▼───────┐         ┌───────▼───────┐
   │Search Service│           │Booking Service │         │Hotel Service  │
   │(proximity)   │           │(ACID critical) │         │(CRUD, pricing)│
   └──────┬──────┘            └───────┬───────┘         └───────┬───────┘
          │                           │                          │
          │                    ┌──────▼──────┐                  │
     ┌────▼─────┐              │Inventory     │           ┌──────▼──────┐
     │Elasticsearch│           │Lock Service  │           │PostgreSQL    │
     │(geo-search) │           │(Redis+DB)    │           │(hotel data)  │
     └────┬─────┘              └──────┬──────┘           └─────────────┘
          │                           │
          │                    ┌──────▼──────┐
   ┌──────▼──────┐             │PostgreSQL    │
   │PostgreSQL    │             │(bookings,    │
   │(hotels +     │             │ inventory)   │
   │ PostGIS)     │             └─────────────┘
   └─────────────┘

Async:
  Booking Confirmed → Kafka → Notification Service → Email/SMS
                           → Analytics Service
```

---

## 6. Proximity Search — Deep Dive

### Why Not SQL LIKE for Location?
```sql
-- ❌ Naive: distance function on every row = O(n) full scan
SELECT *, ST_Distance(location, point) AS dist FROM hotels
WHERE dist < 5000
ORDER BY dist;

-- ✅ Geospatial index (PostGIS or MySQL Spatial Index):
SELECT *, ST_Distance(location, ST_SetSRID(ST_Point(-74.006, 40.713), 4326)) AS dist
FROM hotels
WHERE ST_DWithin(location, ST_SetSRID(ST_Point(-74.006, 40.713), 4326), 5000)
ORDER BY dist ASC
LIMIT 20;
-- PostGIS uses R-tree index → O(log n) for bounding box, then distance filter
```

### Elasticsearch Geo Distance Query (Better for Scale)
```json
GET /hotels/_search
{
  "query": {
    "bool": {
      "filter": [
        {
          "geo_distance": {
            "distance": "5km",
            "location": { "lat": 40.7128, "lon": -74.0060 }
          }
        },
        { "term": { "amenities": "wifi" } },
        { "range": { "price_from": { "gte": 50, "lte": 300 } } }
      ]
    }
  },
  "sort": [
    { "_geo_distance": { "location": { "lat": 40.7128, "lon": -74.0060 }, "order": "asc" } }
  ]
}
```

### Geohash for Proximity (Alternative at Scale)
```
Geohash encodes lat/lng into a string prefix:
  San Francisco → "9q8yy" (precision 5 ≈ 4.9km × 4.9km cell)
  
Nearby search:
  1. Compute geohash of user's location
  2. Compute 8 neighboring geohash cells (N, S, E, W, NE, NW, SE, SW)
  3. Query DB: WHERE geohash IN (9q8yy, 9q8yx, 9q8yz, ...) → covers all nearby hotels
  4. Post-filter: calculate exact distance, remove > radius
  
Trade-off: geohash is approximate, boundary cells need neighbor expansion
```

---

## 7. Booking — Preventing Double Booking (Critical!)

### The Race Condition Problem
```
T=0: User A requests last room for Oct 1
T=0: User B requests last room for Oct 1
T=1: Both check inventory → 1 room available → both proceed
T=2: Both insert booking → now 2 bookings for 1 room! ❌
```

### Solution: Optimistic Locking on Inventory
```sql
-- Step 1: Read inventory WITH version
SELECT total_rooms, booked_rooms, version
FROM room_inventory
WHERE hotel_id = ? AND room_type_id = ? AND date = ?
-- Result: total=5, booked=4, version=10 → 1 room free

-- Step 2: Update only if version hasn't changed (optimistic lock)
UPDATE room_inventory
SET booked_rooms = booked_rooms + 1, version = version + 1
WHERE hotel_id = ? AND room_type_id = ? AND date = ?
  AND version = 10                      ← CAS (compare and swap)
  AND (total_rooms - booked_rooms) >= 1 ← still has room

-- If 0 rows affected → someone else booked it → retry or return "sold out"
-- If 1 row affected → success → create booking record (same transaction)
```

### Solution 2: Pessimistic Locking (for highly contended inventory)
```sql
BEGIN TRANSACTION;
SELECT * FROM room_inventory
WHERE hotel_id = ? AND room_type_id = ? AND date BETWEEN ? AND ?
FOR UPDATE;  ← row-level lock, blocks other writers

-- check availability, decrement booked_rooms, insert booking
COMMIT;  ← releases lock
```
> Use pessimistic locking only for very high contention (sold-out scenarios). Optimistic is preferred for normal load.

### Idempotency Key (Prevent Duplicate on Network Retry)
```
Client generates: idempotency_key = UUID()
POST /bookings { ..., idempotency_key: "abc-123" }

Server:
  SELECT * FROM bookings WHERE idempotency_key = 'abc-123'
  If found → return existing booking (don't create duplicate)
  If not → create booking
  
Client retries on timeout → same key → safe
```

---

## 8. Dynamic Pricing (Revenue Management)

```
base_price = room_type.base_price

Multipliers:
  occupancy_rate = booked_rooms / total_rooms
  if occupancy_rate > 0.8:  multiplier = 1.5x  (surge)
  if occupancy_rate > 0.95: multiplier = 2.0x  (last rooms)
  if days_until_checkin < 7: multiplier = 1.2x  (last minute)
  if it's peak season (summer/holidays): multiplier = 1.3x

final_price = base_price × occupancy_multiplier × timing_multiplier × season_multiplier

Stored: room_inventory.price updated nightly by Pricing Engine
         (or computed at search time from rules)
```

---

## 9. Caching Strategy

```
L1 (App server local): hotel metadata (name, photos, amenities) — changes rarely
  TTL: 30 minutes, invalidated on hotel update

L2 (Redis): hotel search results by geohash + date range
  Key: search:{geohash5}:{checkin}:{checkout}:{guests}:{filters_hash}
  TTL: 5 minutes (availability changes frequently)

L3 (Elasticsearch): geo-indexed hotels for search
  Sync: hotel data changes → event → update ES document (near real-time)

NOT cached: room_inventory.booked_rooms (must always read from DB for booking accuracy)
```

---

## 10. Scale & Trade-offs

| Trade-off | Choice | Reason |
|-----------|--------|--------|
| Consistency vs availability for booking | **CP** (strong consistency) | Overbooking is catastrophic |
| Search storage | Elasticsearch over PostGIS | ES scales horizontally; richer filtering |
| Inventory locking | Optimistic (normal), Pessimistic (sold-out) | Optimistic is faster for normal load |
| Booking confirmation | Sync | User must know immediately |
| Notifications | Async via Kafka | Don't block booking response |
| Photo storage | S3 + CloudFront | Binary blobs, global CDN delivery |

### Bottlenecks & Solutions
```
Search at scale (600 searches/sec):
  → Elasticsearch cluster (10 nodes, 3 shards + 2 replicas each)
  → Redis cache of hot search results (geohash + date combinations)

Booking at peak (Black Friday, holidays):
  → Connection pooling to PostgreSQL (pgBouncer)
  → Horizontal sharding of room_inventory by hotel_id hash
  → Queue bookings during extreme peak (booking request → SQS → worker → DB)

Inventory reads (availability check hits DB on every search):
  → Cache availability summary per hotel per date range in Redis
  → "Hotel X has 3 rooms available for Oct 1-5" (5-min TTL)
  → Booking flow reads DB directly (never cache)
```

---

## Key Talking Points for Interview

1. **Overbooking prevention** is the #1 concern — lead with optimistic locking + idempotency key
2. **Proximity search** — Elasticsearch geo_distance or PostGIS with spatial index
3. **Read/write split** — searches hit ES/Redis; bookings hit primary PostgreSQL
4. **Dynamic pricing** — mention revenue management, occupancy-based multipliers
5. **Sharding strategy** — shard inventory table by hotel_id (range sharding for hotel chains)
6. **CAP choice** — explicitly say booking requires CP, search can tolerate eventual consistency

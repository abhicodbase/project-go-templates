# 25 — Flight Aggregation System

> **Interview context**: Standard system design. Clarifications → FRs → NFRs → Core entities → API → DB choice → HLD → Deep dives.

---

## 1. Requirements Clarification

### Functional Requirements
- Users search flights: origin + destination + date + passengers
- System queries **multiple airline APIs** and aggregates results
- Show unified list sorted by price/duration/stops
- User selects a flight → **redirect to airline website** OR book through platform
- Filter: direct only, price range, airline, departure time
- Price alert: notify user when fare drops for a saved search

### Non-Functional Requirements
```
Scale: 10M searches/day = 115 searches/sec (peak 5×: 600/sec)
Latency: results returned within 3 seconds (airlines are slow!)
Freshness: prices change often → cache max 10-15 minutes
Availability: 99.9%
External dependencies: 50+ airline APIs (slow, rate-limited, unreliable)
```

---

## 2. Core Entities

```sql
-- Airlines (master data)
airlines (airline_id PK, name, iata_code, logo_url, api_config JSONB)

-- Airports (master data)
airports (airport_id PK, iata_code, name, city, country, lat, lng, timezone)

-- Cached flight offers (denormalized search results)
flight_offers (
  offer_id     UUID PRIMARY KEY,
  origin       VARCHAR(3),        ← IATA code
  destination  VARCHAR(3),
  depart_date  DATE,
  airline      VARCHAR(10),
  flight_number VARCHAR(10),
  depart_time  TIMESTAMP,
  arrive_time  TIMESTAMP,
  duration_min INT,
  stops        INT,
  price        DECIMAL(10,2),
  currency     VARCHAR(3),
  seats_left   INT,
  cabin_class  ENUM('ECONOMY','PREMIUM','BUSINESS','FIRST'),
  cached_at    TIMESTAMP,         ← when fetched from airline API
  expires_at   TIMESTAMP          ← now + 15 min
)
-- Indexed: (origin, destination, depart_date, cabin_class, price)

-- User saved searches (price alerts)
saved_searches (
  search_id   UUID PRIMARY KEY,
  user_id     UUID,
  origin, destination, depart_date,
  target_price DECIMAL(10,2),
  notified_at TIMESTAMP
)

-- Bookings (if platform does booking, not redirect)
bookings (booking_id PK, user_id, offer_id, passenger_details JSONB, 
          pnr VARCHAR(20), status, total_price, booked_at)
```

---

## 3. API Design

```
# Search flights
GET /flights/search
  ?origin=BOM&destination=DEL&date=2026-12-01
  &passengers=2&cabin=ECONOMY
  &filters=direct_only,max_price:15000
  &sort=price|duration|depart_time
  Response: [{ airline, flight, depart, arrive, duration, stops, price, deep_link }]

# Price calendar (which day is cheapest this month?)
GET /flights/price-calendar?origin=BOM&destination=DEL&month=2026-12

# Flight detail (refresh price before booking)
GET /flights/{offer_id}/refresh    ← re-fetch from airline API (live price)

# Price alerts
POST /alerts { origin, destination, date, max_price, user_id }
DELETE /alerts/{alert_id}

# Redirect to airline booking
GET /flights/{offer_id}/book → 302 redirect to airline booking page with affiliate link
```

---

## 4. Architecture — The Hard Part: Aggregating Slow Airline APIs

```
┌──────────────────────────────────────────────────────────────────┐
│                     SEARCH FLOW                                    │
│                                                                    │
│  Client ──▶ API Gateway ──▶ Search Service                        │
│                                   │                               │
│               ┌───────────────────▼─────────────────────┐        │
│               │          Cache Layer (Redis)              │        │
│               │  Key: search:BOM:DEL:2026-12-01:ECONOMY  │        │
│               │  TTL: 15 minutes                         │        │
│               └───────────────┬─────────────────────────┘        │
│                               │ MISS                              │
│                               ▼                                   │
│               ┌───────────────────────────────────┐              │
│               │     Aggregation Service            │              │
│               │  Parallel fan-out to airline APIs  │              │
│               │                                   │              │
│               │  goroutine 1: → Air India API     │              │
│               │  goroutine 2: → IndiGo API        │              │
│               │  goroutine 3: → SpiceJet API      │              │
│               │  goroutine 4: → Amadeus GDS       │              │
│               │  (with 3s timeout per airline)    │              │
│               │                                   │              │
│               │  Collect results → merge → sort   │              │
│               │  Store in Redis (15 min cache)    │              │
│               └───────────────────────────────────┘              │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│                     PRICE ALERT PIPELINE                          │
│                                                                    │
│  Price Monitor (cron every 30 min):                               │
│    Fetch saved searches → query airline APIs for each             │
│    Compare new price vs target_price                              │
│    If price <= target: Kafka "price.alert" → Notification Svc     │
│                                              → Email/SMS user     │
└──────────────────────────────────────────────────────────────────┘
```

---

## 5. Airline API Integration — Deep Dive

### Fan-Out with Timeout Pattern
```go
func SearchFlights(origin, dest string, date time.Time) []FlightOffer {
    // Fire all airline API requests in parallel
    results := make(chan []FlightOffer, len(airlines))
    
    for _, airline := range airlines {
        go func(a Airline) {
            ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
            defer cancel()
            
            offers, err := a.FetchFlights(ctx, origin, dest, date)
            if err != nil {
                results <- nil  // airline failed or timed out → skip
                return
            }
            results <- offers
        }(airline)
    }
    
    // Collect results from all airlines (up to 3 second wait)
    var allOffers []FlightOffer
    for i := 0; i < len(airlines); i++ {
        if offers := <-results; offers != nil {
            allOffers = append(allOffers, offers...)
        }
    }
    
    // Sort by price, deduplicate, store in cache
    sort.Slice(allOffers, func(i, j int) bool {
        return allOffers[i].Price < allOffers[j].Price
    })
    return allOffers
}
```

### Rate Limiting per Airline
```
Air India API: 100 requests/min
IndiGo API:    200 requests/min

For each airline: Token bucket in Redis
  If token available: proceed with API call
  If no token: wait for next token (or return stale cached data)

Per-airline circuit breaker:
  If Air India fails 5× in 60 seconds → open circuit
  Skip Air India for next 2 minutes → avoid flooding failed API
  Try again after 2 minutes
```

### GDS (Global Distribution System)
```
Instead of calling each airline individually:
  GDS aggregators: Amadeus, Sabre, Travelport
  Single API call → GDS queries all airlines → returns unified results

Trade-offs:
  GDS Pros: one integration, comprehensive coverage
  GDS Cons: per-query cost ($0.01-0.05), slightly slower, GDS markup
  
  Strategy: Use GDS for long-haul international (comprehensive coverage needed)
            Direct airline APIs for domestic (faster, no GDS markup)
```

---

## 6. Cache Strategy

```
Cache key: flight:search:{origin}:{destination}:{date}:{cabin}:{passengers}
TTL: 15 minutes (prices change fast)

On cache HIT (within 15 min):
  Return cached results immediately (< 50ms)
  Show "prices as of X minutes ago" label to user

On cache MISS:
  Fan-out to airline APIs (3s max)
  Store results in Redis
  Return to user

Cache warming:
  Background worker: every 10 minutes, refresh top 100 most-searched routes
  → Popular routes (BOM-DEL, BLR-DEL) almost always cache HIT

Price refresh before booking:
  User clicks "Book" → always call /flights/{offer_id}/refresh
  → Live price check from airline API (never book stale price)
  → If price changed: show new price with "Price Updated" warning
```

---

## 7. Price Calendar (Fare Trend)

```
"Which day this month is cheapest for BOM to DEL?"

Batch job (runs nightly at 2 AM):
  For each popular route + each day of next 60 days:
    Fetch cheapest fare from airline APIs
    Store in price_calendar table:
      { route: "BOM-DEL", date: "2026-12-01", min_price: 2499, currency: "INR" }

API:
  GET /flights/price-calendar?origin=BOM&destination=DEL&month=2026-12
  → SELECT date, min_price FROM price_calendar
    WHERE origin='BOM' AND destination='DEL' AND date BETWEEN ? AND ?
    ORDER BY date

Response used to render calendar heatmap: cheaper days = green, expensive = red
```

---

## Key Talking Points

1. **Fan-out with timeout** — parallel airline queries, don't wait for slowest; set 3s global deadline
2. **Caching is essential** — 15 min TTL; always show "prices as of X min ago"
3. **Circuit breaker** per airline — if Air India keeps failing, skip it temporarily
4. **GDS vs direct** — mention both; explain trade-offs (coverage vs cost vs speed)
5. **Price refresh before booking** — never book based on cached price; always live-check
6. **Rate limiting per airline** — token bucket in Redis, prevent API abuse
7. **Price calendar** — batch-computed nightly; not computed at query time
8. **Deduplication** — same flight via GDS AND direct airline API → merge using flight_number + date

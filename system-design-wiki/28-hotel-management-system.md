# 28 — Comprehensive Hotel Management System (Marriott/Hilton/Expedia)

> **Interview context**: Complete lifecycle — room inventory, reservations, check-in/out, multi-property chain, pricing, analytics.

---

## 1. Requirements

### Functional
- **Multi-hotel chain**: 1000+ properties, each with 100-500 rooms
- **Room inventory**: real-time availability across all properties
- **Reservations**: book/modify/cancel with pricing rules
- **Check-in/out**: front desk digital process, keycard system
- **Pricing engine**: seasonal rates, loyalty discounts, last-minute deals
- **Analytics**: occupancy rates, revenue per available room (RevPAR), ADR

### Non-Functional
```
Hotels: 1,000 properties (Marriott has 8,500 — scale up progressively)
Rooms: avg 300/hotel → 300,000 total rooms
Reservations: 100,000/day = 1.2/sec (simple, not flash-sale scale)
Concurrent check-ins: morning peaks (500 check-ins/hr)
Availability: 99.99% (front desk must always work)
Consistency: No double-booking — room must be ACID
```

---

## 2. Core Entities

```sql
-- Chain and Properties
hotel_chains (chain_id PK, name, headquarters, loyalty_program_name)

hotels (
  hotel_id    UUID PK, chain_id FK, name, address, city, country,
  lat, lng, star_rating, phone, timezone VARCHAR(50),
  check_in_time TIME DEFAULT '15:00',
  check_out_time TIME DEFAULT '11:00'
)

-- Room Types (template per hotel)
room_types (
  type_id     UUID PK, hotel_id FK,
  name VARCHAR(100),         ← "Deluxe King", "Suite"
  capacity TINYINT,          ← max occupants
  base_price DECIMAL(10,2),
  amenities JSONB,           ← ["King bed", "City view", "Bathtub"]
  floor_plan_url TEXT
)

-- Individual Rooms (physical rooms)
rooms (
  room_id     UUID PK, hotel_id FK, type_id FK,
  room_number VARCHAR(10),   ← "401", "Penthouse-A"
  floor INT, wing VARCHAR(20),
  status ENUM('AVAILABLE','OCCUPIED','MAINTENANCE','HOUSEKEEPING'),
  last_cleaned_at TIMESTAMP,
  next_available_at TIMESTAMP
)

-- Availability Calendar (key for booking)
room_availability (
  hotel_id    UUID, type_id UUID, date DATE,
  total_rooms INT, booked_rooms INT, blocked_rooms INT,
  price DECIMAL(10,2),       ← dynamic price for this date
  PRIMARY KEY (hotel_id, type_id, date)
)

-- Reservations
reservations (
  reservation_id  UUID PK,
  hotel_id UUID FK, type_id UUID FK, room_id UUID,    ← room assigned at check-in
  guest_id UUID FK,
  confirmation_number VARCHAR(20) UNIQUE,              ← "MAR-2026-ABC123"
  check_in DATE, check_out DATE,
  adults TINYINT, children TINYINT,
  status ENUM('RESERVED','CHECKED_IN','CHECKED_OUT','CANCELLED','NO_SHOW'),
  total_price DECIMAL(10,2),
  payment_ref VARCHAR(100),
  special_requests TEXT,
  created_at TIMESTAMP, updated_at TIMESTAMP,
  idempotency_key VARCHAR(64) UNIQUE
)

-- Guests
guests (
  guest_id UUID PK, first_name, last_name, email UNIQUE,
  phone, nationality VARCHAR(2), passport_number,
  loyalty_id VARCHAR(50),                              ← Marriott Bonvoy #
  loyalty_tier ENUM('MEMBER','SILVER','GOLD','PLATINUM'),
  loyalty_points INT DEFAULT 0,
  preferences JSONB                                    ← {"floor": "high", "pillow": "firm"}
)

-- Pricing Rules
pricing_rules (
  rule_id UUID PK, hotel_id FK, type_id FK,
  name VARCHAR(100),                                   ← "Christmas Surge", "Early Bird"
  date_from DATE, date_to DATE,
  price_modifier DECIMAL(5,2),                         ← 1.5 = 50% more
  priority INT,                                        ← higher priority wins
  conditions JSONB                                     ← {"days_before": 30, "loyalty_tier": "GOLD"}
)

-- Check-in Logs (audit trail)
checkin_events (
  event_id UUID PK, reservation_id UUID FK, event_type ENUM('CHECK_IN','CHECK_OUT','ROOM_CHANGE'),
  staff_id UUID, keycard_issued BOOLEAN, room_id UUID, timestamp TIMESTAMP
)
```

---

## 3. API Design

```
# Reservations
POST /reservations              ← book room
GET  /reservations/{id}        ← get reservation
PUT  /reservations/{id}        ← modify (date change, room type upgrade)
DELETE /reservations/{id}      ← cancel

# Availability
GET /hotels/{id}/availability?check_in=2026-12-24&check_out=2026-12-27&adults=2
→ Returns: [{ room_type, available_count, price_per_night, total_price }]

# Front Desk Operations
POST /reservations/{id}/check-in
  { room_id, keycard_id, staff_id }
  → validates guest ID, allocates room, issues digital keycard, updates status

POST /reservations/{id}/check-out
  { charges: [{ description, amount }] }
  → Calculate final bill (room nights + extras), update status, release room

# Room Management
GET  /hotels/{id}/rooms/status  ← housekeeping dashboard (available/occupied/maintenance)
PUT  /rooms/{id}/status { status: "MAINTENANCE", reason: "AC repair" }

# Pricing
GET /hotels/{id}/pricing?check_in=2026-12-24&check_out=2026-12-27&type_id=xxx
→ Returns computed price with applied rules breakdown

# Analytics
GET /hotels/{id}/analytics/occupancy?from=2026-01&to=2026-09
GET /chain/analytics/revpar?period=monthly                   ← chain-level
```

---

## 4. Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                     BOOKING FLOW                                      │
│                                                                        │
│  Guest/OTA ──▶ API Gateway ──▶ Reservation Service                  │
│                                       │                              │
│                          ┌────────────▼──────────────┐              │
│                          │   Pricing Engine            │              │
│                          │   Load pricing rules        │              │
│                          │   Compute final price       │              │
│                          │   Redis cache (5 min TTL)   │              │
│                          └────────────┬──────────────┘              │
│                                       │                              │
│                          ┌────────────▼──────────────┐              │
│                          │   Availability Check        │              │
│                          │   (room_availability table) │              │
│                          │   Optimistic lock + UPSERT  │              │
│                          └────────────┬──────────────┘              │
│                                       │                              │
│                          ┌────────────▼──────────────┐              │
│                          │   PostgreSQL (Primary)      │              │
│                          │   - reservations            │              │
│                          │   - room_availability       │              │
│                          │   - guests                  │              │
│                          └────────────┬──────────────┘              │
└───────────────────────────────────────┼──────────────────────────────┘
                                        │ Events (Kafka)
           ┌───────────────────────────┬┴─────────────────────┐
           ▼                           ▼                       ▼
  Notification Svc           Analytics Service         Loyalty Svc
  (email confirm,             (Redshift / ClickHouse)  (award points)
   SMS check-in reminder)     RevPAR, occupancy         update tier
```

---

## 5. Pricing Engine — Deep Dive

```
Dynamic pricing calculation:

base_price = room_type.base_price              ← $200/night

Active rules (sorted by priority DESC):
  Rule 1: "Christmas Week" (Dec 24-31)     → multiply 2.0x
  Rule 2: "Loyalty Gold Discount"           → multiply 0.9x (10% off)
  Rule 3: "Last Minute Deal" (< 3 days)    → multiply 0.85x

final_price = base_price × 2.0 × 0.9 × 0.85 = $306/night

Occupancy-based dynamic pricing:
  occupancy_rate = booked_rooms / total_rooms for that date
  if occupancy_rate > 0.85: additional_multiplier = 1.3x
  if occupancy_rate < 0.3:  discount_multiplier = 0.8x

Example: Christmas Eve (rule × occupancy): $200 × 2.0 × 1.3 = $520/night

Cache: price per (hotel_id, type_id, date_range, loyalty_tier) — TTL 10 min
Invalidated: on rule changes or major booking events
```

---

## 6. Check-in/out Flow

```
CHECK-IN:
  1. Front desk scans/reads confirmation number
  2. System retrieves reservation → verify status = RESERVED
  3. Find available clean room of correct type:
     SELECT room_id FROM rooms WHERE type_id = ? AND status = 'AVAILABLE'
     AND hotel_id = ? ORDER BY floor ASC LIMIT 1
  4. Assign room: UPDATE rooms SET status = 'OCCUPIED' WHERE room_id = ?
  5. UPDATE reservations SET status = 'CHECKED_IN', room_id = ?
  6. Issue digital keycard:
     → POST /keycard-system/issue { room_id, guest_id, checkout_date }
     → Returns keycard code (sent to guest's phone)
  7. Award loyalty points (async, Kafka event)
  8. Insert checkin_events audit record
  9. Return: { room_number, wifi_password, amenities_info }

CHECK-OUT:
  1. Calculate final charges:
     Room nights × nightly_rate
     + mini-bar consumption (IoT sensor data)
     + restaurant charges
     + parking fees
  2. Process payment (charge stored card)
  3. UPDATE rooms SET status = 'HOUSEKEEPING', last_occupied_by = guest_id
  4. UPDATE reservations SET status = 'CHECKED_OUT'
  5. Trigger housekeeping task (Kafka → housekeeping app → staff notification)
  6. Send folio (receipt) via email
```

---

## 7. Analytics — RevPAR, ADR, Occupancy

```
Key hotel industry metrics:

ADR (Average Daily Rate):
  ADR = Total Room Revenue / Rooms Sold
  Query: SELECT SUM(total_price) / COUNT(*) FROM reservations
         WHERE hotel_id = ? AND status = 'CHECKED_OUT' AND month = '2026-09'

Occupancy Rate:
  OCC = Rooms Occupied / Total Rooms Available
  Query: time-series aggregation on room_availability table

RevPAR (Revenue Per Available Room — most important KPI):
  RevPAR = ADR × Occupancy Rate
  OR: RevPAR = Total Room Revenue / Total Rooms Available

Storage:
  Real-time: queries on PostgreSQL (past 30 days)
  Historical: Amazon Redshift or ClickHouse (data warehouse)
    → Daily ETL: pipe reservations data to data warehouse
    → Enables complex analytics without impacting transactional DB
```

---

## Key Talking Points

1. **Multi-property**: hotel_id on every table; chain-level analytics separate from property-level
2. **Booking = ACID**: room_availability update must be atomic (optimistic locking)
3. **Pricing engine**: layered rules with priority; cache computed prices (not rules themselves)
4. **Check-in**: room assignment at check-in (not booking) — allows flexibility for upgrades
5. **Digital keycard**: IoT integration via keycard API; guest's phone = keycard
6. **Analytics**: don't run aggregations on transactional DB — use Redshift/ClickHouse
7. **RevPAR** — drop this term; shows you know hotel domain
8. **Housekeeping workflow**: async via Kafka after checkout → shows event-driven thinking

# 24 — Event Ticket Booking System (General)

> **Interview context**: Standard ticket booking (concerts, sports, theater). API design, DB choices, bottlenecks, scalability.

---

## 1. Requirements

### Functional
- Browse events (list, search, filter by city/date/category)
- View event detail + seat map + pricing tiers
- Select and hold tickets (time-limited hold)
- Complete payment → receive e-ticket (QR code)
- Cancel booking (refund policy)
- Admin: manage events, seat maps, pricing

### Non-Functional
```
Scale: 10M users, 10K events active, 1M bookings/day
Peak: Major events → 500K users competing for 50K tickets
Hold timeout: 8 minutes
No overselling — ever
P99 hold response: < 300ms
```

---

## 2. DB Schema (Key Tables)

```sql
events (event_id PK, title, venue, date, category, total_capacity, on_sale_at, status)

pricing_tiers (tier_id PK, event_id FK, name, price, total_seats, available_seats)
-- available_seats: atomic decremented on hold, incremented on release

seat_map (seat_id PK, event_id FK, tier_id FK, row, number, status, held_by, held_until)
-- For numbered seating. For GA (General Admission): skip seat_map, just use available_seats

bookings (
  booking_id PK, user_id, event_id, tier_id, seat_ids[],
  quantity INT, total_price, status ENUM(HELD,CONFIRMED,CANCELLED),
  payment_ref, idempotency_key UNIQUE, created_at, expires_at
)

payments (payment_id PK, booking_id FK, amount, provider, status, created_at)
```

---

## 3. Hold Flow (Critical Path)

```
POST /bookings/hold
  { event_id, tier_id, quantity: 2, seats: ["A1", "A2"], idempotency_key }

Server:
  1. Validate: tier exists, event on sale, quantity <= 8 (per-transaction limit)
  2. Check idempotency_key → if exists, return existing booking
  3. BEGIN TRANSACTION
     UPDATE pricing_tiers
     SET available_seats = available_seats - 2
     WHERE tier_id = ? AND available_seats >= 2    ← atomic check + decrement
     (returns rows_affected)
  4. If rows_affected = 0 → ROLLBACK → "Sold out"
  5. If specific seats (numbered):
     UPDATE seat_map SET status='HELD', held_by=user, held_until=NOW()+8min
     WHERE seat_id IN ('A1','A2') AND status='AVAILABLE'
  6. INSERT INTO bookings (status='HELD', expires_at=NOW()+8min)
  7. COMMIT
  8. Return { booking_id, held_until, total_price }
```

---

## 4. Architecture Diagram

```
Client ──▶ CDN (static) + API Gateway
                     │
     ┌───────────────┼───────────────────┐
     │               │                   │
  Event Svc     Hold/Booking Svc    Payment Svc
  (read-heavy)  (CP, ACID)         (Stripe/Razorpay)
     │               │                   │
  Redis Cache    PostgreSQL          Kafka
  (event list,   (seats,            (booking.confirmed
   seat counts)   bookings)          → notifications)
     │
  Elasticsearch
  (event search)

Background:
  Hold Expiry Worker: scans holds expired → release seats
  Notification Worker: Kafka consumer → email QR code
```

---

## 5. Seat Map (Numbered vs General Admission)

```
Numbered seating (Theater, Stadium sections):
  Each seat is a row in seat_map
  Hold: UPDATE seat WHERE status='AVAILABLE' AND seat_id = ?
  → Individual seat-level locking (fine-grained)

GA (General Admission — standing/floor):
  No seat_map rows needed
  Just: pricing_tiers.available_seats counter
  Hold: UPDATE pricing_tiers SET available_seats = available_seats - qty WHERE available_seats >= qty
  → Single counter atomic decrement (simpler, faster)

Best available (system picks seats):
  Don't let user pick → system pops from Redis ZPOPMIN available_seats:{event}:{tier}
  → Atomically removes best (lowest row, lowest seat number) from sorted set
```

---

## 6. Concurrency Patterns

```
Pattern 1: DB Optimistic Locking (our choice for most cases)
  Read available_seats → UPDATE with WHERE available_seats >= qty

Pattern 2: Redis atomic pop (for GA tickets under extreme load)
  DECRBY available:tier:123 2  → if result >= 0: success; if result < 0: undo + fail

Pattern 3: Queue with single writer (for Taylor Swift scale)
  All hold requests → SQS FIFO queue → single consumer → serial processing
  No race conditions; predictable ordering
  Trade-off: higher latency (queue delay)
```

---

## Key Talking Points
1. **Oversell prevention = atomic check + decrement** in DB (not application-level check)
2. **GA vs numbered seating** — different strategies, mention both
3. **Idempotency key** — prevent double charge on payment retry
4. **Hold expiry** — expiry worker or Redis TTL events
5. **Virtual queue** for high-demand events (same as concert design)
6. **E-ticket QR code** — contain booking_id + HMAC signature (unforgeable)

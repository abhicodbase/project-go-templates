# 21 — Concert Ticket Booking (Flash Sale Style)

> **Interview context**: Flash sale ticket system. Tests trade-offs, bottlenecks, scale. "Start small, consider scale as you go."  
> Key focus: preventing oversell, handling traffic spikes 100-1000× normal, fairness.

---

## 1. Requirements Clarification — Ask These in Interview

```
"Before I start designing, let me clarify a few things:"

1. Scale: How many users try to buy on release? (1M? 10M?)
2. Inventory: How many seats per concert? (50K typical arena)
3. Hold time: How long can a user hold a seat before paying? (5 min? 10 min?)
4. Seat selection: Specific seat numbers OR best-available?
5. Queue: Is a virtual waiting queue needed? (Yes for Taylor Swift scale)
6. Payment: Build payment or integrate 3rd party (Stripe)?
7. Consistency requirement: Oversell = catastrophic → CP system
```

---

## 2. Functional Requirements

- Users can **browse** upcoming concerts
- Users can **select** tickets (seat count, section, price tier)
- System **holds** selected tickets for 10 minutes while user pays
- User **completes payment** → ticket confirmed → QR code issued
- If payment incomplete in 10 min → **release hold**
- Virtual **waiting queue** for high-demand releases
- Admin: create concerts, seat maps, pricing tiers, release schedules

## 3. Non-Functional Requirements

```
Scale:
  Normal: 1,000 concurrent users
  Flash sale peak: 5,000,000 concurrent users (5000× spike)
  Ticket inventory: 50,000 seats per concert
  Hold/release cycle: 10 minutes

Constraints:
  NO overselling — ever
  P99 ticket hold response < 500ms under load
  99.99% availability (people WILL try during release)
  Fair queue — first-come-first-served (with queue)
```

---

## 4. Capacity Estimation

```
Concert: 50,000 seats
Flash sale: 5M users trying at T=0 (100:1 demand:supply ratio)

Requests at T=0: 5M clicks in first 1 second = 5,000,000 RPS

This is NOT something you serve directly → need queue

After queue admission (10K users/sec admitted):
  Hold requests: 10,000/sec
  Payment requests: ~3,000/sec (conversion rate ~30%)
  
Storage:
  Events: 1K concerts × 50K seats = 50M seat records (trivial)
  Bookings: 10M bookings/year × 500B = 5 GB (tiny)
```

---

## 5. Core Entities & DB Schema

```sql
-- Events (concerts)
events (
  event_id      UUID PRIMARY KEY,
  title         VARCHAR(200),
  venue         VARCHAR(200),
  event_date    TIMESTAMP,
  total_seats   INT,
  on_sale_at    TIMESTAMP,       ← when tickets go on sale
  status        ENUM('UPCOMING','ON_SALE','SOLD_OUT','COMPLETED')
)

-- Seats (individual seat inventory)
seats (
  seat_id       UUID PRIMARY KEY,
  event_id      UUID REFERENCES events,
  section       VARCHAR(20),     ← "Floor", "Section A", "GA"
  row_number    VARCHAR(5),
  seat_number   INT,
  price_tier    VARCHAR(20),     ← "general", "VIP", "platinum"
  price         DECIMAL(10,2),
  status        ENUM('AVAILABLE','HELD','BOOKED') DEFAULT 'AVAILABLE',
  held_by       UUID,            ← user_id holding this seat
  held_until    TIMESTAMP,       ← hold expiry time
  version       INT DEFAULT 0    ← optimistic locking version
)
-- KEY: index on (event_id, status, price_tier)

-- Bookings
bookings (
  booking_id    UUID PRIMARY KEY,
  user_id       UUID,
  event_id      UUID,
  seat_ids      UUID[],
  total_price   DECIMAL(10,2),
  status        ENUM('HELD','PAYMENT_PENDING','CONFIRMED','CANCELLED'),
  payment_ref   VARCHAR(100),
  idempotency_key VARCHAR(64) UNIQUE,
  created_at    TIMESTAMP,
  expires_at    TIMESTAMP        ← hold expiry (now + 10 min)
)

-- Queue positions (for virtual waiting room)
queue_entries (
  queue_id      UUID PRIMARY KEY,
  event_id      UUID,
  user_id       UUID,
  joined_at     TIMESTAMP,
  token         VARCHAR(64) UNIQUE,  ← given to user to enter sale
  admitted_at   TIMESTAMP,
  status        ENUM('WAITING','ADMITTED','EXPIRED')
)
```

---

## 6. API Design

```
# Browse
GET /events                              ← list upcoming
GET /events/{event_id}                  ← event detail + seat availability
GET /events/{event_id}/seats            ← seat map (available/held/booked)
  ?section=Floor&tier=general

# Queue (for flash sale)
POST /events/{event_id}/queue/join      ← join waiting room
  Response: { queue_id, position: 54321, estimated_wait: "8 min" }
GET  /events/{event_id}/queue/status    ← poll position
  Response: { position: 1234, admitted: false }
              OR { admitted: true, access_token: "abc123", expires_in: 600 }

# Ticket Hold (requires queue access_token)
POST /bookings/hold
  Authorization: Bearer {access_token}
  { event_id, seat_ids: ["seat-1", "seat-2"], idempotency_key }
  Response: { booking_id, held_until: "2026-10-01T10:10:00Z", total_price: 250.00 }

# Complete Payment
POST /bookings/{booking_id}/pay
  { payment_method_id: "pm_stripe_xxx" }
  Response: { status: "CONFIRMED", ticket_url: "https://...", qr_code: "..." }

# Cancel/Release Hold
DELETE /bookings/{booking_id}           ← user cancels → seats released
```

---

## 7. High-Level Architecture

```
                          ┌────────────────────┐
5M Users ──────────────▶  │  CDN + DDoS Protect │
at T=0 (on sale time)     │  (Cloudflare WAF)   │
                          └─────────┬───────────┘
                                    │
                          ┌─────────▼───────────┐
                          │  Virtual Queue       │
                          │  (Redis + Queue SVC) │
                          │  Admits N users/sec  │
                          └─────────┬───────────┘
                    (Admitted users only — 10K/sec)
                                    │
                          ┌─────────▼───────────┐
                          │  API Gateway / LB    │
                          └────────┬────────────┘
                                   │
             ┌─────────────────────┼─────────────────────┐
             │                     │                     │
    ┌────────▼────────┐  ┌─────────▼───────┐  ┌────────▼────────┐
    │  Event Service  │  │  Seat Hold Svc  │  │ Payment Service │
    │  (read-heavy)   │  │  (CP, critical) │  │  (Stripe)       │
    └────────┬────────┘  └─────────┬───────┘  └────────┬────────┘
             │                     │                     │
    ┌────────▼────────┐  ┌─────────▼───────┐  ┌────────▼────────┐
    │  Redis (cache)  │  │  PostgreSQL      │  │  Kafka          │
    │  event metadata │  │  seats + bookings│  │  confirmation   │
    │  seat counts    │  │  (ACID)          │  │  events         │
    └─────────────────┘  └─────────────────┘  └─────────────────┘
    
                         Background:
                         ┌──────────────────────────────┐
                         │  Hold Expiry Worker           │
                         │  Scans: holds where           │
                         │  expires_at < NOW()           │
                         │  → UPDATE seats SET AVAILABLE │
                         │  → UPDATE bookings CANCELLED  │
                         └──────────────────────────────┘
```

---

## 8. Virtual Waiting Queue — Deep Dive (Taylor Swift Problem)

```
5M users hit "Buy Now" at 10:00:00 AM exactly.
Without queue: 5M concurrent DB requests → system crashes.
With queue: controlled admission.

Architecture:
  T=09:55: Users join waiting room
    POST /queue/join → "You are position 54,321"
    Redis: RPUSH queue:event_123 {user_id + timestamp}
  
  T=10:00: Sale opens
    Queue Admission Service: pops N users/second from queue
    N = system_capacity × 0.7 (leave 30% headroom)
    
    For each admitted user:
      Generate access_token (short-lived JWT, 10 min TTL)
      Redis SET: token:{access_token} → user_id (10 min TTL)
      → Notify user via WebSocket/Push: "You're admitted! Buy now."
  
  T=10:00 to T+N min: Admitted users buy tickets
    access_token validated on every request
    Token gates Seat Hold endpoint (no token = 403)

  Queue fairness:
    Within same second → randomize order (not alphabetical)
    No re-queuing: if token expires without buying, go back to end of queue
```

---

## 9. Seat Hold — Preventing Oversell (Critical!)

```
Race condition:
  User A and User B both try to hold Seat #1 simultaneously.
  Both check: status = AVAILABLE
  Both try to update → one must fail.

Solution: Optimistic Locking with CAS

  Step 1: READ seat
    SELECT seat_id, status, version FROM seats WHERE seat_id = ? AND status = 'AVAILABLE'
  
  Step 2: Atomically update with version check
    UPDATE seats
    SET status = 'HELD', held_by = {user_id}, held_until = NOW() + INTERVAL '10 min', version = version + 1
    WHERE seat_id = ? AND status = 'AVAILABLE' AND version = {read_version}
  
  If 0 rows updated → someone else got it → return "Seat taken, try another"
  If 1 row updated → success → create booking record

Alternative: Redis-based distributed lock
  SET seat:lock:{seat_id} {user_id} NX EX 1   ← 1-second lock
  Only winner proceeds to DB update
  Reduces DB load from concurrent lock contention

For best-available (no specific seat):
  Redis Sorted Set: ZPOPMIN available_seats:event_123:{tier} 2  ← atomically pop 2 seats
  O(1) atomic operation — no race condition
```

---

## 10. Hold Expiry & Seat Release

```
Problem: User holds seats then abandons checkout → seats locked for 10 min

Solution 1: Background sweeper (simple)
  Runs every 30 seconds:
    SELECT seat_id FROM seats WHERE status = 'HELD' AND held_until < NOW()
    FOR EACH expired_seat:
      UPDATE seats SET status = 'AVAILABLE', held_by = NULL, held_until = NULL
      UPDATE bookings SET status = 'CANCELLED' WHERE booking_id = ...

Solution 2: Redis TTL + event (better)
  On seat hold: SET seat:held:{seat_id} {booking_id} EX 600
  When TTL expires → Redis key expiry event → subscribe via Keyspace Notifications
  → Trigger seat release in DB
  
  More reactive (no sweeper polling delay), but requires Redis keyspace notifications enabled
```

---

## 11. Trade-offs & Bottlenecks (What Interviewers Want to Hear)

| Bottleneck | Solution | Trade-off |
|------------|---------|-----------|
| 5M users at T=0 | Virtual queue (Redis) | Fairness vs. first-come-first-served |
| Seat inventory reads | Redis cache of available counts | Slight stale count is OK for display |
| Seat hold writes | Optimistic locking in PostgreSQL | Retry overhead for high contention |
| Hold expiry | Background sweeper or Redis TTL events | Sweeper: up to 30s delay in release |
| Payment timeout | Booking expires → seats auto-released | User loses seats if slow payment |
| DB writes at scale | Connection pooling (pgBouncer), read replicas | Read replicas for event browsing only |

### Scale Progression (Tell This Story)
```
Stage 1 (10K users):  Single PostgreSQL, no queue needed
Stage 2 (100K users): Add Redis cache for seat counts, read replicas
Stage 3 (1M users):   Add virtual queue, connection pooling
Stage 4 (10M+ users): Queue is mandatory, Redis for seat pop (ZPOPMIN),
                       DB sharded by event_id, multiple regions
```

---

## Key Talking Points

1. **Queue is non-negotiable** for flash sales — lead with this
2. **Oversell prevention**: optimistic locking with CAS, not application-level checks
3. **Hold expiry**: mention both approaches and their trade-offs
4. **Idempotency key**: prevent double booking on payment retry
5. **Read vs write split**: seat counts/event info from cache; holds/bookings to DB
6. **Start small**: single region → multi-region; single DB → sharded by event_id
7. **Trade-off**: strong consistency for inventory (CP) vs high availability for browsing (AP)

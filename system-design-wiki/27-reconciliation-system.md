# 27 — Reconciliation System (Agoda — External Platform API)

> **Interview context**: Agoda provides hotel booking APIs to external platforms. Data stored at both Agoda and external platform. Design a reconciliation system to detect and resolve discrepancies.

---

## 1. Problem Statement

```
Agoda exposes booking APIs to external partners (OTAs, travel agents).
Partner calls: POST /bookings → Agoda creates booking, returns booking_id.

Data stored at:
  Agoda side:    bookings table (booking_id, status, amount, hotel, dates...)
  Partner side:  their own DB (partner_booking_id, agoda_booking_id, status, amount...)

Discrepancies happen:
  Network timeout: Partner sent request, Agoda processed it, ACK never received
  → Partner thinks: booking FAILED; Agoda thinks: booking CONFIRMED

  Double charge: Partner retried, Agoda created 2 bookings
  → Partner thinks: 1 booking; Agoda has 2 bookings (one needs refund)

  Status drift: Booking cancelled at Agoda (hotel request), partner not notified
  → Agoda status: CANCELLED; Partner status: CONFIRMED (guest shows up with wrong expectation)

Goal: Detect + reconcile these discrepancies automatically
```

---

## 2. Types of Discrepancies

```
Type 1: Missing at Agoda (partner has booking, Agoda doesn't)
  → Create booking at Agoda OR inform partner it was never created

Type 2: Missing at Partner (Agoda has booking, partner doesn't)
  → Usually: timeout scenario; Agoda auto-cancel if no partner ACK within N hours

Type 3: Status mismatch
  Agoda: CONFIRMED | Partner: CANCELLED → Agoda must honour cancellation
  Agoda: CANCELLED | Partner: CONFIRMED → Partner must be notified immediately

Type 4: Amount mismatch
  Agoda charges $150 | Partner recorded $140 → $10 discrepancy → financial reconciliation

Type 5: Duplicate booking
  Partner retried, Agoda created 2 bookings for same stay → cancel duplicate, refund
```

---

## 3. Core Entities

```sql
-- Agoda-side bookings (source of truth)
bookings (
  booking_id      UUID PRIMARY KEY,
  partner_id      VARCHAR(50),          ← which external partner
  partner_booking_id VARCHAR(100),      ← partner's own reference
  hotel_id        BIGINT,
  guest_name      VARCHAR(200),
  check_in        DATE,
  check_out       DATE,
  amount          DECIMAL(10,2),
  currency        VARCHAR(3),
  status          ENUM('CONFIRMED','CANCELLED','MODIFIED','NO_SHOW'),
  created_at      TIMESTAMP,
  updated_at      TIMESTAMP,
  last_synced_at  TIMESTAMP             ← when last sent to partner
)

-- Reconciliation records (per reconciliation run)
reconciliation_runs (
  run_id          UUID PRIMARY KEY,
  partner_id      VARCHAR(50),
  period_start    TIMESTAMP,
  period_end      TIMESTAMP,
  status          ENUM('RUNNING','COMPLETED','FAILED'),
  total_records   INT,
  matched         INT,
  discrepancies   INT,
  started_at      TIMESTAMP,
  completed_at    TIMESTAMP
)

-- Discrepancy records (action required)
discrepancies (
  discrepancy_id  UUID PRIMARY KEY,
  run_id          UUID,
  booking_id      UUID,
  partner_booking_id VARCHAR(100),
  type            ENUM('MISSING_AGODA','MISSING_PARTNER','STATUS_MISMATCH',
                       'AMOUNT_MISMATCH','DUPLICATE'),
  agoda_value     TEXT,                 ← JSON of Agoda's data
  partner_value   TEXT,                 ← JSON of partner's data
  resolution      ENUM('AUTO_FIXED','MANUAL_REVIEW','PARTNER_NOTIFIED'),
  resolved_at     TIMESTAMP,
  notes           TEXT
)
```

---

## 4. Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                     DATA EXPORT PHASE                                 │
│                                                                        │
│  Agoda DB ──▶ Reconciliation Service:                                │
│    SELECT booking_id, partner_booking_id, status, amount, updated_at │
│    WHERE partner_id = ? AND period BETWEEN ? AND ?                    │
│    → Export as CSV/JSON → upload to S3 (shared storage)              │
│                                                                        │
│  Partner system → uploads their booking snapshot (via API or S3)     │
│    { partner_booking_id, agoda_booking_id, status, amount }[]        │
└──────────────────────────────────┬───────────────────────────────────┘
                                   │
┌──────────────────────────────────▼───────────────────────────────────┐
│                     COMPARISON PHASE                                  │
│                                                                        │
│  Reconciliation Engine:                                               │
│    Load Agoda data → HashMap<agoda_booking_id, BookingRecord>         │
│    Load Partner data → HashMap<agoda_booking_id, PartnerRecord>       │
│                                                                        │
│    For each Agoda booking:                                            │
│      partner_record = partner_map.get(booking_id)                    │
│      if partner_record == null:     → Type 2: Missing at Partner      │
│      elif status mismatch:          → Type 3: Status Mismatch         │
│      elif amount difference > $0.01: → Type 4: Amount Mismatch       │
│      else:                          → Matched ✓                       │
│                                                                        │
│    For each Partner booking not in Agoda map:                         │
│      → Type 1: Missing at Agoda                                       │
│                                                                        │
│    Write discrepancies → discrepancies table                          │
└──────────────────────────────────┬───────────────────────────────────┘
                                   │
┌──────────────────────────────────▼───────────────────────────────────┐
│                     RESOLUTION PHASE                                  │
│                                                                        │
│  Auto-resolution (safe cases):                                        │
│    Status mismatch (Agoda=CANCELLED, Partner=CONFIRMED):              │
│      → Call Partner API: notify cancellation                          │
│      → Mark discrepancy resolved                                      │
│                                                                        │
│    Missing at Partner (timeout scenario):                             │
│      → Resend booking confirmation to Partner via webhook             │
│                                                                        │
│    Amount mismatch < $5:                                              │
│      → Log as low-priority, batch report to finance team             │
│                                                                        │
│  Manual review queue:                                                 │
│    Large amount mismatches, duplicate bookings, complex cases        │
│    → Slack alert + dashboard for ops team                            │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 5. When to Run Reconciliation

```
Frequency options:

Real-time reconciliation:
  On every booking event (status change at Agoda):
    Immediately notify partner via webhook
    If webhook fails → retry queue (SQS with exponential backoff)
    If all retries fail → flag for reconciliation

  Best for: immediate state synchronization
  Problem: partner's system must be always-on (what if partner is down?)

Near-real-time (event-driven):
  Agoda booking event → Kafka → Reconciliation Consumer
  Consumer: compare Agoda state with partner's last-known state
  If mismatch → notify partner API → record resolution

Scheduled batch (T+24h reconciliation):
  Daily job at 2 AM:
    Compare yesterday's bookings at Agoda vs partner data dump
    Catches any events that slipped through real-time

Production approach: BOTH
  Real-time notifications for immediate sync (best effort)
  Daily batch reconciliation as safety net (catch all misses)
```

---

## 6. Idempotency — Critical for Reconciliation

```
Reconciliation system must be idempotent:
  If the same reconciliation run is triggered twice → same result, no duplicate fixes

Implementation:
  Each reconciliation run has unique run_id
  Before processing: check if run_id already in reconciliation_runs
  If exists AND status=COMPLETED → skip (already done)
  If exists AND status=RUNNING → another worker is handling it → skip

  Resolution actions also idempotent:
    Partner notification: include booking_id + action in request
    Partner deduplicates on their side using booking_id
```

---

## 7. Car Dealer Reconciliation (Same Pattern)

> *(This addresses the separate question: "Reconciliation system of a car dealer showroom")*

```
Same problem, different domain:

Car Dealer: sells cars through Agoda-like platform.
Data at: Dealer's system (inventory, sales) + Platform's system (orders, payments)

Discrepancy types:
  Car sold by dealer but not recorded on platform → revenue leak
  Platform shows payment received, dealer's POS shows declined → financial mismatch
  Car inventory: dealer shows 2 units, platform shows 3 available → oversell risk

Architecture (identical to above):
  Daily export: Dealer POS system → CSV → S3
  Daily export: Platform → order data → S3
  Reconciliation Engine compares:
    Match: order_id from platform ↔ dealer's sale_id
    Compare: amount, car_model, VIN number, sale_date
  Discrepancies → alerts to dealer + platform finance team

Key additions for dealer:
  VIN (Vehicle ID Number) as unique identifier (like booking_id)
  Amount must match exactly (car prices = large amounts, no tolerance)
  Inventory sync: recon run also syncs available cars to platform
```

---

## Key Talking Points

1. **Two data sources** — Agoda DB + Partner dump; reconciliation = diff between them
2. **HashMap comparison** — O(n) comparison for n records; much faster than nested loops O(n²)
3. **Real-time + batch** hybrid — webhooks for immediate sync; batch as safety net
4. **Idempotency** — reconciliation runs must be safe to re-run
5. **Auto-resolve vs manual queue** — define clear rules for what's auto-safe vs needs human
6. **S3 as exchange** — partner uploads their data to shared S3 bucket (standard B2B pattern)
7. **Discrepancy taxonomy** — clearly name types (missing, status mismatch, amount mismatch) — shows structured thinking
8. **Financial reconciliation** — mention currency, exchange rates, tolerance thresholds (< $1 auto-resolve)

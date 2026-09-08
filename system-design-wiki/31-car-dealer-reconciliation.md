# 31 — Car Dealer Reconciliation System

> **Interview context**: Design a reconciliation system for a car dealer showroom. Think like a tutor. Detailed steps. Visual diagrams included.

---

## 1. Understanding the Problem (Tutor Explanation)

> Think of it this way: A car dealer has TWO record books.  
> **Book 1**: Their own POS (Point of Sale) system — every car sold, every payment received.  
> **Book 2**: The platform/manufacturer's system — what they see from their side.  
>
> At the end of each day, these two books should **match exactly**. If they don't — that's a discrepancy. The reconciliation system finds and fixes those differences.

### Real-World Discrepancies in a Car Dealership

```
┌──────────────────────────────────────────────────────────────────────┐
│  Discrepancy Type     │  Example                                      │
├──────────────────────────────────────────────────────────────────────┤
│ Sale recorded at      │ Dealer sold Car VIN-1234, customer paid.      │
│ dealer but not        │ Platform system didn't receive the sale record │
│ at platform           │ (network failure during API call)             │
├──────────────────────────────────────────────────────────────────────┤
│ Amount mismatch       │ Dealer POS: ₹12,00,000 | Platform: ₹11,50,000│
│                       │ (discount applied on one side, not the other) │
├──────────────────────────────────────────────────────────────────────┤
│ Status mismatch       │ Platform: Sale cancelled | Dealer POS: Active │
│                       │ (cancellation processed by HQ, not at dealer) │
├──────────────────────────────────────────────────────────────────────┤
│ Duplicate entry       │ Dealer retried API → platform recorded 2 sales│
│                       │ for same VIN                                  │
├──────────────────────────────────────────────────────────────────────┤
│ Inventory mismatch    │ Platform: 5 cars available | Dealer: 3 in stock│
│                       │ (2 cars in transit not reflected at dealer)   │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 2. Requirements

### Functional
- Collect transaction data from **dealer POS** and **platform system** for a given period
- **Compare** the two datasets: find matches, find mismatches
- **Classify** discrepancies: missing record, amount diff, status diff, duplicate
- **Auto-resolve** safe discrepancies; flag complex ones for **manual review**
- Generate **reconciliation report** (human-readable + machine-readable)
- **Track history** of all reconciliation runs + resolutions

### Non-Functional
```
Dealers: 500 showrooms
Sales per dealer per day: avg 20 cars = 10,000 sales/day total
Monthly reconciliation volume: 300,000 records
Run frequency: daily (overnight), with ad-hoc on-demand runs
Report ready within: 30 minutes of run start
Discrepancy alert: notify finance team within 5 minutes of finding issues
```

---

## 3. Core Entities (DB Schema)

```sql
-- Dealers (master data)
dealers (
  dealer_id   VARCHAR(20) PRIMARY KEY,  ← "DLR-MUM-001"
  name        VARCHAR(200),
  city        VARCHAR(100),
  state       VARCHAR(50),
  contact_email VARCHAR(200),
  platform    ENUM('MARUTI', 'HYUNDAI', 'TATA', 'CUSTOM')  ← which manufacturer platform
)

-- Dealer transactions (fetched from dealer POS)
dealer_transactions (
  txn_id          VARCHAR(50) PRIMARY KEY,  ← dealer's local transaction ID
  dealer_id       VARCHAR(20),
  vin             VARCHAR(17) UNIQUE,        ← Vehicle Identification Number (unique per car!)
  customer_name   VARCHAR(200),
  sale_date       DATE,
  sale_amount     DECIMAL(12,2),
  discount_amount DECIMAL(12,2),
  net_amount      DECIMAL(12,2),
  payment_mode    ENUM('CASH','LOAN','EXCHANGE'),
  status          ENUM('BOOKED','DELIVERED','CANCELLED'),
  invoice_number  VARCHAR(50),
  fetched_at      TIMESTAMP
)

-- Platform transactions (fetched from manufacturer/platform API)
platform_transactions (
  platform_txn_id VARCHAR(50) PRIMARY KEY,
  dealer_id       VARCHAR(20),
  vin             VARCHAR(17),
  sale_date       DATE,
  sale_amount     DECIMAL(12,2),
  status          ENUM('ACTIVE','CANCELLED','RETURNED'),
  dealer_ref      VARCHAR(50),              ← dealer's txn_id (if sent)
  fetched_at      TIMESTAMP
)

-- Reconciliation runs
recon_runs (
  run_id       UUID PRIMARY KEY,
  dealer_id    VARCHAR(20),
  period_start DATE,
  period_end   DATE,
  status       ENUM('QUEUED','RUNNING','COMPLETED','FAILED'),
  total_dealer_records     INT,
  total_platform_records   INT,
  matched_count            INT,
  discrepancy_count        INT,
  auto_resolved_count      INT,
  manual_review_count      INT,
  triggered_by   ENUM('SCHEDULED','MANUAL','API'),
  started_at     TIMESTAMP,
  completed_at   TIMESTAMP
)

-- Discrepancies found
discrepancies (
  discrepancy_id UUID PRIMARY KEY,
  run_id         UUID REFERENCES recon_runs,
  dealer_id      VARCHAR(20),
  vin            VARCHAR(17),
  type           ENUM('MISSING_AT_PLATFORM','MISSING_AT_DEALER',
                      'AMOUNT_MISMATCH','STATUS_MISMATCH','DUPLICATE'),
  dealer_amount  DECIMAL(12,2),
  platform_amount DECIMAL(12,2),
  amount_diff    DECIMAL(12,2),
  dealer_status  VARCHAR(20),
  platform_status VARCHAR(20),
  resolution     ENUM('AUTO_FIXED','MANUAL_REVIEW','PENDING','DISMISSED'),
  resolution_notes TEXT,
  resolved_by    VARCHAR(100),
  resolved_at    TIMESTAMP,
  created_at     TIMESTAMP
)
```

---

## 4. Complete Architecture — Step by Step

```
STEP 1: Data Collection
                    ┌──────────────────────────────────────────┐
                    │         DATA SOURCES                      │
                    │                                          │
                    │  Dealer POS System        Platform API  │
                    │  (MySQL database)         (REST API)    │
                    └──────────────────────────────────────────┘
                                   │
                    ┌──────────────▼──────────────────────────┐
                    │         DATA COLLECTORS                  │
                    │                                         │
                    │  Dealer Collector:                      │
                    │  SELECT * FROM sales                    │
                    │  WHERE sale_date BETWEEN ? AND ?        │
                    │  → Load into dealer_transactions table  │
                    │                                         │
                    │  Platform Collector:                    │
                    │  GET /dealer/{id}/sales?from=?&to=?     │
                    │  → Parse JSON → platform_transactions   │
                    └─────────────────────────────────────────┘

STEP 2: Reconciliation Engine
                    ┌──────────────────────────────────────────┐
                    │       RECONCILIATION ENGINE               │
                    │                                          │
                    │  Input:                                  │
                    │    Map A: { vin → DealerTransaction }   │
                    │    Map B: { vin → PlatformTransaction }  │
                    │                                          │
                    │  Algorithm:                              │
                    │    UNION(A.keys, B.keys) → all VINs     │
                    │                                          │
                    │    For each VIN:                         │
                    │      a = Map A.get(vin)                  │
                    │      b = Map B.get(vin)                  │
                    │                                          │
                    │      if a && b:                          │
                    │        compare(a, b) → MATCH or MISMATCH│
                    │      elif a && !b:                       │
                    │        → MISSING_AT_PLATFORM            │
                    │      elif !a && b:                       │
                    │        → MISSING_AT_DEALER              │
                    └─────────────────────────────────────────┘

STEP 3: Resolution
                    ┌──────────────────────────────────────────┐
                    │        AUTO-RESOLUTION ENGINE             │
                    │                                          │
                    │  Rule 1: Amount diff < ₹100             │
                    │    → AUTO-RESOLVE (rounding differences) │
                    │                                          │
                    │  Rule 2: MISSING_AT_PLATFORM             │
                    │    + dealer has invoice number           │
                    │    → Resend to platform API              │
                    │    → Mark AUTO_FIXED                     │
                    │                                          │
                    │  Rule 3: STATUS_MISMATCH                 │
                    │    dealer=CANCELLED, platform=ACTIVE      │
                    │    → Cancel on platform API              │
                    │    → Mark AUTO_FIXED                     │
                    │                                          │
                    │  Rule 4: Amount diff > ₹10,000           │
                    │    → MANUAL_REVIEW                       │
                    │    → Alert finance team                  │
                    │    → Dashboard task created              │
                    └─────────────────────────────────────────┘

STEP 4: Reporting
                    ┌──────────────────────────────────────────┐
                    │        REPORT GENERATION                  │
                    │                                          │
                    │  Summary Report (email to dealer manager)│
                    │    Total Sales: 25                       │
                    │    Matched: 23 (92%)                     │
                    │    Discrepancies: 2                      │
                    │      - VIN-1234: Amount mismatch ₹5,000 │
                    │      - VIN-5678: Missing at platform     │
                    │                                          │
                    │  Detail Report (CSV, downloadable)       │
                    │    All discrepancies with dealer+platform│
                    │    values, actions taken, resolution     │
                    └─────────────────────────────────────────┘
```

---

## 5. Data Collection Strategies

### Option A: API Pull (Polling)
```
Reconciliation Service → API call every day at 11 PM:
  GET dealer_pos_api.com/sales?date=2026-09-07
  GET platform_api.com/dealer/DLR-MUM-001/sales?date=2026-09-07

Pros: simple, works for any system
Cons: depends on both systems being available at recon time
     Large payload (fetch all day's data in one shot)
```

### Option B: CDC (Change Data Capture) — More Robust
```
Dealer POS DB → Debezium reads MySQL binlog →
  Every insert/update → Kafka "dealer.sales.events" topic
  
Platform API → Webhook pushed on every state change →
  Kafka "platform.sales.events" topic

Both streams stored in: recon_events table (append-only)
Recon run: read from recon_events for the period → compare

Pros: real-time; not dependent on both being up simultaneously
Cons: requires Debezium setup at dealer (may not always be possible)
```

### Option C: File Exchange (Legacy Dealers)
```
Many dealers still use Excel/CSV from POS:
  Daily at 11 PM: dealer uploads sales_2026_09_07.csv to shared SFTP
  Recon service polls SFTP → downloads file → parses → loads to DB

This is the most common pattern for SME dealers.
```

---

## 6. VIN as the Match Key (Critical Design Decision)

```
Why VIN? (Vehicle Identification Number)
  17-character globally unique identifier for every car
  Assigned by manufacturer — same VIN everywhere
  Format: WBA3A5C51FF358239 (World Manufacturer + Vehicle + Serial)

What NOT to use as match key:
  ❌ Invoice number: dealer generates this → could be different across systems
  ❌ Customer name: typos, formatting differences
  ❌ Sale date + amount: two Civics sold same day for same price → false match
  ❌ Dealer transaction ID: only exists in dealer system

VIN is the universal "foreign key" between dealer and platform.
Both systems always have VIN (it's printed on the car).
```

---

## 7. Handling Duplicates

```
Scenario: Dealer retried API call → Platform recorded sale twice

Detection:
  Platform dataset: VIN-1234 appears TWICE with different platform_txn_ids
  → Flag as DUPLICATE before reconciliation starts
  
  Query: SELECT vin, COUNT(*) FROM platform_transactions
         WHERE run_id = ? GROUP BY vin HAVING COUNT(*) > 1

Resolution:
  1. Find which one matches dealer's invoice_number
  2. Keep that one → cancel/void the other via platform API
  3. Log both as DUPLICATE discrepancy
  4. Alert finance: "platform has duplicate charge for VIN-1234, ₹1,20,000 refunded"
```

---

## 8. Reconciliation Algorithm (Code)

```python
def reconcile(dealer_txns: list, platform_txns: list) -> ReconReport:
    # Build lookup maps using VIN as key
    dealer_map  = {t.vin: t for t in dealer_txns}
    platform_map = {t.vin: t for t in platform_txns}
    
    all_vins = set(dealer_map.keys()) | set(platform_map.keys())
    
    matches, discrepancies = [], []
    
    for vin in all_vins:
        d = dealer_map.get(vin)    # dealer record (may be None)
        p = platform_map.get(vin)  # platform record (may be None)
        
        if d and p:
            # Both have it — compare fields
            amount_diff = abs(d.net_amount - p.sale_amount)
            
            if amount_diff <= 100 and d.status_matches(p.status):
                matches.append(vin)  # MATCHED ✓
            else:
                discrepancies.append(Discrepancy(
                    vin=vin,
                    type='AMOUNT_MISMATCH' if amount_diff > 100 else 'STATUS_MISMATCH',
                    dealer_amount=d.net_amount,
                    platform_amount=p.sale_amount,
                    amount_diff=amount_diff
                ))
        
        elif d and not p:
            discrepancies.append(Discrepancy(vin=vin, type='MISSING_AT_PLATFORM', dealer=d))
        
        elif p and not d:
            discrepancies.append(Discrepancy(vin=vin, type='MISSING_AT_DEALER', platform=p))
    
    return ReconReport(
        matched=len(matches),
        discrepancies=discrepancies,
        match_rate=len(matches)/len(all_vins)*100
    )
    
# Time complexity: O(n + m) — one pass through each dataset
# Space complexity: O(n + m) — two HashMaps
```

---

## 9. Scale Considerations

```
10,000 sales/day × 30 days = 300,000 records/month

For 300K records:
  Processing time: < 5 seconds (HashMap O(n) is fast)
  Storage: 300K × 500 bytes = 150 MB/month → tiny

If you scale to 1M dealers with 100 sales/day = 100M records/month:
  → Distribute by dealer_id: each dealer's reconciliation is independent
  → Run 1M dealer reconciliations in parallel (each takes < 1 sec)
  → Use a job queue (SQS): 1M jobs → processed by 100 workers = 10,000 jobs/worker
  → Total time: 100M / (100 workers × 100K records/worker/sec) = 10 seconds
```

---

## 10. Report & Alerting

```
Reconciliation Report (sent after each run):

  ═══════════════════════════════════════════════════
  RECONCILIATION REPORT — DLR-MUM-001
  Period: 2026-09-07  |  Run: 2026-09-08 00:30 AM
  ═══════════════════════════════════════════════════
  
  Summary:
    Dealer Records:    25
    Platform Records:  25
    Matched:           23 (92%) ✓
    Discrepancies:      2 (8%)  ⚠
    Auto-Resolved:      1
    Manual Review:      1
  
  Discrepancies Found:
  ─────────────────────────────────────────────────
  1. VIN: WBA3A5C51FF123456 | Type: AMOUNT_MISMATCH
     Dealer: ₹12,00,000  |  Platform: ₹11,50,000
     Difference: ₹50,000
     Action: MANUAL_REVIEW → Finance team notified
     
  2. VIN: WBA3A5C51FF654321 | Type: MISSING_AT_PLATFORM
     Dealer Invoice: INV-2026-0001
     Action: AUTO_FIXED → Resent to platform at 00:35 AM
             Platform confirmed: P-TXN-98765
  ═══════════════════════════════════════════════════

Alerts:
  Slack channel #finance-alerts: "VIN-123 amount mismatch ₹50K at DLR-MUM-001 needs review"
  Email to dealer manager: full report PDF attached
```

---

## Key Talking Points (What Interviewers Want)

1. **VIN as match key** — explain why; shows domain knowledge + uniqueness thinking
2. **HashMap-based O(n) comparison** — don't compare records nested-loop O(n²)
3. **Three data collection strategies** — API pull, CDC, file exchange — cover all cases
4. **Auto-resolve rules** — define clear thresholds; small diffs auto-fix, large diffs = human
5. **Idempotent runs** — running same recon twice = same result, no duplicate fixes
6. **Duplicate detection** — check for same VIN twice in platform data before main recon
7. **Report format** — mention summary + detail; non-technical dealer manager reads summary
8. **Scale**: 10K sales/day is trivial; demonstrate you can scale to 100M with partition-by-dealer

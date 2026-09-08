# 30 — Payment Gateway System + Kafka Exactly-Once

> **Interview context**: Design a payment gateway that scales with company growth. Build a feature on top of Kafka to process messages **exactly once** (older Kafka versions guarantee at-least-once only).

---

## 1. Requirements

### Functional
- Accept payment from clients (card, UPI, net banking, wallets)
- Route to appropriate payment processor (Stripe, Razorpay, bank networks)
- Handle async results (payments can be async — card auth may come in seconds)
- Refunds, partial refunds, chargebacks
- Idempotent APIs (retry-safe)
- Webhook notifications to merchants on payment status changes

### Non-Functional
```
Scale: 10K transactions/sec (peak, e.g., Big Billion Days)
Latency: payment response < 5 seconds (most card payments: 2-3s)
Durability: ZERO payment loss — catastrophic business/legal failure
Consistency: a payment must NEVER be double-charged
Availability: 99.999% (24/7/365 — money never sleeps)
```

---

## 2. Core Entities

```sql
-- Payment (the core record)
payments (
  payment_id       UUID PRIMARY KEY,
  merchant_id      UUID,
  customer_id      UUID,
  amount           DECIMAL(12,2),
  currency         VARCHAR(3),
  status           ENUM('INITIATED','PROCESSING','SUCCESS','FAILED',
                         'REFUNDED','PARTIALLY_REFUNDED','DISPUTED'),
  payment_method   ENUM('CARD','UPI','NETBANKING','WALLET'),
  gateway          VARCHAR(50),        ← "stripe", "razorpay", "paytm"
  gateway_txn_id   VARCHAR(100),       ← gateway's reference ID
  idempotency_key  VARCHAR(64) UNIQUE, ← merchant-provided (prevent double charge)
  created_at       TIMESTAMP,
  updated_at       TIMESTAMP,
  retry_count      INT DEFAULT 0,
  failure_reason   TEXT
)

-- Payment events (immutable audit log — append only)
payment_events (
  event_id    UUID PRIMARY KEY,
  payment_id  UUID,
  event_type  ENUM('INITIATED','AUTH_REQUEST','AUTH_RESPONSE',
                   'CAPTURE','REFUND_INITIATED','SETTLED','DISPUTED'),
  metadata    JSONB,
  created_at  TIMESTAMP
)

-- Merchants
merchants (
  merchant_id  UUID PK, name, email, webhook_url TEXT,
  api_key      VARCHAR(64),              ← hashed for authentication
  currency     VARCHAR(3) DEFAULT 'INR',
  active       BOOLEAN
)

-- Refunds
refunds (
  refund_id   UUID PK, payment_id UUID FK,
  amount      DECIMAL(12,2),
  reason      TEXT, status ENUM('INITIATED','PROCESSING','SUCCESS','FAILED'),
  gateway_refund_id VARCHAR(100), created_at TIMESTAMP
)
```

---

## 3. Architecture

```
┌───────────────────────────────────────────────────────────────────┐
│                     PAYMENT INITIATION                             │
│                                                                    │
│  Merchant App/Website                                             │
│       │                                                           │
│  POST /payments { amount, currency, payment_method, card_token,  │
│                   idempotency_key, order_id }                    │
│       │                                                           │
│  ┌────▼──────────────────────────────────────────────────────┐   │
│  │                   Payment API Service                      │   │
│  │  1. Validate: amount > 0, merchant is active              │   │
│  │  2. Idempotency check: SELECT WHERE idempotency_key = ?   │   │
│  │     If exists → return existing payment response           │   │
│  │  3. INSERT payment (status='INITIATED')                    │   │
│  │  4. Publish to Kafka: "payment.initiate" topic             │   │
│  │  5. Return: { payment_id, status: "PROCESSING" }          │   │
│  └────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────┘
                         │ Kafka "payment.initiate"
┌────────────────────────▼──────────────────────────────────────────┐
│                     PAYMENT PROCESSING                             │
│                                                                    │
│  Payment Processor Worker (Kafka Consumer):                       │
│  1. Read payment event from Kafka                                 │
│  2. Select gateway (Stripe for cards, UPI gateway for UPI)        │
│  3. Call gateway API: charge card / initiate UPI                  │
│  4. Gateway responds: success/failure/pending                     │
│  5. UPDATE payment status in DB                                   │
│  6. Publish to Kafka: "payment.result" topic                      │
└────────────────────────┬──────────────────────────────────────────┘
                         │ Kafka "payment.result"
┌────────────────────────▼──────────────────────────────────────────┐
│                     NOTIFICATION & SETTLEMENT                      │
│                                                                    │
│  Result Consumer:                                                  │
│  1. Fetch merchant webhook_url from DB                            │
│  2. POST webhook: { payment_id, status, amount, gateway_txn_id }  │
│  3. If webhook fails: retry with exponential backoff (3 attempts)  │
│  4. Store payment event log (immutable)                           │
│  5. Update merchant balance (for settlement at end of day)         │
└───────────────────────────────────────────────────────────────────┘
```

---

## 4. Exactly-Once Kafka Processing (The Core Interview Question)

### Problem: At-Least-Once vs Exactly-Once
```
Default Kafka guarantee: AT-LEAST-ONCE delivery.

Scenario:
  Worker reads payment message from Kafka
  → Calls Stripe API → Stripe charges card (SUCCESS)
  → Worker crashes BEFORE committing Kafka offset
  
  Kafka: offset not committed → message re-delivered on worker restart
  Worker: re-reads same message → calls Stripe again → DOUBLE CHARGE!

This is catastrophic for payments.
```

### Solution: Idempotent Consumer Pattern (Building Exactly-Once)

**Step 1: Idempotent Processing Key**
```
Each Kafka message has a unique key: payment_id
Before processing: check if this payment_id was already processed successfully

DB table:
  processed_messages (
    message_id   VARCHAR(100) PRIMARY KEY,  ← payment_id + kafka_topic + partition + offset
    processed_at TIMESTAMP,
    result       JSONB                       ← the outcome (so we return same result on re-process)
  )

Worker logic:
  FOR EACH message from Kafka:
    key = f"{payment_id}:{topic}:{partition}:{offset}"
    
    IF SELECT FROM processed_messages WHERE message_id = key:
        SKIP (already processed — commit offset and move on)
    
    result = process_payment(payment)   ← call Stripe, update DB
    
    INSERT INTO processed_messages (message_id, processed_at, result) VALUES (key, ...)
    commit Kafka offset
```

**Step 2: Outbox Pattern (DB + Kafka as atomic unit)**
```
Problem: How do we guarantee that if we write to DB, we also publish to Kafka?
  Option A: Write DB first, then publish Kafka → crash between = Kafka never gets message
  Option B: Publish Kafka first, then write DB → crash = DB never updated

Solution: Outbox Pattern
  
  Step 1: In same DB transaction:
    UPDATE payments SET status = 'SUCCESS'
    INSERT INTO outbox (id, topic, payload, created_at, published=FALSE)
    COMMIT
  
  Step 2: Outbox publisher (separate process, CDC or polling):
    SELECT * FROM outbox WHERE published = FALSE ORDER BY created_at
    FOR EACH row: publish to Kafka → on success: UPDATE outbox SET published = TRUE
  
  → DB write and Kafka publish are now atomic with respect to each other
  → If Kafka publish fails: row stays in outbox → retry next cycle
  → Kafka consumer gets at-least-once → idempotent processing key prevents double processing
```

**Step 3: Kafka Transactions (Kafka 0.11+ only)**
```
If you have Kafka 0.11+, you can use Kafka Transactions:
  producer.initTransactions()
  
  try:
    producer.beginTransaction()
    producer.send("payment.result", message)
    consumer.commitSync(offsets)    ← offset commit inside transaction
    producer.commitTransaction()
  except:
    producer.abortTransaction()
    # Message not sent, offset not committed → safe to retry

This gives true exactly-once semantics within Kafka cluster.
But: only for Kafka-to-Kafka pipelines (not external systems like Stripe).
External calls (Stripe API) still need idempotency keys.
```

---

## 5. Gateway Routing — Fallback Strategy

```
Payment routing:
  Cards (Indian): RazorPay (primary) → PayU (fallback) → Cashfree (fallback)
  Cards (International): Stripe (primary) → Braintree (fallback)
  UPI: NPCI gateway (primary) → PhonePe switch (fallback)
  
  Router logic:
    1. Check gateway health (real-time success rate > 95%?)
    2. If primary healthy → route to primary
    3. If primary unhealthy (circuit breaker OPEN) → route to fallback
    4. If all fallbacks fail → return FAILED (very rare)

Circuit breaker per gateway:
  Window: last 100 transactions
  If failure rate > 20%: OPEN circuit for 5 minutes
  After 5 min: try 1 request (HALF-OPEN)
  If success: CLOSE circuit (back to normal)
  If failure: OPEN again for another 5 min
```

---

## 6. Security

```
PCI-DSS compliance (mandatory for card data):
  Never store raw card numbers → use tokenization
  Client: card_number → payment gateway tokenizer → returns token
  Merchant stores: token (not card number)
  
  Encryption:
    All APIs: TLS 1.3 minimum
    Stored sensitive data: AES-256 encryption
    Key management: AWS KMS or HashiCorp Vault
  
  API authentication:
    Merchant → Payment Gateway: API key (hashed in DB, never stored plain)
    Payment Gateway → Merchant webhook: HMAC-SHA256 signature on payload
    Merchant verifies signature before processing webhook
  
  Fraud detection:
    Rule engine: amount > $10K from new account → flag for review
    ML model: anomaly detection on transaction patterns
    Velocity check: > 5 payments from same card in 10 min → hold
```

---

## 7. Scale Progression

```
Stage 1 (100 TPS):
  Single API server + PostgreSQL + direct gateway calls
  Synchronous: API → Gateway → response → return
  No Kafka needed yet

Stage 2 (1,000 TPS):
  Add Kafka for async processing
  API returns immediately with payment_id
  Background worker calls gateway
  Webhook notifies merchant of result

Stage 3 (10,000 TPS):
  Multiple gateway worker pools (partitioned by gateway type)
  Redis for idempotency key cache (hot path)
  PostgreSQL sharded by merchant_id
  Separate DB for payment_events (append-only, Cassandra)
  Circuit breakers per gateway with automatic fallback

Stage 4 (100,000 TPS):
  Multi-region active-active (payments in nearest region)
  Global PostgreSQL (Aurora Global) for payment records
  Kafka MirrorMaker for cross-region event sync
  Dedicated fraud detection service (ML inference < 100ms)
```

---

## Key Talking Points

1. **Idempotency key** — merchant must send this; prevents double charge on retry
2. **Exactly-once = idempotent consumer + at-least-once delivery** — build it at application level
3. **Outbox pattern** — atomic DB write + Kafka publish without distributed transaction
4. **Kafka transactions** — mention as 0.11+ feature; clarify it only covers Kafka-to-Kafka
5. **External calls (Stripe) still need idempotency keys** — Kafka TX alone isn't enough
6. **Circuit breaker** per gateway — critical for availability; gateway outages are common
7. **PCI-DSS**: never store raw card numbers, tokenization, TLS, HMAC on webhooks
8. **Scale story**: synchronous → Kafka async → sharding → multi-region

# 20 — Multi-Source Log & Media Ingestion System

> **Interview context**: A system accepting data from multiple sources (API, CSV, events). How to collect all data, normalize it, store it, and handle failures.

![Multi-Source Log & Media Ingestion — High Level Design](/Users/abhishekkumar/.gemini/antigravity/scratch/coding/platform-go/system-design-wiki/diagrams/log-ingestion-hld.jpg)

---

## 1. Requirements Clarification

### Functional Requirements
- Ingest data from **3+ source types**: REST API, CSV file uploads, real-time events (webhooks/Kafka)
- Parse, validate, and normalize data from each source into a unified format
- Store raw data (for replay) and processed data (for queries)
- Support **deduplication** — same log from 2 sources = 1 record
- Query/search logs: by time range, source, log level, service name
- Media files (images, videos, binary) attached to events → store in object storage
- Dashboard: real-time throughput metrics, error rates per source

### Non-Functional Requirements
- Ingest **10M events/day** from APIs + **1M CSV rows/day** + real-time events
- P99 API ingestion latency < 500ms (return quickly, process async)
- Data available for query within **30 seconds** of ingestion
- Zero data loss — every event must eventually be stored
- Handle **burst traffic** (10× normal during incidents)

---

## 2. Source Types & Their Challenges

```
Source 1: REST API (applications push logs via HTTP POST)
  Challenge: variable rate, need fast ACK, process async
  Volume: 5M events/day

Source 2: CSV File Upload (batch logs from legacy systems, reports)
  Challenge: large files (up to 1GB), long processing, progress tracking
  Volume: 100 files/day × 10K rows = 1M rows/day

Source 3: Real-time Events (Kafka topics, webhooks from external services)
  Challenge: ordering, exactly-once, high throughput
  Volume: 4M events/day
```

---

## 3. Core Entities & Schema

```sql
-- Unified log event (processed, queryable)
events (
  event_id      UUID PRIMARY KEY,
  source_type   ENUM('api', 'csv', 'event_stream'),
  source_id     VARCHAR(100),     ← which API key, which CSV file, which Kafka topic
  service_name  VARCHAR(100),
  log_level     ENUM('DEBUG','INFO','WARN','ERROR','FATAL'),
  message       TEXT,
  metadata      JSONB,            ← flexible fields from different sources
  event_time    TIMESTAMP,        ← when event happened (not ingestion time)
  ingested_at   TIMESTAMP,
  dedupe_key    VARCHAR(64) UNIQUE, ← hash(source_type + source_id + content + time)
  media_urls    TEXT[],           ← S3 URLs for attached media
  partitioned by (ingested_at)    ← TimescaleDB or Cassandra time-series partitioning
)

-- CSV Upload Jobs (for tracking large file processing)
csv_jobs (
  job_id        UUID PRIMARY KEY,
  filename      VARCHAR(300),
  s3_raw_key    TEXT,             ← raw file stored here first
  status        ENUM('UPLOADED','PARSING','COMPLETED','FAILED'),
  total_rows    INT,
  processed_rows INT,
  failed_rows   INT,
  error_log     TEXT,
  created_at    TIMESTAMP,
  completed_at  TIMESTAMP
)

-- Raw event archive (for replay, compliance)
raw_events_archive:
  S3: s3://logs-raw/{year}/{month}/{day}/{source_type}/{batch_id}.json.gz
  (Never deleted — cheap S3 Glacier after 90 days)
```

---

## 4. API Design

```
# API Source: Accept events
POST /ingest/events
  { source_id: "myapp-prod", events: [ { level, message, timestamp, metadata }, ... ] }
Response: { accepted: 150, batch_id: "abc-123" }  ← fast, async processing

# CSV Source: Upload batch file
POST /ingest/csv
  Content-Type: multipart/form-data
  file: <csv_file>
  source_id: "legacy-billing-system"
Response: { job_id: "xyz-456", status: "UPLOADED" }

GET /ingest/csv/{job_id}/status
Response: { status: "PARSING", processed_rows: 4521, total_rows: 10000, eta: "2 min" }

# Query events
GET /events
  ?service=payment-service&level=ERROR&from=2026-09-01T00:00Z&to=2026-09-02T00:00Z
  &source_type=api&search=timeout&page=1&limit=50

# Metrics
GET /metrics/throughput?window=1h&group_by=source_type
GET /metrics/errors?window=24h&group_by=service
```

---

## 5. High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        INGESTION LAYER                               │
│                                                                       │
│  API Push         CSV Upload         Event Stream                     │
│  POST /events     POST /ingest/csv   Kafka Consumer                  │
│      │                │                   │                          │
│      │           S3 Raw Upload             │                          │
│      │           (streaming upload)        │                          │
│      ▼                ▼                   ▼                          │
│  ┌───────────────────────────────────────────────┐                  │
│  │            Kafka (ingestion.raw topic)          │                  │
│  │   Partitioned by source_type (parallelism)     │                  │
│  └───────────────────────┬───────────────────────┘                  │
└──────────────────────────┼──────────────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────────────┐
│                     PROCESSING LAYER                                  │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │              Stream Processor (Kafka Streams / Flink)         │    │
│  │                                                               │    │
│  │  1. Parse (JSON/CSV/Avro → unified schema)                   │    │
│  │  2. Validate (required fields, type checking)                 │    │
│  │  3. Normalize (timestamps to UTC, log levels to ENUM)         │    │
│  │  4. Deduplicate (bloom filter on dedupe_key)                  │    │
│  │  5. Enrich (add service metadata from Config Service)         │    │
│  │  6. Route media: extract URLs → download → upload to S3       │    │
│  └─────────────────────────────────────────────────────────────┘    │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
            ┌──────────────┼──────────────────┐
            ▼              ▼                  ▼
     ┌──────────┐  ┌───────────────┐  ┌──────────────┐
     │TimescaleDB│  │ Elasticsearch │  │  S3 Archive  │
     │(queryable │  │(full-text     │  │(raw + media  │
     │ time-series│ │ search)       │  │  storage)    │
     └──────────┘  └───────────────┘  └──────────────┘
                           │
                    ┌──────▼──────┐
                    │  Dashboard   │
                    │  (Grafana /  │
                    │   custom UI) │
                    └─────────────┘
```

---

## 6. CSV Processing — Deep Dive

```
Large CSV files (up to 1GB) must be processed without blocking:

Step 1: Streaming Upload to S3
  Client → POST /ingest/csv → API Server
  API Server: stream file directly to S3 (don't buffer in memory)
  → s3://logs-raw/uploads/{job_id}/raw.csv
  → return { job_id } immediately

Step 2: Async Processing Job
  S3 upload complete → S3 event → SQS → CSV Processor worker

Step 3: Chunked Processing
  CSV Processor:
    Opens S3 object with streaming reader (not download all)
    Reads 1000 rows at a time:
      for chunk in read_chunks(s3_file, chunk_size=1000):
          events = parse_chunk(chunk)
          validate_events(events)
          publish_to_kafka(events)
          update_job_progress(processed_rows += 1000)
    
    On partial failure:
      Store failed rows with error reason
      Continue processing → complete job partially
      
Step 4: Progress Tracking
  Redis: job:{job_id}:progress → { processed: 5000, total: 10000, failed: 23 }
  Client polls GET /ingest/csv/{job_id}/status → reads from Redis
```

---

## 7. Deduplication Strategy

```
Problem: Same event arrives via API AND as a real-time event stream → store once

Deduplication key:
  dedupe_key = SHA256(source_type + "|" + event_time + "|" + message + "|" + service_name)

Bloom Filter (probabilistic, fast):
  Bloom filter in Redis: 1% false positive rate → 99% of duplicates caught
  Before inserting: check bloom filter
  If NOT in filter: insert + add to filter
  If IN filter: check DB (true dedup) → skip if duplicate

  Redis Bloom Filter: BLOOM.ADD dedupe_filter {dedupe_key}
                      BLOOM.EXISTS dedupe_filter {dedupe_key}

Exact dedup: DB UNIQUE constraint on dedupe_key column
  If bloom filter false positive: DB insert fails on unique constraint → caught
```

---

## 8. Media Handling

```
Events may include media URLs or base64 encoded images.

Inline base64 (small images < 1MB):
  Processor detects base64 field
  Decode → upload to S3: s3://logs-media/{year}/{month}/{event_id}.jpg
  Replace base64 with S3 URL in normalized event

External media URL (link in log event):
  Processor: download URL → upload to S3 (avoids link rot)
  Replace external URL with S3 URL
  
Large media (> 10MB):
  Don't process inline → flag for async Media Processor
  Store event with media_status: "PENDING"
  Media Processor: download, compress, store, update event record

S3 Structure:
  s3://logs-media/{year}/{month}/{day}/{event_id}/{filename}
  Lifecycle: standard → Glacier after 90 days → delete after 7 years
```

---

## 9. Failure Handling

```
API Push failure:
  Client gets 202 Accepted immediately
  If Kafka publish fails → retry 3× with backoff
  If still fails → write to DLQ (Dead Letter Queue)
  DLQ alert → ops team investigates → manual replay from DLQ

CSV Processing failure:
  Worker crashes mid-file?
    → Job state persisted in DB
    → New worker picks up from last committed offset
    → Idempotent: re-process same rows is safe (dedupe_key prevents duplicates)

Event Stream (Kafka consumer) failure:
  Kafka offsets not committed until processing success
  Worker crashes → auto-restart → re-reads from last committed offset
  → At-least-once delivery → dedup layer prevents duplicate storage

Network partition (can't write to TimescaleDB):
  Events queued in Kafka (Kafka is the buffer)
  Kafka retention: 7 days → events not lost
  When DB recovers → consumer catches up from last committed offset
```

---

## 10. Querying & Search

```
Query patterns:
  Time-range query: "Show errors from payment-service in last 1 hour"
    → TimescaleDB: SELECT * FROM events WHERE service='payment' AND level='ERROR'
                   AND ingested_at > NOW() - INTERVAL '1 hour'
    → Partitioned by ingested_at (date) → only scans today's partition

  Full-text search: "Find logs containing 'connection timeout'"
    → Elasticsearch: { "query": { "match": { "message": "connection timeout" } } }

  Aggregate metrics: "Error rate per service in last 24h"
    → TimescaleDB time_bucket aggregation:
       SELECT service_name, time_bucket('1 hour', ingested_at), COUNT(*)
       FROM events WHERE level='ERROR' AND ingested_at > NOW() - INTERVAL '24h'
       GROUP BY 1, 2

  Media browse: "Show all images from batch upload CSV job xyz"
    → Query events WHERE source_id = 'csv:xyz' AND media_urls != '{}' LIMIT 50
```

---

## Key Talking Points

1. **All sources → Kafka** (unified buffer) — decouple ingestion from processing
2. **CSV streaming to S3 first** — never buffer large files in memory
3. **Deduplication**: Bloom filter (fast, cheap) + DB unique constraint (safety net)
4. **At-least-once + idempotent** = effectively exactly-once without 2PC overhead
5. **TimescaleDB** for time-series queries, **Elasticsearch** for full-text — each optimized for its use case
6. **S3 as raw archive** — always retain raw; processed is derived from raw (replay capability)
7. **Burst handling**: Kafka absorbs bursts; consumers process at sustainable rate

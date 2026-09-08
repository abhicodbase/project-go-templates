# 26 — Hotel Tag Management System (Agoda Interview)

> **Interview context**: Agoda. Hotel reviews stored in SQL DB. 500 reviews/sec. Design a tag management system that extracts tags from reviews. Tags used for hotel search ("good location" → top 10 hotels).

---

## 1. Problem Understanding

```
Input:  500 review comments/second stored in SQL DB
          "The location was amazing, very close to metro station"
          "The room was small but very clean and cozy"
          "Great staff, very friendly and helpful"

Process: Extract tags from text
          → "good location", "clean room", "friendly staff"

Output: 
  1. Tags associated with hotels
  2. Search: user types "good location" → top 10 hotels with that tag, ranked by count

Scale: 500 reviews/sec = 43M reviews/day
       Hotel count: ~1M hotels
       Unique tags: ~10K standardized tags (not unlimited)
```

---

## 2. Requirements Clarification

### Functional
- Extract tags from review text (automated)
- Tag dictionary: **predefined set** of ~10K tags (not infinite)
- Map tags → hotels with frequency count
- Search API: given tag → top 10 hotels (ranked by tag count)
- Admin: define/manage tag dictionary

### Non-Functional
- Reviews arrive at 500/sec → must process asynchronously
- Search latency < 100ms
- New review's tags appear in search results within 5 minutes
- Tag extraction accuracy > 90%

---

## 3. Tag Extraction Approaches

### Approach 1: Rule-Based (Fast, Explainable)
```python
TAG_KEYWORDS = {
  "good location":    ["location", "near metro", "central", "walking distance", "nearby"],
  "clean room":       ["clean", "spotless", "tidy", "hygienic"],
  "friendly staff":   ["staff", "friendly", "helpful", "polite", "courteous"],
  "good breakfast":   ["breakfast", "buffet", "morning meal"],
  "noisy":           ["noisy", "loud", "noise", "disturbing"],
  "good value":      ["value", "worth", "cheap", "affordable", "budget"],
}

def extract_tags(review_text: str) -> list[str]:
    text = review_text.lower()
    matched_tags = []
    for tag, keywords in TAG_KEYWORDS.items():
        if any(kw in text for kw in keywords):
            matched_tags.append(tag)
    return matched_tags

# Fast: O(keywords × review_length) per review
# Trade-off: misses paraphrases ("strategically located" = "good location"?)
```

### Approach 2: NLP / ML (Higher Accuracy)
```
Pre-trained NLP model (BERT fine-tuned on hotel reviews):
  Input: "The room was surprisingly spacious and very clean"
  Output: ["clean room", "spacious room"]

Tools: HuggingFace Transformers, spaCy, or Google NL API
Pros: handles paraphrases, multi-language
Cons: slower (50-200ms per review), GPU cost
```

### Approach 3: Hybrid (Production Choice)
```
1st pass: Rule-based (fast, ~90% coverage of common tags)
2nd pass: ML on unmatched reviews (catches nuanced language)
Cost: run ML only on 10% of reviews that didn't match rules
```

---

## 4. Core Entities & DB Schema

```sql
-- Tag dictionary (predefined, admin-managed)
tags (
  tag_id    INT PRIMARY KEY AUTO_INCREMENT,
  tag_name  VARCHAR(100) UNIQUE,  ← "good location"
  category  VARCHAR(50),          ← "location", "cleanliness", "service", "value"
  keywords  TEXT,                  ← comma-separated trigger words
  is_active BOOLEAN
)

-- Hotel-tag associations (the core table for search)
hotel_tags (
  hotel_id  BIGINT,
  tag_id    INT,
  count     INT DEFAULT 0,        ← how many reviews mention this tag for this hotel
  PRIMARY KEY (hotel_id, tag_id)
)
-- INDEX: (tag_id, count DESC) ← for "top 10 hotels with this tag" query

-- Reviews (existing SQL table)
reviews (
  review_id  BIGINT PRIMARY KEY,
  hotel_id   BIGINT,
  user_id    BIGINT,
  text       TEXT,
  rating     TINYINT,
  created_at TIMESTAMP,
  processed  BOOLEAN DEFAULT FALSE  ← tracking processing state
)

-- Review-tag mappings (which tags extracted from which review)
review_tags (
  review_id BIGINT,
  tag_id    INT,
  PRIMARY KEY (review_id, tag_id)
)
```

---

## 5. Architecture — Full Pipeline

```
┌──────────────────────────────────────────────────────────────────────┐
│                    INGESTION (500 reviews/sec)                        │
│                                                                        │
│  Reviews written → SQL DB (existing system)                           │
│                                                                        │
│  CDC (Change Data Capture):                                           │
│    Debezium reads MySQL binlog                                         │
│    New/updated reviews → Kafka topic "hotel.reviews"                  │
│    (decoupled: main app unaffected)                                   │
└────────────────────────┬─────────────────────────────────────────────┘
                         │
┌────────────────────────▼─────────────────────────────────────────────┐
│                    PROCESSING (Tag Extraction)                         │
│                                                                        │
│  Kafka Consumer Group: "tag-extractor" (10 consumers)                 │
│                                                                        │
│  For each review:                                                      │
│    1. Load review text from Kafka message                             │
│    2. Load tag dictionary (Redis cached, refreshed hourly)            │
│    3. Extract tags: rule-based (+ ML if needed)                       │
│    4. Store extracted tags → review_tags table                        │
│    5. Update hotel_tags counters:                                     │
│       UPDATE hotel_tags SET count = count + 1                         │
│       WHERE hotel_id = ? AND tag_id = ?                               │
│       ON CONFLICT DO UPDATE SET count = count + 1  ← UPSERT           │
│    6. Invalidate Redis cache: DEL hotel_tags:{tag_id}:top10           │
└────────────────────────┬─────────────────────────────────────────────┘
                         │
┌────────────────────────▼─────────────────────────────────────────────┐
│                    SEARCH (Tag → Top 10 Hotels)                       │
│                                                                        │
│  User: "good location"                                                │
│                                                                        │
│  1. Resolve tag: "good location" → tag_id = 42                       │
│  2. Redis cache lookup: hotel_tags:42:top10                           │
│     HIT: return cached [hotel_id, count] pairs (< 1ms)               │
│     MISS: query DB:                                                   │
│       SELECT hotel_id, count FROM hotel_tags                          │
│       WHERE tag_id = 42 ORDER BY count DESC LIMIT 10                 │
│       → Store in Redis with TTL = 5 min                               │
│  3. Fetch hotel details for top 10 hotel_ids                          │
│  4. Return to user                                                    │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 6. CDC (Change Data Capture) — Why Not Polling?

```
Naive approach (DON'T DO THIS):
  SELECT * FROM reviews WHERE processed = FALSE
  → Polling every 1 second → 500 queries/sec of dead-weight DB load
  → Race conditions if multiple workers poll
  → Missing rows if processed flag update fails

CDC with Debezium:
  Reads MySQL binary log (binlog) → captures every INSERT/UPDATE in real-time
  → Publishes to Kafka with zero load on application DB
  → At-least-once delivery (Debezium handles retries)
  → No polling, no flags, no race conditions
```

---

## 7. Handling Scale (500 Reviews/Sec)

```
Tag extraction: 500/sec with 10 Kafka consumers = 50/sec per consumer
  Rule-based extraction: ~5ms per review → each consumer handles 200/sec easily

DB updates (hotel_tags.count):
  500 updates/sec to hotel_tags table
  → This is write-heavy! Batch with short interval:
    Instead of 1 UPDATE per review:
      Buffer in memory for 5 seconds → group by (hotel_id, tag_id) → batch UPSERT
      INSERT INTO hotel_tags (hotel_id, tag_id, count) VALUES (...)
      ON CONFLICT DO UPDATE SET count = hotel_tags.count + EXCLUDED.count

  → Reduces 500 individual updates/sec to ~10 batch updates/sec

Redis cache invalidation:
  On each hotel_tag update → DEL hotel_tags:{tag_id}:top10
  → Next query rebuilds cache
  → High write rate + invalidation = low cache hit rate?
  
  Solution: Use Redis ZINCRBY instead of DB + cache:
    ZINCRBY hotel_tags:42 1 hotel_id_123  (sorted set, auto-ranked)
    ZREVRANGE hotel_tags:42 0 9  (top 10 instantly)
    → No cache invalidation needed — Redis IS the data store for hot rankings
    → Persist to DB asynchronously every 5 minutes for durability
```

---

## 8. Multi-Language Tag Extraction

```
Reviews arrive in English, Thai, Japanese, Korean (Agoda serves Asia-Pacific)

Approach:
  Step 1: Language detection (langdetect library or FastText)
  Step 2: If not English → translate to English (Google Translate API or local model)
  Step 3: Apply tag extraction rules (English only)
  
  OR: Train multilingual model (mBERT) for all languages simultaneously
  
  Trade-off: Translation adds latency; multilingual model needs more GPU
```

---

## 9. Tag Dictionary Management

```
Problem: Who decides what a tag is? How are new tags added?

Tag lifecycle:
  1. ML discovers new common phrase clusters in reviews (unsupervised)
     → Show to admin: "The phrase 'rooftop bar' appears 50K times — add as tag?"
  2. Admin approves → adds to tags table with keywords
  3. Tag dictionary updated in Redis (cached lookup table)
  4. Re-process historical reviews? (optional — expensive)

Predefined categories for tags:
  Location: "good location", "near airport", "city center"
  Room:     "clean room", "spacious room", "small room", "noisy room"
  Service:  "friendly staff", "fast check-in", "24hr service"
  Value:    "good value", "overpriced", "worth the price"
  Amenities: "good breakfast", "nice pool", "free parking"
```

---

## Key Talking Points

1. **CDC (Debezium)** for reading reviews — not polling — decouple from main DB
2. **Kafka consumer group** for parallel tag extraction (10 consumers = 10× throughput)
3. **Redis Sorted Set** (`ZINCRBY`) for tag rankings — O(log N) update, O(log N) read
4. **Batch UPSERT** to hotel_tags — don't do individual DB writes at 500/sec
5. **Rule-based first, ML second** — hybrid approach for accuracy + cost balance
6. **Tag dictionary in Redis** — cached so every consumer doesn't query DB for keywords
7. **Multi-language** — mention translation or mBERT; shows Agoda-specific awareness
8. **Cache TTL = 5 min** for top-10 results — balance freshness vs DB load

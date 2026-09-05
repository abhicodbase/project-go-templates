# 11 — Search & Indexing

## TL;DR
> Elasticsearch is a distributed search engine built on Lucene. Inverted index is what makes full-text search fast.  
> Typeahead uses trie or Redis sorted sets. Design search to handle typos, synonyms, and ranking.

---

## 1. Why Dedicated Search?

```
SQL LIKE query (naive):
  SELECT * FROM products WHERE name LIKE '%wireless headphones%'
  → Full table scan every time (O(n)) → very slow at scale

Elasticsearch full-text search:
  GET /products/_search?q=wireless+headphones
  → Inverted index lookup → milliseconds even on millions of docs
```

---

## 2. Inverted Index

The core data structure behind all full-text search engines.

### Building an Inverted Index
```
Documents:
  Doc 1: "system design interview"
  Doc 2: "design patterns in Go"
  Doc 3: "interview preparation tips"

Inverted Index:
  "system"      → [Doc 1]
  "design"      → [Doc 1, Doc 2]
  "interview"   → [Doc 1, Doc 3]
  "patterns"    → [Doc 2]
  "go"          → [Doc 2]
  "preparation" → [Doc 3]
  "tips"        → [Doc 3]

Query: "design interview"
  → docs containing "design": {1, 2}
  → docs containing "interview": {1, 3}
  → intersection: {1}  (or union for OR queries)
  → Result: Doc 1 (ranked by relevance)
```

### Text Analysis Pipeline
```
Input: "Running Quickly through the Forest"
  │
  ▼ Tokenization
["Running", "Quickly", "through", "the", "Forest"]
  │
  ▼ Lowercasing
["running", "quickly", "through", "the", "forest"]
  │
  ▼ Stop word removal (remove "the", "through")
["running", "quickly", "forest"]
  │
  ▼ Stemming (or Lemmatization)
["run", "quick", "forest"]
  │
  ▼ Stored in inverted index
```

**Stemming**: "running" → "run", "quickly" → "quick"
**Lemmatization**: "better" → "good" (more accurate, slower)
**Stop words**: "the", "a", "in", "at" (too common to be useful for search)

---

## 3. Elasticsearch Deep Dive

Elasticsearch is a distributed search and analytics engine.

### Core Concepts

| Concept | SQL Equivalent | Description |
|---------|---------------|-------------|
| Index | Database/Table | Collection of documents |
| Document | Row | JSON object with fields |
| Field | Column | Attribute of a document |
| Mapping | Schema | Definition of field types |
| Shard | - | Piece of an index on a node |
| Replica | - | Copy of a shard for HA |

### Architecture
```
Elasticsearch Cluster (3 nodes):
  ┌──────────────────────────────────────────┐
  │ Node 1 (Master)                          │
  │  Shard 0 (Primary)  Shard 2 (Replica)   │
  ├──────────────────────────────────────────┤
  │ Node 2                                   │
  │  Shard 1 (Primary)  Shard 0 (Replica)   │
  ├──────────────────────────────────────────┤
  │ Node 3                                   │
  │  Shard 2 (Primary)  Shard 1 (Replica)   │
  └──────────────────────────────────────────┘

Index "products" has 3 primary shards + 1 replica each
Document routing: shard = hash(doc_id) % num_shards
```

### Basic Operations
```json
// Index a document
PUT /products/_doc/1
{
  "name": "Wireless Headphones",
  "brand": "Sony",
  "price": 299.99,
  "category": "Electronics",
  "description": "High-quality noise-cancelling wireless headphones"
}

// Search
GET /products/_search
{
  "query": {
    "bool": {
      "must": [
        { "match": { "description": "noise cancelling" } }
      ],
      "filter": [
        { "range": { "price": { "lte": 400 } } },
        { "term": { "category": "Electronics" } }
      ]
    }
  },
  "sort": [{ "_score": "desc" }, { "price": "asc" }],
  "from": 0,
  "size": 10
}
```

### Query Types
| Query | Use Case |
|-------|---------|
| `match` | Full-text search on analyzed text |
| `term` | Exact match on keyword/numeric fields |
| `range` | Numeric/date range (price: 100-500) |
| `bool` | Combine must/should/filter/must_not |
| `fuzzy` | Typo-tolerant search ("headphons" → "headphones") |
| `prefix` | Starts with ("wire" → "wireless") |
| `wildcard` | Pattern matching ("head*") |
| `multi_match` | Search across multiple fields |

### Relevance Scoring (TF-IDF / BM25)
```
TF (Term Frequency): how often does the term appear in the doc?
IDF (Inverse Document Frequency): how rare is the term across all docs?

Score = TF × IDF
  → Common terms ("the") have low IDF (appear everywhere) → low score boost
  → Rare terms ("elasticsearch") have high IDF → high score boost

BM25 (modern default in ES): improves TF-IDF with document length normalization
```

### Aggregations (Analytics)
```json
// Group products by category + avg price per category
GET /products/_search
{
  "aggs": {
    "by_category": {
      "terms": { "field": "category.keyword" },
      "aggs": {
        "avg_price": { "avg": { "field": "price" } }
      }
    }
  }
}
// Result: { Electronics: avg $350, Clothing: avg $45, ... }
```

---

## 4. Keeping Search in Sync with DB

The hardest part: Elasticsearch is not the source of truth — the DB is.

### Approach 1: Dual Write
```
Application:
  1. Write to DB (primary)
  2. Write to Elasticsearch
  
Problem: if step 2 fails → DB and ES out of sync
```

### Approach 2: Change Data Capture (CDC)
```
DB ──binlog/WAL──▶ CDC tool (Debezium) ──▶ Kafka ──▶ Elasticsearch
   (MySQL binlog)  (reads DB change log)           (consumer indexes)

Pros: guaranteed sync, no application changes needed
Cons: eventual consistency, slight delay
```

### Approach 3: Sync Worker / Queue
```
DB write ──▶ Publish "ProductUpdated" event to Kafka
              ──▶ Search Indexer Consumer ──▶ Elasticsearch
```

---

## 5. Typeahead / Autocomplete

Suggest search terms as the user types.

### Requirements
- Sub-100ms latency
- Suggest popular / relevant completions
- Handle typos (fuzzy matching)

### Approach 1: Trie (Prefix Tree)
```
Stored words: "apple", "application", "apply", "apt"

Trie:
a
└── p
    ├── p
    │   ├── l
    │   │   ├── e     ← "apple"
    │   │   ├── i
    │   │   │   └── cation ← "application"
    │   │   └── y     ← "apply"
    └── t             ← "apt"

Query "appl" → traverse to "appl" node → all children are completions
```

**Problem**: very memory-intensive for millions of queries; hard to distribute.

### Approach 2: Redis Sorted Sets
```
ZADD autocomplete:queries 1000 "system design"
ZADD autocomplete:queries 950 "system design interview"
ZADD autocomplete:queries 800 "system design basics"

Prefix search "system des":
  ZRANGEBYLEX autocomplete:queries "[system des" "[system des\xff" LIMIT 0 10

Returns suggestions sorted by score (popularity)
```

### Approach 3: Elasticsearch Completion Suggester
```json
// Index with completion field
PUT /search_suggestions/_doc/1
{
  "suggest": {
    "input": ["system design", "system design interview"],
    "weight": 100
  }
}

// Query
GET /search_suggestions/_search
{
  "suggest": {
    "my-suggest": {
      "prefix": "system des",
      "completion": { "field": "suggest", "size": 10 }
    }
  }
}
```

### Approach 4: Distributed Trie Service
For hyperscale (Google-level):
- Trie built from query logs (log sampled queries, aggregate counts)
- Trie serialized and served from in-memory stores
- Updated periodically (not real-time)

---

## 6. Search Ranking Signals

Beyond text relevance, combine multiple signals:

| Signal | Example |
|--------|---------|
| **Text relevance** | BM25 score |
| **Popularity** | Click-through rate, purchase rate |
| **Recency** | Newer items ranked higher |
| **Personalization** | User's past purchases/views |
| **Business rules** | Promoted products, in-stock boost |
| **Geographic proximity** | Nearby restaurants/stores |
| **Price/Rating** | User filters applied |

```
final_score = (BM25_score × 0.4) 
            + (popularity_score × 0.3)
            + (recency_score × 0.2)
            + (personalization_score × 0.1)
```

---

## 7. Handling Typos

```
Query: "wirless headphons" (2 typos)

Fuzzy Search (Edit Distance = Levenshtein Distance):
  "wirless" → "wireless" (1 character insertion)
  "headphons" → "headphones" (1 character insertion)

Elasticsearch fuzzy query:
  { "fuzzy": { "name": { "value": "wirless", "fuzziness": "AUTO" } } }
  AUTO fuzziness: 0 edits for 1-2 char, 1 edit for 3-5 char, 2 edits for 6+ char
```

---

## Key Takeaways

1. **Inverted index** = foundation of fast full-text search; understand tokenize → stem → index pipeline
2. **Elasticsearch**: documents in indices, sharded across nodes, replicated for HA
3. **BM25** relevance scoring: term frequency × inverse document frequency × doc length normalization
4. **CDC (Debezium + Kafka)** is the best way to keep ES in sync with DB at scale
5. **Typeahead**: Redis Sorted Sets for simple cases; ES Completion Suggester for rich completions
6. **Ranking** is multi-dimensional: text relevance + popularity + personalization + business rules
7. **Fuzziness (edit distance)** for typo tolerance; tune by word length

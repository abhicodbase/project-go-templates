# Database Fundamentals

## ACID Properties

| Property | Meaning | Example |
|----------|---------|---------|
| **Atomicity** | All or nothing — transaction either fully completes or fully rolls back | Booking + payment deduction must both succeed or both fail |
| **Consistency** | DB goes from one valid state to another, constraints maintained | Can't have booking with negative price |
| **Isolation** | Concurrent transactions don't interfere with each other | Two users booking the last room — only one succeeds |
| **Durability** | Committed data persists even after crash | Booking survives server restart |

---

## Isolation Levels & Read Anomalies

```
READ UNCOMMITTED  → Can see uncommitted changes (dirty reads) — almost never use
READ COMMITTED    → Only see committed data (PostgreSQL default)
REPEATABLE READ   → Same query in same transaction returns same result
SERIALIZABLE      → Full isolation — as if executed serially (most expensive)
```

### The Anomalies
```
Dirty Read:       Read uncommitted data that later gets rolled back
Non-repeatable:   Same row returns different value on second read in same transaction
Phantom Read:     A query returns different rows on second read in same transaction
                  (new rows inserted by another transaction)
```

---

## Indexes

### B-Tree Index (Default)
```sql
-- For equality and range queries
CREATE INDEX idx_hotels_city ON hotels (city);
CREATE INDEX idx_bookings_user ON bookings (user_id, check_in); -- composite index

-- Covered index — query satisfied entirely from index (no table lookup)
CREATE INDEX idx_hotels_city_rating ON hotels (city, rating, name);
-- SELECT name FROM hotels WHERE city='Bangkok' AND rating > 4.0 -- covered!
```

### Index Selectivity
```
High selectivity: user_id, booking_id — index very useful (few rows per value)
Low selectivity:  status ('active'/'inactive'), boolean — index often not useful

Rule: Index if WHERE clause filters to < 10-20% of rows
```

### Explain Plan
```sql
-- Always check execution plan for slow queries
EXPLAIN ANALYZE
SELECT h.name, COUNT(b.id) as booking_count
FROM hotels h
JOIN bookings b ON b.hotel_id = h.id
WHERE h.city = 'Bangkok'
  AND b.check_in >= '2024-01-01'
GROUP BY h.name;

-- Look for:
-- Seq Scan on large table → missing index
-- Hash Join vs Nested Loop → check cost
-- Rows estimated vs actual → stale statistics (run ANALYZE)
```

---

## N+1 Query Problem

```go
// ❌ N+1 — 1 query for hotels + N queries for bookings (one per hotel)
hotels, _ := db.Query("SELECT * FROM hotels WHERE city=$1", city)
for _, hotel := range hotels {
    bookings, _ := db.Query("SELECT * FROM bookings WHERE hotel_id=$1", hotel.ID)
    hotel.Bookings = bookings
}

// ✅ JOIN — 1 query total
rows, _ := db.Query(`
    SELECT h.id, h.name, b.id, b.check_in, b.total_price
    FROM hotels h
    LEFT JOIN bookings b ON b.hotel_id = h.id
    WHERE h.city = $1
`, city)

// ✅ Or use IN clause — 2 queries total
hotels, _ := db.Query("SELECT * FROM hotels WHERE city=$1", city)
hotelIDs := extractIDs(hotels)
bookings, _ := db.Query("SELECT * FROM bookings WHERE hotel_id = ANY($1)", pq.Array(hotelIDs))
```

---

## Transactions in Go

```go
func (r *BookingRepo) CreateWithInventory(ctx context.Context, booking *Booking) error {
    tx, err := r.db.BeginTx(ctx, &sql.TxOptions{
        Isolation: sql.LevelReadCommitted,
    })
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback() // Safe no-op if committed

    // Check and decrement room inventory (SELECT FOR UPDATE prevents concurrent overbooking)
    var available int
    err = tx.QueryRowContext(ctx,
        "SELECT available_rooms FROM hotel_inventory WHERE hotel_id=$1 AND date=$2 FOR UPDATE",
        booking.HotelID, booking.CheckIn,
    ).Scan(&available)
    if err != nil {
        return fmt.Errorf("check inventory: %w", err)
    }
    if available <= 0 {
        return ErrNoRoomsAvailable
    }

    // Decrement inventory
    _, err = tx.ExecContext(ctx,
        "UPDATE hotel_inventory SET available_rooms = available_rooms - 1 WHERE hotel_id=$1 AND date=$2",
        booking.HotelID, booking.CheckIn,
    )
    if err != nil {
        return fmt.Errorf("decrement inventory: %w", err)
    }

    // Create booking
    _, err = tx.ExecContext(ctx,
        "INSERT INTO bookings (id, hotel_id, user_id, check_in, check_out, price) VALUES ($1,$2,$3,$4,$5,$6)",
        booking.ID, booking.HotelID, booking.UserID, booking.CheckIn, booking.CheckOut, booking.Price,
    )
    if err != nil {
        return fmt.Errorf("insert booking: %w", err)
    }

    return tx.Commit()
}
```

---

## Caching Strategies

### Cache-Aside (Lazy Loading) — Most Common
```go
func (s *HotelService) GetHotel(ctx context.Context, id string) (*Hotel, error) {
    // 1. Check cache
    if cached, found := s.cache.Get(ctx, "hotel:"+id); found {
        return cached.(*Hotel), nil
    }
    
    // 2. Cache miss — fetch from DB
    hotel, err := s.repo.Get(ctx, id)
    if err != nil {
        return nil, err
    }
    
    // 3. Populate cache
    s.cache.Set(ctx, "hotel:"+id, hotel, 5*time.Minute)
    return hotel, nil
}
```

### Write-Through
```go
func (s *HotelService) UpdateHotel(ctx context.Context, hotel *Hotel) error {
    // Write to DB AND cache simultaneously
    if err := s.repo.Update(ctx, hotel); err != nil {
        return err
    }
    s.cache.Set(ctx, "hotel:"+hotel.ID, hotel, 5*time.Minute) // always current
    return nil
}
```

### Cache Invalidation Strategies
```
TTL (Time-To-Live):         Cache expires after N seconds → simple, stale possible
Event-based invalidation:   On update, delete cache key → always fresh, more complex
Version-based:              Cache key includes version number → old keys never overwritten
```

### Cache Stampede / Thundering Herd
```go
// When many requests hit an expired cache simultaneously, all rush to DB
// Fix: Probabilistic early expiration OR mutex per key

var keyMutexes sync.Map

func (c *Cache) GetOrSet(ctx context.Context, key string, fn func() (interface{}, error)) (interface{}, error) {
    // Try cache first
    if val, found := c.get(key); found { return val, nil }
    
    // Only one goroutine populates per key
    mu, _ := keyMutexes.LoadOrStore(key, &sync.Mutex{})
    mutex := mu.(*sync.Mutex)
    mutex.Lock()
    defer mutex.Unlock()
    
    // Double-check after acquiring lock
    if val, found := c.get(key); found { return val, nil }
    
    val, err := fn() // fetch from DB
    if err != nil { return nil, err }
    c.set(key, val)
    return val, nil
}
```

---

## Interview Q&A

**Q: What is the difference between optimistic and pessimistic locking?**
> A: Pessimistic locking: acquire a lock before accessing data — `SELECT FOR UPDATE` in SQL. Other transactions block until the lock is released. Use when conflicts are frequent and you must prevent them. Optimistic locking: assume conflicts are rare — don't lock; instead add a `version` column. When updating, check that version hasn't changed since you read it. If it has, retry or return a conflict error. Use when conflicts are infrequent. For hotel booking, pessimistic locking on the inventory row is appropriate because conflicts (multiple people booking the last room) are real and must be prevented — the window is short enough that blocking is acceptable.

**Q: How would you handle database connection exhaustion?**
> A: Use connection pooling with appropriate limits: `SetMaxOpenConns` (typically 25-100, matching DB's max_connections), `SetMaxIdleConns` (same as max open), `SetConnMaxLifetime` (5-10 min, to handle DB-side connection termination). Monitor pool metrics: waitcount (requests waiting for a connection) and maxlifetimeclosed. If exhausted: add read replicas for read queries, cache more aggressively to reduce DB load, or use a connection pooler like PgBouncer which multiplexes app connections to fewer DB connections.

# Flight Pricing Aggregator — Detailed Design Walkthrough

## The problem, restated

Multiple suppliers expose **metered** (cost-per-call) flight inventory APIs. Users search by
origin/destination/dates. Requirements:
1. Show the **cheapest** price across suppliers for the same logical flight.
2. Prices/seats change frequently — show reasonably fresh data.
3. No supplier will push updates — you can only **pull**, and every pull costs money.

The entire problem reduces to one sentence: **how do you keep a cache fresh enough,
for cheap enough, when your only tool is a metered pull?** Everything in the design
answers that question from a different angle.

---

## Step-by-step design flow

### Step 1 — Scope before designing (say this out loud in the interview)
Ask:
- Approximate search QPS, and how skewed is demand (are 80% of searches on 20% of routes)?
- Acceptable staleness — is 2 minutes old okay, or does the business need near-real-time?
- Roughly how expensive is a supplier call, and is there a hard budget (calls/min) per supplier?
- Is booking in scope, or just search/browse?

This matters because the answer changes the design: a highly skewed demand curve justifies
aggressive caching of hot routes; a flat/long-tail demand curve pushes you toward
on-demand fetch with no pre-warming.

### Step 2 — Define the canonical data model
A flight is not "one row" — it's an aggregation point. Define:

```
FlightOfferKey = (origin, destination, date, airline, flight_number)

FlightOfferKey -> [
  { supplier_id: "A", price: 4200, seats: 3, fetched_at: t1 },
  { supplier_id: "B", price: 4050, seats: 1, fetched_at: t2 },
  { supplier_id: "C", price: 4300, seats: 0, fetched_at: t3 },
]
```

**Never pre-collapse this to a single "winning" row.** Store all supplier offers per key,
and pick the minimum price **at read time**. Why: if you pre-collapse and store only the
winner, you lose the ability to fall back when the winning supplier's seats hit zero, and
you can't cheaply detect "supplier B just got cheaper" without re-running the merge anyway.

### Step 3 — The read path (what happens on a user search)
1. User searches `(origin, dest, date range)`.
2. Search service queries the **Flight Offer Cache** for all `FlightOfferKey`s matching the route.
3. For each key, compute `min(price)` across supplier rows still within their freshness TTL.
4. If the route has **no cached data** or data is **past a hard staleness limit**: trigger
   a synchronous (bounded-wait) fetch to suppliers before responding — but use
   **single-flight** so concurrent identical searches don't each trigger their own fetch.
5. If data is **stale but within a soft limit**: return it immediately (stale-while-revalidate)
   and kick off an async background refresh for next time.

### Step 4 — The refresh path (what keeps the cache warm)
This is the part that actually solves the "metered API" constraint.

- **Demand tracking**: every search is logged (route, timestamp). A stream/aggregation job
  (could literally be your Airflow-style pipeline) computes a rolling demand score per route.
- **Poll scheduler**: a priority queue of `(route, supplier)` pairs ranked by demand score
  and current staleness. It decides *what to poll next* — not on a fixed timer, but weighted
  by how much it would matter if the data were fresher.
- **Budget manager**: a token bucket (or similar) per supplier enforces the metering
  constraint — e.g., "Supplier A allows 500 calls/min." The scheduler pulls the highest-priority
  route that still fits within the remaining budget for the relevant supplier.
- **Result**: hot routes stay warm proactively; cold/long-tail routes are only fetched
  reactively, when a real user searches for them (Step 3's read-through path).

### Step 5 — Resilience in the supplier adapter layer
Each supplier gets its own adapter with:
- **Independent timeout** tuned to that supplier's latency profile (don't share one global timeout).
- **Circuit breaker** — after N consecutive failures, stop calling that supplier for a cooldown
  window; degrade gracefully by serving results from the remaining suppliers.
- **Retry with backoff + jitter**, capped attempts, only for idempotent GET-style calls.

### Step 6 — The booking-time nuance (the part interviewers probe hardest)
Cached data is fine to *display*, but a user could book based on a price/seat count that's
seconds to minutes stale. At the moment of booking:
- Make a **synchronous, real-time verification call** to the specific winning supplier to
  reconfirm price and seat availability before finalizing.
- If price/availability changed: surface it to the user (classic "price changed, please
  confirm" UX) rather than silently booking at the old price.
- This is the one place where you deliberately spend metered budget outside the normal
  polling plan — it's justified because it directly gates a financial transaction.

---

## Flow diagram (text form — matches the diagram shown above)

```
                         ┌─────────┐
                         │  User   │  searches by route + date
                         └────┬────┘
                              │
                              v
                    ┌───────────────────┐        search logs
                    │  Search Service     │ ───────────────────►  ┌──────────────────┐
                    │  reads cache,        │                        │  Poll Scheduler    │
                    │  serves results       │                        │  demand-weighted,  │
                    └────────┬───────────┘                        │  budget-aware       │
                              │                                     └─────────┬─────────┘
                              v                                                │ tells adapter
                   ┌────────────────────────┐                                │ what to fetch
                   │  Flight Offer Cache      │                               │
                   │  per-key list of         │                               │
                   │  supplier offers,         │                               │
                   │  cheapest picked at        │                              │
                   │  read time                  │                             │
                   └────────────┬─────────────┘                               │
                                 │                                             │
                                 v                                             v
                   ┌──────────────────────────────────────────────────────────┐
                   │              Supplier Adapter Layer                        │
                   │   per-supplier timeout, circuit breaker, retry+backoff      │
                   └──────┬──────────────────┬──────────────────┬──────────────┘
                          v                  v                  v
                   ┌───────────┐      ┌───────────┐      ┌───────────┐
                   │ Supplier A │      │ Supplier B │      │ Supplier C │
                   │ metered API │      │ metered API │      │ metered API │
                   └───────────┘      └───────────┘      └───────────┘
```

---

## Terminology to have ready (with context for THIS problem)

| Term | What it means | Why it matters here |
|---|---|---|
| **Cache-aside (lazy loading)** | App checks cache first; on miss, fetches from source and populates cache | Your default pattern for cold/long-tail routes — fetch on demand, not proactively |
| **Read-through cache** | Cache itself is responsible for fetching on miss, transparent to caller | Cleaner separation — search service just asks the cache, doesn't know about suppliers |
| **Stale-while-revalidate** | Serve slightly-stale cached data immediately, refresh in background for next request | Lets you meet latency SLAs without blocking users on a live supplier call every time |
| **TTL (time to live)** | How long a cached value is considered fresh | Should differ per route based on demand/volatility — not one global TTL |
| **Single-flight (request coalescing)** | Only one of many concurrent identical requests actually executes the expensive call; others wait on its result | Prevents a burst of simultaneous searches for the same cold route from each burning a metered call |
| **Token bucket** | Rate-limiting algorithm — a bucket refills at a fixed rate, each call consumes a token, calls block/reject when empty | How you enforce "Supplier A allows 500 calls/min" cleanly |
| **Circuit breaker** | Trips after repeated failures, stops calling a failing dependency for a cooldown, allows trial requests to test recovery | Stops a slow/down supplier from degrading the whole search experience |
| **Bulkhead** | Isolating resource pools (threads, connections) per dependency so one bad dependency can't starve others | Each supplier adapter should have its own connection pool, not a shared one |
| **Idempotency key** | A unique key per logical operation so retries don't cause duplicate side effects | Relevant at booking time — retried booking confirmations shouldn't double-book |
| **Demand-weighted polling** | Prioritizing refresh effort by how much traffic/interest a route gets, not a fixed schedule | The core mechanism that makes a metered-API budget go where it matters |
| **Backpressure** | Signaling upstream to slow down when downstream can't keep up (vs. silently queuing forever) | Prevents the poll scheduler from queuing unbounded fetch requests during a demand spike |
| **Eventual consistency** | Data converges to correctness over time, but may be briefly stale/inconsistent | The honest framing for search results — you are NOT promising real-time truth, just "fresh enough" |
| **Canonical key / normalization** | Mapping different suppliers' inconsistent formats into one shared identity for the same real-world entity | Needed because Supplier A and B might describe the same flight with different field names/formats |

---

## Anticipated "why not" follow-ups and how to answer them

- **"Why not just poll every route on a fixed schedule?"** — Wastes budget on cold routes
  nobody searches, and won't scale as route count grows; demand-weighted polling spends
  the same budget where it actually reduces staleness that users experience.
- **"Why not cache the merged/cheapest result instead of all supplier rows?"** — You lose
  the ability to fall back to the next-cheapest when the winning supplier goes stale or
  runs out of seats, without re-fetching everything.
- **"Why not always fetch live on every search for accuracy?"** — Defeats the purpose of a
  metered API entirely; also adds latency to every search, most of which don't need
  second-level freshness.
- **"What if the demand-tracking job itself lags or fails?"** — Fall back to a default
  polling floor (e.g., poll top-N historically popular routes at a fixed baseline) so the
  system degrades gracefully rather than going cold everywhere.
- **"How do you handle a supplier changing schema/breaking the adapter?"** — Adapter layer
  isolates the blast radius to one supplier; contract/schema validation at the adapter
  boundary with alerting on parse failures, not silent swallowing.

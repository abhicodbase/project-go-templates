# 3-Day Platform Round Prep Plan

Treat this as a beginner-paced bootcamp — each day builds on the last, ends with a
self-check, and points to the exact files/examples already prepared for you.
Roughly 6-7 focused hours/day. Adjust blocks around your work schedule, but don't skip
the "self-check" at the end of each day — that's what actually tells you if you're ready.

---

# DAY 1 — Foundations: Connectivity, API, DB, gRPC

**Goal by end of day:** you can look at any API/DB code snippet and immediately spot the
5-6 recurring issue categories, and explain 3-4 connectivity patterns from memory.

### Morning block (3 hrs) — Learn the vocabulary
1. **(45 min)** Read `connectivity-patterns-and-final-checklist.md` fully once, slowly.
   Don't memorize yet — just get familiar with each term existing.
2. **(45 min)** Read the **Terminology table** in `flight-pricing-system-design.md`.
   For each term, say out loud (even to yourself): "this means X, and it matters because Y."
   Example: *"Circuit breaker — trips after repeated failures, stops calling a dead
   dependency — matters because without it, one slow supplier can hang my whole search."*
3. **(90 min)** REST vs gRPC vs GraphQL, deep dive:
   - Read the REST vs gRPC table again.
   - Write (by hand or in a notes doc) 3 sentences from memory: when you'd pick each.
   - Example answer to practice saying out loud: *"I'd use REST for our partner-facing
     supplier APIs since they need universal compatibility and easy debugging with curl.
     I'd use gRPC between our internal KYC and Cross-Sell services where I control both
     ends and want speed plus a strict contract."*

### Afternoon block (3 hrs) — Apply it to code
4. **(2 hrs)** Work through **Snippets 1, 2, 3, 4** from `connectivity-api-db-grpc-snippets.md`
   one at a time:
   - Set a 3-minute timer per snippet.
   - Read it cold, narrate out loud what it does.
   - Name every issue you can spot BEFORE reading the answer key.
   - Compare against the answer key — note what you missed.
   - Example of the narration habit to build: *"This opens a DB connection inside the
     function on every call — that's expensive per-call initialization, it should be a
     shared pooled connection injected at startup instead."*
5. **(1 hr)** Do the same for **Snippets 5 and 6** (webhook idempotency, API versioning).

### Evening self-check (30 min)
Without looking at any notes, answer these out loud, timed to ~1 min each:
- What's the difference between at-least-once and exactly-once delivery, and why does
  it matter for a webhook handler?
- Name 3 things wrong with opening a DB connection inside a request handler.
- When would you choose cursor-based pagination over offset/limit?
- What gRPC status code would you return for "record not found," and why not just a
  generic error?

If you can't answer 3 of 4 fluently, re-read that section before moving to Day 2.

---

# DAY 2 — System Design + Quality Assurance

**Goal by end of day:** you can take a vague system prompt, scope it with questions,
draw a box diagram while narrating, and describe a testing strategy for it — all inside
~15-20 minutes, the way the real round paces it.

### Morning block (3 hrs) — Design method + worked example
1. **(30 min)** Re-read `flight-pricing-system-design.md` Steps 1-6 once, focusing on
   the **order of reasoning**, not just the final answer: scope → data model → read
   path → refresh path → resilience → the booking-time nuance.
2. **(30 min)** Copy the **text flow diagram** from that file by hand (paper or
   Excalidraw). Practice re-drawing it from memory once, narrating each arrow as you draw:
   *"User hits search service, which reads the cache — if it's stale, the poll
   scheduler decides whether to refresh based on demand and budget..."*
3. **(2 hrs)** Do a **cold design run** on a fresh problem (don't reuse flight pricing).
   Use this one:

   > **Prompt:** "Design a system that shows real-time seat availability for a stadium
   > ticketing platform. Multiple ticket resellers list the same seats. You must never
   > show a seat as available if it's already sold, and avoid double-selling the same
   > seat to two buyers at once."

   Work through it yourself using the **same 6-step method**: scope questions → data
   model (what's the canonical "seat" key, similar to `FlightOfferKey`) → read path →
   how do you prevent double-sell (this is a concurrency/locking problem — reuse the
   "read-then-write race" pattern from the loan eligibility notes) → resilience →
   what's the booking-time nuance here (reserve + confirm pattern, similar to the
   flight booking re-verification call).
   Write your answer out, even roughly — the act of writing surfaces gaps.

### Afternoon block (2.5 hrs) — Testing strategy
4. **(45 min)** Re-read the **Testing strategy framework** (unit → integration →
   contract → E2E → non-functional) from the earlier notes. Memorize the 5-layer
   structure well enough to recite it in under 90 seconds.
5. **(1 hr)** Apply it to the stadium ticketing system you just designed:
   - What would you unit test? (e.g., the seat-locking logic in isolation)
   - What would you integration test? (e.g., reseller adapter against a sandboxed API)
   - What's the one critical E2E path? (buy a seat → confirm → seat marked sold)
   - What non-functional test matters most here? (concurrency/load test hammering the
     same seat with parallel buy requests — does your locking actually hold?)
6. **(45 min)** Practice the **"start with why"** framing the toolkit explicitly calls
   out: for each test type above, say one sentence on *why* it exists before describing
   it. Example: *"I'd write a concurrency test here because the core risk in this
   system isn't a single request failing, it's two requests racing — unit tests alone
   won't catch that."*

### Evening self-check (30 min)
- Recite your 5-layer testing pyramid from memory, unprompted.
- In under 3 minutes, sketch (paper is fine) the stadium ticketing HLD from memory —
  don't peek at your Day 2 notes first, then compare after.
- Say out loud: what's the single hardest failure mode in that system, and how does
  your design handle it?

---

# DAY 3 — Code Review, Communication, Full Mocks

**Goal by end of day:** you've run at least 2 full-length simulated rounds, in the
target format (live narration, interrupted with follow-ups, time-boxed), and you have
your leadership/behavioral stories ready cold.

### Morning block (2.5 hrs) — Code review criteria + practice
1. **(30 min)** Re-read the **Code review criteria framework** (correctness → failure
   handling → idempotency → readability → test coverage → performance → security).
   Practice reciting it in under 60 seconds, in that priority order — priority order
   matters because it shows you triage issues by severity, not just list them randomly.
2. **(2 hrs)** Timed drill: for each of the 8 snippets you've now seen across all
   files (Order Fulfillment, Webhook Processor, and the 6 connectivity/DB/gRPC ones),
   spend exactly 3 minutes per snippet:
   - Read cold.
   - Narrate what it does.
   - List issues **in priority order** (security/correctness first).
   - State one fix out loud per top issue.
   Do this for at least 4 snippets you haven't drilled yet, to build speed under a
   clock rather than re-reading ones you already know well.

### Afternoon block (3 hrs) — Full mocks
3. **(1 hr)** Run **Mock #1**: pick one system scenario (Booking Confirmation Pipeline,
   Loan Eligibility Engine, or a new one) and run it cold, live, narrating scope
   questions → design → diagram → testing plan → code review if a snippet is given.
   Set a real 45-50 minute timer to simulate the actual round's pacing pressure.
4. **(30 min)** Review: what did you get stuck on? Where did you ramble instead of
   being concise? Note it.
5. **(1 hr)** Run **Mock #2** on a different scenario, applying what you just learned
   about your own weak spots — this time explicitly practice the **time-boxing** habit:
   say out loud "let me flag the top issues here, then zoom out to the system level" at
   the 5-7 minute mark of any code review, the way the toolkit recommends.
6. **(30 min)** Prep your **2-3 leadership stories** (Scope → Decision → Tradeoff →
   Impact beyond your team), tied to your Cross-Sell Personal Loan or KYC work. Say each
   one out loud, timed to under 2 minutes — Staff-level interviewers will cut you off if
   you ramble past that.

### Evening — Final review (1 hr)
- Skim all 4 files once more, fast — you're not learning anything new now, just
  refreshing recall.
- Re-read the **anticipated "why not" follow-ups** section in the flight-pricing file
  and the **REST vs gRPC / webhook vs pub-sub vs polling** tables one final time —
  these are the highest-density "recite from memory" content.
- Get sleep. Pattern recognition under live pressure is what's being tested, and
  that degrades fast on low sleep.

---

## Quick reference: what to have "cold" by end of Day 3

- REST vs gRPC vs GraphQL — one sentence each, when to use.
- Webhook vs Pub/Sub vs Polling — one sentence each, when to use.
- Testing pyramid — 5 layers, recited in under 90 seconds.
- Code review criteria — 7 items, in priority order, under 60 seconds.
- 2-3 leadership stories, each under 2 minutes.
- The 6-step design method (scope → data model → read path → refresh/write path →
  resilience → the "nuance" the interviewer is fishing for).
- Top 8 code smells: SQL injection via string concat, swallowed errors, no
  timeout/context, hardcoded secrets, expensive per-call resource init, N+1 queries,
  missing idempotency, missing pagination.

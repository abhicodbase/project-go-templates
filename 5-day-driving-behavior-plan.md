# 5-Day Plan: From "Answering" to "Driving"

## The actual gap, named precisely

You have the technical depth — the feedback confirms that ("was able to answer").
What's missing is **ownership of the conversation's structure and pace**. A Senior
engineer answers well when asked. A Staff engineer makes the interviewer feel like
they're watching someone run a real design review — setting the agenda, deciding
what to cover and in what order, checking in, and pulling the conversation forward
instead of waiting for the next question.

This is entirely a matter of a few specific habits, practiced until automatic.

---

## The core technique: narrate your process, don't just produce answers

The single highest-leverage fix: **say what you're about to do before you do it, out
loud, every single time you transition.** This one habit covers most of "driving."

Compare:
- ❌ (answering): *[interviewer asks a question]* *[you answer it]* *[silence, waiting
  for next question]*
- ✅ (driving): *"Before I answer that, let me first walk through how I'd scope this
  — I'll ask a few questions, then propose a design, then we can dig into
  whichever part you want to go deeper on."* *[you do that, out loud, unprompted]*

Practice inserting these "process narration" lines at 5 specific moments in every
mock from now on:

1. **At the very start**, before any content: "Let me start by asking a few scoping
   questions before I propose anything."
2. **After scoping, before designing**: "Okay, based on that, here's how I'd
   structure my answer — I'll cover the data model first, then the read path, then
   resilience, then testing. Let me know if you want me to go deeper on any one of
   those instead of covering all of them."
3. **Mid-design, at natural section boundaries**: "That covers the read path — I want
   to move on to how writes work now, unless you'd like me to go deeper here first."
4. **After answering a follow-up "why not X" question**: "Good question — that's
   actually a tradeoff I should have flagged myself. Let me also mention two other
   tradeoffs I see in this design before we move on."
5. **Periodically, checking in**: "Does this level of detail match what you're
   looking for, or should I zoom in further on any part?"

---

## Day-by-day plan

### Day 1 — Diagnose and drill the opening moves in isolation

- Don't attempt a full mock yet. Instead, take 5 of the scenarios already prepped
  (airline aggregator, booking-agent review, flight pricing, supplier booking API,
  hotel proximity search) and for EACH one, practice ONLY the first 60 seconds —
  the scoping-questions opening — with the explicit goal of sounding like you're
  setting the agenda, not waiting to be told what to cover.
- Record yourself (voice memo) doing this 5 times. Listen back. The failure mode to
  catch: does it sound like a question, or like you announcing a plan and then
  asking questions as part of executing that plan? The second is what you want.
- Bad: "So... what's the traffic like?" (sounds passive, like you're stalling)
- Good: "I want to scope this before designing anything — let me ask a few things
  first: what's the traffic volume, is this read-heavy or write-heavy, and what's
  the consistency requirement?" (sounds like you're driving toward a plan)

### Day 2 — Full mocks with an explicit "driving checklist" self-grade

- Run 2 full mocks (pick 2 of the scenarios not yet used this week) end to end.
- After each, self-grade against this checklist — this is the actual skill being
  fixed, so grade it explicitly rather than just grading correctness:
  - [ ] Did I state my plan/structure before diving into content, at the start?
  - [ ] Did I narrate transitions between sections, or did I just stop and wait?
  - [ ] Did I proactively raise at least one tradeoff or risk WITHOUT being asked?
  - [ ] Did I check in at least once ("does this match what you're looking for")?
  - [ ] Did I ever go silent after answering, waiting passively for the next
        question? (This is the failure mode to eliminate entirely.)
- If you catch yourself going silent after an answer, that's the exact moment to
  practice filling with: "Let me also flag [related consideration] before we move
  on" — always have a next thing to say, don't hand control back by default.

### Day 3 — Practice with a harder constraint: assume the interviewer won't prompt you

- Run 2 more mocks, but this time explicitly ask whoever mocks with you (or imagine
  it yourself) to stay SILENT after each of your answers, for a full 5 seconds,
  rather than immediately asking the next question — this simulates an interviewer
  who is deliberately testing whether you'll drive or wait.
- The goal: don't let the silence make you uncomfortable into stopping. Fill it with
  your own next question, your own next tradeoff, your own transition. This is the
  single most realistic simulation of what a Staff-bar interviewer is actually
  testing for.

### Day 4 — Run the hotel/proximity-search HLD (the actual R4 content) with full driving discipline

- This is genuinely new content (geospatial search), so it's also the best test:
  can you drive a conversation about material you're less rehearsed in? That's
  closer to the real exam conditions than a scenario you've now drilled five times.
- Apply the same checklist from Day 2. Expect it to feel harder — that's fine, the
  content difficulty and the driving-behavior practice are separate skills, and
  you're deliberately stacking them here to pressure-test both at once.

### Day 5 — One full mock, cold, no checklist visible during it

- Run one final mock without looking at the checklist while doing it — only
  self-grade after. By now the checklist items should be happening automatically,
  not as a conscious script you're running through.
- If any item still isn't automatic, that's the one thing to consciously hold onto
  walking into the real round — better to consciously remember one thing than to
  have tried to remember five and executed none of them naturally.

---

## Phrases to have ready (the "driver's toolkit")

Keep these loose in memory — not to recite verbatim, but so the SHAPE of driving
language is available to you under pressure:

- Opening: "Before I propose anything, let me ask a few scoping questions."
- Structuring: "Here's how I'll walk through this — [X], then [Y], then [Z]."
- Transitioning: "That covers [X]. I want to move to [Y] next, unless you'd like to
  go deeper here first."
- Volunteering: "One tradeoff I want to flag before we move on is..."
- Checking in: "Does this match the depth you're looking for?"
- Recovering from a tough question: "Good question — let me think through that
  out loud rather than just giving you an answer" (buys time WHILE still sounding
  in control, versus silence which reads as stalling)
- Closing a topic: "So to summarize this part — [one sentence]. Should I go deeper
  on any of this, or move to [next topic]?"

---

## One reframe worth holding onto

The interviewer isn't grading whether you eventually arrive at correct answers —
you've already demonstrated you can. They're grading whether working with you would
feel like working with someone who takes ownership of ambiguous problems, or someone
who needs to be managed through them. That's genuinely what "Staff" means at most
companies: less "knows more," more "needs less direction." Five days is enough time
to make that visible, because the underlying knowledge is already there — this is
entirely about making the ownership behavior automatic under pressure.

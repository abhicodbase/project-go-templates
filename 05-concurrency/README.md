# Concurrency in Go

## Why Go's Concurrency Model?
Go uses **CSP (Communicating Sequential Processes)**: goroutines + channels.

> *"Do not communicate by sharing memory; instead, share memory by communicating."*

---

## Topics
1. [Goroutines & Channels](goroutines-channels.md) — The basics
2. [Sync Primitives](sync-primitives.md) — Mutex, RWMutex, WaitGroup, atomic
3. [Concurrency Patterns](concurrency-patterns.md) — Worker pool, fan-out/in, pipeline
4. [Context & Cancellation](context-cancellation.md) — context.Context

## Runnable Examples
- [`examples/worker_pool.go`](examples/worker_pool.go) — Bounded worker pool
- [`examples/rate_limiter.go`](examples/rate_limiter.go) — Token bucket rate limiter
- [`examples/pipeline.go`](examples/pipeline.go) — Pipeline pattern

## Key Interview Questions
- What is the difference between a goroutine and a thread?
- When would you use a channel vs a mutex?
- What is a goroutine leak and how do you prevent it?
- Explain the select statement
- What is a race condition and how do you detect it in Go?

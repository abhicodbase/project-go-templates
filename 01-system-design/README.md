# System Analysis & Design

> **Interview focus**: You'll be given a scenario with intentional gaps or faults. Analyze requirements, identify bottlenecks, and propose improvements.

## Topics
1. [Scalability](scalability.md) — Horizontal/vertical scaling, load balancing, statelessness
2. [Fault Tolerance](fault-tolerance.md) — Circuit breakers, retries, bulkheads, graceful degradation
3. [Microservices Patterns](microservices-patterns.md) — Service mesh, API gateway, sidecar
4. [Event-Driven Architecture](event-driven.md) — Event sourcing, CQRS, Kafka

## Case Studies (Practice These!)
- [Rate Limiter Design](case-studies/rate-limiter-design.md) ⭐
- [URL Shortener](case-studies/url-shortener.md)
- [Notification System](case-studies/notification-system.md)

## Key Framework: How to Approach Any System Design Question

```
1. CLARIFY   → Ask about scale, users, SLA, consistency requirements
2. ESTIMATE  → DAU, QPS, storage, bandwidth
3. DESIGN    → High-level components, data flow
4. DEEP-DIVE → Pick the most critical component, zoom in
5. TRADE-OFFS → Always discuss alternatives and why you chose this
6. BOTTLENECKS → Identify and address potential failure points
```

## Common Agoda-Context Scenarios
- Hotel search with high read load (caching, CDN)
- Booking system with distributed transactions
- Pricing engine with real-time updates
- Notification system for booking confirmations

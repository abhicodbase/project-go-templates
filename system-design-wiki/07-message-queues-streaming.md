# 07 — Message Queues & Event Streaming

## TL;DR
> Message queues decouple producers from consumers. Kafka is for high-throughput, ordered, replayable event streams.  
> RabbitMQ is for task queues and complex routing. Use async messaging to avoid tight coupling and handle traffic spikes.

---

## 1. Why Message Queues?

### Synchronous vs Asynchronous Communication
```
Synchronous (Tight Coupling):
  User ──POST /order──▶ Order Service
                        │──▶ Payment Service  (wait...)
                        │──▶ Inventory Service (wait...)
                        │──▶ Notification Service (wait...)
                        ◀── All done (3 services in serial = slow + fragile)

Asynchronous (Loose Coupling):
  User ──POST /order──▶ Order Service ──▶ Message Queue ──▶ ACK
         ◀── 202 Accepted                 │
                                          ├──▶ Payment Consumer (async)
                                          ├──▶ Inventory Consumer (async)
                                          └──▶ Notification Consumer (async)
```

### Benefits of Message Queues
| Benefit | Description |
|---------|-------------|
| **Decoupling** | Producer doesn't know or care about consumer |
| **Load Leveling** | Queue absorbs traffic spikes; consumers process at own pace |
| **Fault Isolation** | If consumer crashes, messages wait in queue (not lost) |
| **Retry** | Failed processing can be retried automatically |
| **Ordering** | Some queues guarantee FIFO ordering |
| **Fan-out** | One message → multiple consumers |

---

## 2. Core Concepts

### Producer / Consumer / Queue
```
Producer ──publish──▶ [Queue: order.created] ──consume──▶ Consumer
```

### Point-to-Point vs Pub/Sub
```
Point-to-Point (Queue):           Pub/Sub (Topic):
Producer ──▶ Queue ──▶ Consumer   Publisher ──▶ Topic ──▶ Subscriber A
            (one consumer gets             └──▶ Subscriber B
             each message)                └──▶ Subscriber C
```

### Message Delivery Guarantees
| Guarantee | Meaning | Risk |
|-----------|---------|------|
| **At Most Once** | Message delivered 0 or 1 times | Message may be lost |
| **At Least Once** | Message delivered 1+ times | Message may be duplicated |
| **Exactly Once** | Message delivered exactly 1 time | Hardest to achieve; needs idempotent consumers |

> **Design consumers to be idempotent** — assume at-least-once delivery; handle duplicates gracefully.

### Dead Letter Queue (DLQ)
```
Consumer fails to process message 3 times:
  ──▶ Message moved to DLQ (dead.orders)
  ──▶ Alert / manual investigation

Prevents poison messages from blocking the queue forever.
```

---

## 3. RabbitMQ

Traditional message broker with complex routing capabilities.

### Architecture
```
Producer ──▶ Exchange ──routing──▶ Queue A ──▶ Consumer A
                                 └─▶ Queue B ──▶ Consumer B
```

### Exchange Types
| Type | Routing | Use Case |
|------|---------|---------|
| **Direct** | Route by exact routing key | Specific worker pool |
| **Fanout** | Broadcast to all bound queues | Notifications to all services |
| **Topic** | Pattern-match routing key (*.error, logs.#) | Log routing by severity |
| **Headers** | Match on message headers | Complex routing rules |

### RabbitMQ Message Flow
```
1. Producer connects to RabbitMQ broker
2. Producer publishes to Exchange with routing key "order.created"
3. Exchange routes to matching queues based on bindings
4. Consumer fetches messages (push or pull)
5. Consumer sends ACK on success (or NACK on failure → requeue or DLQ)
```

### Acknowledgement & Requeue
```
Consumer receives message:
  ├── Success → ACK → message deleted from queue
  ├── Failure → NACK + requeue=true → back to queue for retry
  └── Failure → NACK + requeue=false → moved to DLQ
```

---

## 4. Apache Kafka

Distributed event streaming platform designed for high throughput, durability, and replay.

### Kafka Core Architecture
```
                    ┌───────────────────────────────────────┐
                    │              Kafka Cluster             │
                    │                                       │
Producer ──▶        │  Topic: "orders" (3 partitions)      │  ──▶ Consumer Group A
                    │    Partition 0: [msg1][msg3][msg5]    │      Consumer 1 ← P0
                    │    Partition 1: [msg2][msg4][msg6]    │      Consumer 2 ← P1
                    │    Partition 2: [msg7][msg8][msg9]    │      Consumer 3 ← P2
                    └───────────────────────────────────────┘
                                                             ──▶ Consumer Group B
                                                                  (reads same topic)
```

### Key Concepts

**Topic**: Named stream of records (like a table in a database)

**Partition**: Topic divided into ordered, immutable log segments
```
Partition 0: offset 0  offset 1  offset 2  offset 3  ...
              [msg1]   [msg2]   [msg3]   [msg4]
                                               ▲
                                         Consumer offset (reads from here next)
```

**Offset**: Position of a message within a partition. Consumers track their own offset.

**Consumer Group**: Set of consumers that cooperatively read a topic (each partition assigned to one consumer in the group)

**Broker**: One Kafka server node. Cluster = multiple brokers.

**Replication**: Each partition has 1 leader + N followers (replicas).

### Kafka vs RabbitMQ

| Feature | Kafka | RabbitMQ |
|---------|-------|----------|
| Model | Distributed log (pull-based) | Message broker (push-based) |
| Retention | Messages kept for configurable period (days/weeks) | Deleted after ACK |
| Replay | Yes (seek to any offset) | No (once consumed, gone) |
| Throughput | Millions of msg/sec | Hundreds of thousands/sec |
| Ordering | Per-partition ordering | Per-queue ordering |
| Routing | Simple (partition by key) | Complex (topic/direct/fanout) |
| Consumer model | Consumers pull at own pace | Broker pushes to consumers |
| Use case | Event streaming, audit logs, analytics pipeline | Task queues, RPC, complex routing |

### Kafka Message Flow (Order Processing Example)
```
Order Service ──▶ "orders" topic ──▶ Payment Service (offset: 0)
                                 ──▶ Inventory Service (offset: 0)
                                 ──▶ Analytics Service (offset: 0)

Each consumer group maintains its own offset — can replay independently.
```

### Kafka Partitioning Strategy
```
Kafka assigns messages to partitions by:
  1. Explicit partition key (use user_id to group user's events on same partition)
  2. Round-robin (no key)

// Go producer with partition key
msg := &sarama.ProducerMessage{
    Topic: "orders",
    Key:   sarama.StringEncoder(order.UserID),  // same user → same partition → ordered
    Value: sarama.StringEncoder(orderJSON),
}
```

### Kafka Guarantees
- **Within a partition**: messages are ordered
- **Across partitions**: no ordering guarantee
- **At-least-once** by default (enable idempotent producer for exactly-once)
- **Retention**: configurable (e.g., keep 7 days or 100 GB, whichever first)

---

## 5. Event-Driven Architecture

Systems communicate via events rather than direct API calls.

```
Traditional (Orchestration):
  Order Service directly calls Payment, Inventory, Notification services
  → Tight coupling, synchronous, if one fails all fail

Event-Driven (Choreography):
  Order Service emits "OrderPlaced" event
  Payment Service listens → processes payment → emits "PaymentCompleted"
  Inventory Service listens to "OrderPlaced" → reserves stock → emits "StockReserved"
  Notification Service listens to "PaymentCompleted" → sends email
  → Loose coupling, each service knows its own job
```

### Event Sourcing
Store every state change as an **event** (append-only log), not the current state.
```
Instead of:
  orders table: { id: 1, status: "SHIPPED", amount: 100 }

Event log:
  event 1: OrderPlaced  { order_id: 1, amount: 100 }
  event 2: PaymentTaken { order_id: 1, amount: 100 }
  event 3: OrderShipped { order_id: 1, tracking: "XYZ" }

Current state = replay all events
```
**Benefits**: Audit trail, replay, time-travel debugging, multiple projections from same events
**Trade-off**: Read complexity (must replay or maintain projections)

---

## 6. Common Queue Patterns

### Work Queue (Task Distribution)
```
Producer ──▶ Queue ──▶ Worker 1 (processing task A)
                   ──▶ Worker 2 (processing task B)
                   ──▶ Worker 3 (processing task C)

Use: image resizing, email sending, video transcoding
```

### Fan-Out (Pub/Sub)
```
Order Service ──▶ "order.placed" topic
                    │──▶ Email Service
                    │──▶ SMS Service
                    │──▶ Analytics Service
                    └──▶ Fraud Detection Service
```

### Request-Reply (Async RPC)
```
Client ──▶ request queue ──▶ Server (processes)
Client ◀── reply queue   ◀── Server (responds)

Client includes reply-to queue name and correlation ID in request.
```

### Priority Queue
```
Priority 1 (HIGH): payment failures ──▶ processed first
Priority 2 (MED):  new orders
Priority 3 (LOW):  analytics events
```

---

## 7. Choosing Between Queue Systems

| Scenario | Recommendation |
|----------|---------------|
| Task queue with retries, routing | **RabbitMQ** |
| High-throughput event stream | **Kafka** |
| Real-time analytics pipeline | **Kafka** |
| Simple job queue in AWS | **SQS** |
| Managed Kafka in cloud | **AWS MSK**, **Confluent Cloud** |
| IoT device messages | **AWS IoT Core**, **MQTT** |
| Simple pub/sub in GCP | **Google Cloud Pub/Sub** |

---

## Key Takeaways

1. **Decouple** with queues — producers and consumers evolve independently
2. **Design for at-least-once** delivery; make consumers **idempotent**
3. **DLQ** for failed messages — don't silently drop
4. **Kafka** for event streams you need to replay/audit; **RabbitMQ** for task queues with routing
5. **Partition key** determines ordering in Kafka — choose wisely (e.g., user_id for user events)
6. Event-driven (choreography) reduces coupling vs direct service calls (orchestration)
7. **Consumer groups** in Kafka allow multiple independent processors of the same stream

# Kafka Internals & Concepts

## Core Concepts

### Topic, Partition, Offset
```
Topic: "booking-events"
  Partition 0: [offset:0 | offset:1 | offset:2 | offset:3 → ]
  Partition 1: [offset:0 | offset:1 | offset:2 → ]
  Partition 2: [offset:0 | offset:1 → ]

- Topic: logical stream of records (like a table)
- Partition: unit of parallelism — each partition is ordered
- Offset: unique, monotonically increasing ID within a partition
- Ordering: guaranteed WITHIN a partition, NOT across partitions
```

### Producers
```go
// Key determines partition assignment (same key → same partition)
// Use meaningful keys: booking_id, hotel_id, user_id
producer.Produce(&kafka.Message{
    TopicPartition: kafka.TopicPartition{Topic: "booking-events", Partition: kafka.PartitionAny},
    Key:   []byte("booking-123"),   // same key always goes to same partition
    Value: []byte(`{"type":"BookingCreated","id":"booking-123"}`),
    Headers: []kafka.Header{
        {Key: "event-type", Value: []byte("BookingCreated")},
        {Key: "correlation-id", Value: []byte(correlationID)},
    },
})
```

### Consumer Groups
```
Topic: "booking-events" (3 partitions)
Consumer Group: "notification-service"
  Consumer A → Partition 0
  Consumer B → Partition 1
  Consumer C → Partition 2

Each consumer group maintains its OWN offset per partition
→ Multiple groups can independently consume the same topic

If Consumer A dies:
  Kafka rebalances: Consumer B → Partition 0 + 1
                    Consumer C → Partition 2
```

---

## Delivery Guarantees

### At-Most-Once
```
Commit offset BEFORE processing
→ If crash after commit but before processing: message LOST
→ Fastest, lowest latency
→ Use for: metrics, non-critical events
```

### At-Least-Once (Most Common)
```
Commit offset AFTER processing
→ If crash after processing but before commit: message REDELIVERED
→ Consumer must be IDEMPOTENT
→ Use for: most business events
```

### Exactly-Once (Kafka Transactions)
```
Use Kafka transactions + idempotent producer
→ Expensive, complex
→ Use for: financial transactions, inventory updates
```

---

## Consumer Rebalancing

When consumers join/leave, Kafka reassigns partitions.

**Problem**: During rebalance, no consumer is processing → latency spike
**Solution**: Use incremental cooperative rebalancing (Kafka 2.4+) — only moves partitions that need to change

```go
// Always commit before rebalance
c.SubscribeTopics([]string{"booking-events"}, func(c *kafka.Consumer, event kafka.Event) error {
    switch e := event.(type) {
    case kafka.AssignedPartitions:
        log.Info("Partitions assigned", "partitions", e.Partitions)
        c.Assign(e.Partitions)
    case kafka.RevokedPartitions:
        // Commit current offsets before giving up partitions
        c.Commit()
        c.Unassign()
    }
    return nil
})
```

---

## Key Configuration (Production)

### Producer
```
acks=all          → Wait for all replicas to acknowledge (most durable)
retries=10        → Retry on transient failures
enable.idempotence=true → Prevent duplicates on retry
compression.type=snappy → Reduce network/disk usage
```

### Consumer
```
auto.offset.reset=earliest    → On new group, start from beginning
enable.auto.commit=false      → Manual commit (for at-least-once control)
max.poll.interval.ms=300000   → Max time between polls before considered dead
session.timeout.ms=45000      → Time before consumer considered failed
```

---

## Partitioning Strategy

```
Booking events → partition by booking_id
  → All events for booking-123 go to same partition
  → Ordered processing per booking guaranteed

Hotel events → partition by hotel_id
  → All updates for hotel-456 go to same partition

Notification events → partition by user_id
  → User's notifications arrive in order

❌ Don't use null key → round-robin → no ordering guarantee
```

---

## Interview Q&A

**Q: What happens to messages when a consumer in a group crashes?**
> A: Kafka detects the crash when the consumer stops sending heartbeats (within `session.timeout.ms`, typically 30-45s). Kafka then triggers a rebalance — the partitions that were assigned to the crashed consumer are reassigned to the surviving consumers in the group. The new consumers continue from the last committed offset of those partitions. This means if the crashed consumer had processed messages but not committed their offsets yet, those messages will be redelivered — at-least-once delivery. To minimize redelivery: commit frequently and make consumers idempotent.

**Q: How do you ensure message ordering in Kafka?**
> A: Ordering is guaranteed only within a single partition. To ensure all related messages are ordered: use a consistent key (e.g., `booking_id`) so all messages for that booking always go to the same partition. Within a consumer, you process messages sequentially. If you need global ordering across all messages in a topic, use a single partition — but this kills parallelism. The design trade-off: choose a key that gives good distribution (avoid hot partitions) while guaranteeing order for related events.

**Q: What is a Dead Letter Queue (DLQ) in Kafka?**
> A: A DLQ is a separate topic where messages that failed processing are moved instead of blocking the consumer. If a consumer fails to process a message after N retries, it publishes the failed message to a DLQ topic (e.g., `booking-events-dlq`) and commits the original offset. A separate consumer (or operator) monitors the DLQ, investigates failures, and either retries, fixes, or discards. Without a DLQ, one bad message can block all processing for a partition indefinitely.

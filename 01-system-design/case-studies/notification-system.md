# Case Study: Notification System Design

## Problem Statement
Design Agoda's notification system that sends:
- Booking confirmations (email + SMS)
- Price alerts
- Promotional messages
- At scale: 10M notifications/day

---

## Clarifications to Ask
- Delivery guarantees? At-least-once
- Latency? Booking confirmations < 30s; promotions best-effort
- Channels? Email, SMS, Push notification, WhatsApp
- User preferences? Some users opt-out of SMS
- Template management? Yes, per language/channel

---

## Architecture

```
Booking Service ──► Kafka ──► Notification Service ──► Channel Router
                                                             │
                                                   ┌────────┼────────┐
                                                   ▼        ▼        ▼
                                                  Email    SMS      Push
                                                (SendGrid)(Twilio)(FCM/APNs)
```

## Key Components

### 1. Event Producer (Booking Service)
```go
// Publish notification event to Kafka
type NotificationEvent struct {
    EventID    string          `json:"event_id"`
    Type       string          `json:"type"`    // "booking_confirmed"
    UserID     string          `json:"user_id"`
    BookingID  string          `json:"booking_id"`
    Data       json.RawMessage `json:"data"`
    Priority   string          `json:"priority"` // "high", "normal"
    Timestamp  time.Time       `json:"timestamp"`
}
```

### 2. Notification Service (Consumer + Orchestrator)
```go
func (s *NotificationService) HandleEvent(ctx context.Context, event NotificationEvent) error {
    // 1. Load user notification preferences
    prefs, err := s.prefsRepo.Get(ctx, event.UserID)
    
    // 2. Determine channels
    channels := s.router.Route(event.Type, prefs)
    
    // 3. Load template and render
    template, _ := s.templateRepo.Get(ctx, event.Type, prefs.Language)
    rendered, _ := s.renderer.Render(template, event.Data)
    
    // 4. Send via each channel (idempotent with deduplication key)
    for _, ch := range channels {
        s.sender.Send(ctx, SendRequest{
            Channel:        ch,
            To:             prefs.Contacts[ch],
            Content:        rendered,
            DeduplicationKey: event.EventID + ":" + ch,
        })
    }
    return nil
}
```

### 3. Deduplication (Idempotency)
```go
// Check if already sent before calling external API
func (s *Sender) Send(ctx context.Context, req SendRequest) error {
    key := "sent:" + req.DeduplicationKey
    set, err := s.redis.SetNX(ctx, key, "1", 24*time.Hour).Result()
    if err == nil && !set {
        log.Info("duplicate notification skipped", "key", req.DeduplicationKey)
        return nil // Already sent
    }
    return s.externalAPI.Send(ctx, req)
}
```

---

## Trade-offs

| Decision | Reasoning |
|----------|-----------|
| Kafka for events | Durable, replayable, handles spikes |
| Separate queues per priority | High-priority (booking) never blocked by bulk promo |
| Deduplication in Redis | Prevent duplicate sends on retry |
| Template service | Centralized i18n and template management |
| Dead Letter Queue | Failed notifications don't block others; can retry later |

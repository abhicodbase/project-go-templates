# 12 — Real-time Systems

## TL;DR
> WebSockets for full-duplex real-time (chat, gaming). SSE for server-to-client streams (live feeds).  
> Use a Pub/Sub backend (Redis, Kafka) to fan-out messages across multiple server instances.  
> Notification systems need fan-out-on-write vs fan-out-on-read trade-off.

---

## 1. Real-time Communication Patterns

| Pattern | Direction | Protocol | Best For |
|---------|-----------|----------|---------|
| **Long Polling** | Server → Client | HTTP | Simple notifications, legacy support |
| **SSE** | Server → Client | HTTP | Live feeds, dashboards, notifications |
| **WebSocket** | Full-duplex | WS | Chat, gaming, collaboration |
| **WebRTC** | Peer-to-peer | UDP | Video calls, P2P file transfer |
| **gRPC Streaming** | Full-duplex | HTTP/2 | Internal microservice streaming |

---

## 2. Chat System Design

### Architecture Overview
```
Client A ──WS──▶ Chat Server 1 ──publish──▶ [Redis Pub/Sub] ──subscribe──▶ Chat Server 2 ──WS──▶ Client B
                 (A is connected here)                          (B is connected here)
```

### Key Components

**Connection Manager**: tracks which user is connected to which server
```
Redis Hash:
  HSET user_connections user_id_123 server_1
  HSET user_connections user_id_456 server_2
```

**Message Flow (1-to-1 chat)**:
```
1. Client A sends message: { to: "user_456", text: "Hello!" }
2. Chat Server 1 receives message
3. Looks up: user_456 is on Server 2 (via Redis)
4. Server 1 publishes to Redis channel: "user:456"
5. Server 2 subscribed to "user:456" → receives message
6. Server 2 delivers to Client B via WebSocket
7. Message persisted to DB (asynchronously)
```

### Group Chat Fan-out
```
Group message sent to group_id:abc (1000 members):
  Option 1: Fan-out-on-write
    Server iterates all 1000 member IDs
    Publishes to each member's channel
    → Fast read, expensive write

  Option 2: Fan-out-on-read
    Store message once with group_id
    Each member fetches on next poll/scroll
    → Cheap write, expensive read (need to query per group)

  Hybrid: Fan-out-on-write for small groups, fan-out-on-read for large groups (>1000 members)
```

### Online Presence
```
Client ──▶ WebSocket connected → mark user ONLINE in Redis (with TTL 30s)
Client sends heartbeat every 20s → refresh TTL
WebSocket disconnects → Redis TTL expires → user OFFLINE

// Redis:
SET presence:user_123 online EX 30  // 30-second TTL
SETEX presence:user_123 30 online   // same thing

// Check: GET presence:user_123 → "online" or nil (offline)
```

### Message Persistence & History
```
Messages stored in:
  - Cassandra or DynamoDB (write-heavy, time-ordered, partition by conversation_id)
  - Schema: { conversation_id, message_id (TimeUUID), sender_id, content, timestamp }

History retrieval:
  SELECT * FROM messages WHERE conversation_id = ? ORDER BY timestamp DESC LIMIT 50;
```

---

## 3. Live Feed / News Feed

Twitter-style home timeline with real-time updates.

### Fan-out-on-Write (Push Model)
```
User A posts tweet:
  For each of A's 1000 followers:
    Prepend tweet_id to follower's timeline cache (Redis List)

Read timeline:
  GET user:456:timeline → [tweet_3, tweet_7, tweet_1, ...]  (fast! pre-computed)
```

**Pros**: O(1) read, instant delivery
**Cons**: Write is O(followers) — celebrities with 100M followers cause huge fan-out

### Fan-out-on-Read (Pull Model)
```
User A posts tweet → stored in A's tweet store only

Read timeline (user 456):
  Fetch list of who user 456 follows: [A, B, C, ...]
  For each: fetch latest tweets
  Merge and sort by timestamp

→ Very expensive for active users following many people
```

### Hybrid (Twitter's Approach)
- Regular users (< 1M followers): **fan-out-on-write** (fast reads)
- Celebrities (> 1M followers): **fan-out-on-read** (avoid massive write storms)
- Timeline = pre-computed part + merge celebrity tweets at read time

---

## 4. Notification System

### Architecture
```
                    ┌──────────────────────┐
Events ────────────▶│  Notification Service│
(order placed,      │                      │──▶ Push (FCM/APNs)
 payment done, etc) │  Fan-out worker      │──▶ Email (SendGrid)
                    │  Rate limiting       │──▶ SMS (Twilio)
                    │  Preference check    │──▶ In-app (WebSocket)
                    └──────────────────────┘
```

### Components

**Event Source**: any service publishes events to Kafka
```
Order Service → "order.confirmed" event → Kafka
```

**Notification Service**:
1. Consumes event from Kafka
2. Looks up user notification preferences
3. Determines which channels to use (push, email, SMS)
4. Enqueues per-channel jobs

**Push Notification Flow**:
```
Notification Service ──▶ FCM/APNs ──▶ User's Device
```

**Email Flow**:
```
Notification Service ──▶ Email Queue ──▶ Email Provider (SendGrid) ──▶ User's Inbox
```

### Notification Preferences
```json
{
  "user_id": "123",
  "preferences": {
    "order_updates": ["push", "email"],
    "promotions": ["email"],
    "system_alerts": ["push", "sms", "email"]
  }
}
```

### Rate Limiting Notifications
```
Prevent notification spam:
  Max 1 email per user per hour for same notification type
  Max 10 push notifications per user per day

Implementation:
  Redis: INCR notify:email:user_123:order_update → check < 1/hour
```

---

## 5. Live Dashboard / Real-time Metrics

### Architecture
```
Events → Kafka → Stream Processor (Flink/Kafka Streams) → Aggregated Results
                                                          │
                                                          ├── Redis (hot data)
                                                          └── TimescaleDB (historical)

Dashboard:
  WebSocket/SSE ← Server ← Redis (poll every 1-5 seconds)
```

### Time-Window Aggregations
```
Tumbling window (fixed, non-overlapping):
  [0s-60s]: count = 450
  [60s-120s]: count = 523

Sliding window (moving):
  [0s-60s]: 450
  [1s-61s]: 460
  [2s-62s]: 455
  (updated every second)

Session window (event-based):
  Groups events by user session (gap > 30min = new session)
```

---

## 6. Real-time Gaming

### Requirements
- Ultra-low latency (< 50ms)
- Ordered game events
- State synchronization across players

### Architecture
```
Player A ──UDP──▶ Game Server ──▶ Player B
Player B ──UDP──▶ Game Server ──▶ Player A

Game state broadcast every 20ms (50 fps)
```

### Why UDP (not TCP) for Gaming?
```
TCP: waits for lost packet → retransmit → HOL blocking (stale game state arrives late)
UDP: lost packet? just ignore it (old position data is useless anyway)
     Application implements its own reliability for critical events only
```

### Techniques
- **Client-side prediction**: simulate movement locally, reconcile with server
- **Lag compensation**: server simulates what client saw when they fired
- **Delta compression**: only send what changed, not full state
- **Interpolation**: smooth out position between server updates

---

## 7. WebRTC (Video Calls)

Peer-to-peer real-time communication in the browser.

```
Step 1: Signaling (via your server)
  Client A ──offer SDP──▶ Signaling Server ──▶ Client B
  Client A ◀──answer SDP── Signaling Server ◀── Client B

Step 2: ICE (find optimal path)
  STUN server: discover public IP/port
  TURN server: relay if direct P2P fails (symmetric NAT)

Step 3: Direct P2P (once connected)
  Client A ◀──────── UDP media stream ──────▶ Client B
           (bypasses your server — just NAT traversal)
```

**Scalability problem**: P2P doesn't work for group calls (N*(N-1)/2 connections).
**Solution**: SFU (Selective Forwarding Unit) — central server receives and forwards streams.
```
Each client: 1 upload stream to SFU + N-1 download streams from SFU
SFU: routes media without transcoding (very efficient)
```

---

## 8. Handling Reconnections

Real-time connections drop. Handle gracefully.

```
Client reconnect flow:
1. Detect disconnect (WebSocket error / timeout)
2. Exponential backoff: retry after 1s, 2s, 4s, 8s, 16s (+ jitter)
3. On reconnect: send last_seen_event_id
4. Server: replay missed events since last_seen_event_id
5. Client: apply missed events to current state

// SSE built-in reconnect:
data: {...}
id: 1234       ← Last-Event-ID header sent on reconnect
```

### Message Buffer for Offline Users
```
User goes offline at 10:00 AM
Messages sent to user: stored in Redis list (max 100) or DB

User reconnects at 10:05 AM:
  Server checks: "what's the last message user received?"
  Replays all missed messages in order
```

---

## Key Takeaways

1. **WebSocket** requires pub/sub backend (Redis/Kafka) to scale across multiple server instances
2. **Fan-out-on-write** for small follower counts; **fan-out-on-read** for celebrities
3. **Online presence** = Redis key per user with short TTL + heartbeat
4. **Chat history** in Cassandra/DynamoDB: partition by conversation, order by timestamp
5. **Notification system**: Kafka → consumer → per-channel workers (push/email/SMS) + preference check
6. **Gaming**: UDP for low latency; client-side prediction for smooth UX
7. **WebRTC**: STUN for NAT traversal, SFU for group calls
8. Always handle **reconnection + message replay** — connections drop in the real world

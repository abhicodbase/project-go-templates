# 29 — Figma / Excalidraw: Collaborative Design Tool

> **Interview context**: 60-minute HLD. WebSockets, Canvas vs SVG, 5K-10K concurrent users on same document, conflict resolution, real-time collaboration.  
> Reference: https://www.youtube.com/watch?v=1lNJVDfsTSo&t=2231s

---

## 1. Requirements Clarification

### Functional Requirements
- Multiple users **simultaneously edit** the same design document
- Real-time cursor sharing (see where others' mouse is)
- Operations: draw shapes, move, resize, delete, group, text, images
- **Undo/redo** per user (not global undo!)
- Comments on canvas
- Version history (snapshots)
- Export: PNG, SVG, PDF

### Non-Functional Requirements
```
Concurrent users per document: up to 10,000 (e.g., company-wide template)
Typical session: 5-10 concurrent editors (most common)
Latency: cursor/edit updates visible to others in < 100ms
Document size: up to 10,000 elements per canvas
Scale: 50M users, 10M active documents
Consistency: eventual consistency acceptable (brief visual glitches OK)
             Conflicts must be resolved deterministically
```

---

## 2. Canvas vs SVG — Why Canvas?

```
SVG (Vector):
  DOM elements for each shape (each rect, circle, path is an HTML element)
  Pros: search engines can index, CSS styling, accessible
  Cons: 10,000 elements → 10,000 DOM nodes → extremely slow rendering
        DOM manipulation for each edit → expensive reflow/repaint

Canvas (HTML5 2D/WebGL):
  Single pixel buffer, you draw imperatively (ctx.fillRect, ctx.arc)
  Pros: constant rendering cost regardless of element count
        GPU-accelerated (WebGL) → smooth 60fps for 100K+ elements
        Custom hit-testing logic
  Cons: no built-in accessibility, no "select element" API

Decision: Canvas wins for large documents (10K+ elements)

Figma's approach: WebGL rendering engine (custom, Rust/WebAssembly)
Excalidraw: Canvas 2D with custom element tree in JavaScript
```

---

## 3. Conflict Resolution: CRDT vs OT

### Operational Transformation (OT) — Google Docs approach
```
User A: "Move rect_1 to position (100, 200)"
User B: "Move rect_1 to position (150, 300)"  ← simultaneous

OT: transform B's operation relative to A's:
  "Given A moved rect_1 to (100,200), where should B's move land?"
  → Apply B's delta on top of A's result

Problem: OT requires a central server to:
  1. Receive all operations
  2. Apply them in order
  3. Transform and send transformed ops to clients

→ Central server = bottleneck for 10K concurrent users
```

### CRDT (Conflict-free Replicated Data Types) — Better for scale
```
Each element is a CRDT: last-writer-wins (LWW) or position CRDT

For canvas elements: LWW-Register per property
  element.x = LWW-Register   ← whoever writes last with higher lamport timestamp wins
  element.y = LWW-Register
  element.width = LWW-Register

When two users move the same element simultaneously:
  User A sets: rect_1.x = 100, timestamp = 1000
  User B sets: rect_1.x = 150, timestamp = 1001
  
  After sync: both clients apply: rect_1.x = 150 (B wins, higher timestamp)
  → Deterministic resolution without coordination

For text editing: RGA (Replicated Growable Array) or Y.js
  Each character has a unique ID
  Insertions are positional: "insert char 'H' after char_id_42"
  Never conflicts even with simultaneous inserts
```

### Yjs (Production CRDT Library — used by Excalidraw)
```javascript
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'

const doc = new Y.Doc()
const shapes = doc.getMap('shapes')    // CRDT Map

// Connect to collaboration server
const provider = new WebsocketProvider('wss://collab.figma.com', 'doc-id', doc)

// All edits automatically synced to all clients:
shapes.set('rect_1', { x: 100, y: 200, width: 300, height: 150 })

// Observe changes from other clients:
shapes.observe(event => {
  event.changes.keys.forEach((change, key) => {
    if (change.action === 'add' || change.action === 'update') {
      renderShape(key, shapes.get(key))
    }
  })
})
```

---

## 4. Architecture

```
┌───────────────────────────────────────────────────────────────────┐
│                     CLIENT (Browser)                               │
│                                                                    │
│  Canvas Renderer (WebGL/Canvas2D)                                 │
│  + Yjs CRDT document (local copy)                                 │
│  + WebSocket connection to Collaboration Server                   │
│  + Local operation log (for undo/redo)                            │
└────────────────────────────┬──────────────────────────────────────┘
                             │ WebSocket (persistent connection)
┌────────────────────────────▼──────────────────────────────────────┐
│               COLLABORATION SERVER LAYER                           │
│                                                                    │
│  WebSocket Server (stateful — clients grouped by doc_id)         │
│  Receives: {doc_id, user_id, operation: CRDT delta}              │
│  Broadcasts to: all other connections for same doc_id            │
│                                                                    │
│  Problem: 10K users on same doc → 10K connections on 1 server?   │
│  Solution: Shard by doc_id (consistent hashing)                  │
│    All connections for doc:abc → always go to Server Node 7      │
│    → Server 7 has all 10K connections → can broadcast in O(1)    │
│                                                                    │
│  CRDT state stored: Redis (hot) + PostgreSQL (persistent)         │
└────────────────────────────┬──────────────────────────────────────┘
                             │ Persist
┌────────────────────────────▼──────────────────────────────────────┐
│                     STORAGE LAYER                                  │
│                                                                    │
│  PostgreSQL: documents, users, permissions                        │
│  Redis: active document state (last 10 minutes of ops)            │
│  S3: document snapshots (JSON), exported files (PNG/SVG/PDF)      │
│  Elasticsearch: document search, element search                   │
└───────────────────────────────────────────────────────────────────┘
```

---

## 5. Handling 5K-10K Concurrent Users on One Document

```
Challenge: 10,000 users each sending 10 ops/sec = 100,000 operations/sec
           One WebSocket server can handle ~50K concurrent connections
           But broadcasting: 1 op → 9,999 other clients = 10K sends/op × 100K ops = 1B sends/sec

Mitigation strategies:

1. Cursor updates: don't broadcast every mouse move
   Client-side throttle: send cursor position at most 60ms intervals
   Server: broadcast cursors to all → use UDP-like fire-and-forget (lossy OK)

2. Operation batching:
   Buffer operations for 16ms (1 frame) → send as batch
   → Reduces ops/sec from 10/user to 1 batch/user/16ms

3. Viewport-based filtering:
   Only send operations that affect the current viewport of each user
   User viewing page 3 → don't send ops on page 1

4. "Presence awareness" instead of full cursor broadcast:
   Show user avatars (not exact cursors) for users not in current viewport

5. Server-side fan-out scaling:
   For mega-documents (10K users): use pub-sub at server level
   WebSocket Server 7 (owns doc:abc) → publishes to Redis Pub/Sub
   Other WebSocket servers also subscribe → forward to their local connections
   → Horizontally scalable fan-out
```

---

## 6. Undo/Redo Per User

```
Global undo (problematic): 
  User A draws circle → User B moves square → User A presses Ctrl+Z
  Global undo: undoes B's move (not A's circle!) → BAD UX

Per-user undo (correct):
  Each user has their own operation stack (stored client-side)
  Undo: reverse the user's last operation
  
  Implementation with CRDT:
    Each operation has user_id tag
    Undo: generate inverse operation (move back, restore deleted element)
    Broadcast inverse operation → applied by all clients
  
  Challenge: What if B modified A's circle after A drew it?
    A draws circle → B resizes circle → A presses undo
    Correct: A's undo removes circle → B's resize target doesn't exist
    → Element deleted, B's edit is implicitly undone too
    → This is acceptable behavior (and what Figma does)
```

---

## 7. Version History (Snapshots)

```
Continuous snapshot approach:
  Every N operations (e.g., 100 ops) OR every 5 minutes:
    Serialize full document state → compress → store in S3
    
  Snapshot: { doc_id, snapshot_id, timestamp, state_json_gz, op_count }

Restore to version:
  Load snapshot from S3 → apply operations after snapshot timestamp
  → Efficient: don't replay all operations from day 1

Named versions (Figma "Version History"):
  User explicitly saves version → tagged snapshot
  Stored permanently (not garbage collected)
  
Operation log (for fine-grained replay):
  All operations stored in Cassandra (append-only):
    { doc_id, op_id (TimeUUID), user_id, operation_json }
  Partitioned by doc_id, clustered by op_id (time-ordered)
```

---

## 8. Protocols Used

```
WebSocket:
  Primary: real-time bidirectional ops + cursor sync
  Why not HTTP: need push from server; can't poll at 60fps

WebRTC (optional, for video/audio):
  If design reviews with voice → peer-to-peer audio (no server relay)
  Less common in design tools

Protocol Buffers (protobuf) over WebSocket:
  Binary encoding of CRDT deltas (not JSON)
  → 5-10× smaller payload than JSON
  → Critical for 10K users on same doc

HTTP/2:
  Initial document load, snapshot fetch
  Multiple concurrent requests (multiplexed)

SSE (Server-Sent Events):
  Alternative to WebSocket for one-directional server push
  Simpler (HTTP-based), no upgrade handshake
  Trade-off: half-duplex (client can't push ops via SSE)
  → WebSocket is better for true bidirectional collaboration
```

---

## Key Talking Points

1. **Canvas vs SVG**: Canvas = O(1) rendering regardless of element count; SVG = O(n) DOM
2. **CRDT over OT**: no central coordinator needed; deterministic conflict resolution
3. **Yjs**: mention it by name — it's what Excalidraw uses; shows you've researched
4. **Sharding by doc_id**: all WebSocket connections for a doc go to the same server node
5. **Viewport filtering**: don't broadcast ops to users not viewing that canvas area
6. **Per-user undo**: not global undo — explain the difference clearly
7. **Snapshots + op log**: efficient history without replaying all ops from scratch
8. **Protocol**: WebSocket + protobuf for binary efficiency; mention why not SSE

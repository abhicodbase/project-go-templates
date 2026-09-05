# 08 — Distributed Systems

## TL;DR
> Distributed systems are fundamentally about trade-offs. Consistent hashing minimizes resharding pain.  
> Consensus (Raft/Paxos) ensures agreement across nodes. Leader election coordinates distributed work.  
> Design for partial failure — network partitions and node crashes are normal.

---

## 1. Fallacies of Distributed Computing

The 8 things developers wrongly assume are true:
1. The network is reliable
2. Latency is zero
3. Bandwidth is infinite
4. The network is secure
5. Topology doesn't change
6. There is one administrator
7. Transport cost is zero
8. The network is homogeneous

> **All of these are false.** Design your system assuming any of these can fail at any time.

---

## 2. Consistent Hashing

Used to distribute data/load across servers in a way that minimizes redistribution when servers are added/removed.

### Problem with Simple Modulo Hashing
```
3 servers: shard = hash(key) % 3
key="alice" → hash % 3 = 1 → Server 1

Add 4th server: shard = hash(key) % 4
key="alice" → hash % 4 = 2 → Server 2  ← MOVED! (almost all keys remap)
```
Adding 1 server remaps ~75% of keys → massive data migration.

### Consistent Hashing Ring
```
                   0
               ┌───────┐
           270─┤       ├─90
               └───────┘
                   180

Place servers on the ring by hash(server_id):
  Server A at 60°
  Server B at 150°
  Server C at 240°

Key assignment: walk clockwise from key's position → first server encountered
  key at 30° → Server A (60° is next clockwise)
  key at 100° → Server B (150° is next clockwise)
  key at 200° → Server C (240° is next clockwise)
```

### Adding/Removing Servers
```
Add Server D at 100°:
  Only keys between 60° (A) and 100° (D) move from A to D
  ~1/N of keys redistribute (not all!)

Remove Server A:
  Only keys assigned to A move to next server (B)
```

### Virtual Nodes (Vnodes)
To prevent uneven distribution:
```
Instead of 1 point per server:
  Server A → 100 virtual nodes spread around ring
  Server B → 100 virtual nodes
  Server C → 100 virtual nodes

Keys distribute evenly across all virtual nodes → even load balancing
When server added/removed: only that server's virtual nodes affected
```

Used by: **Cassandra**, **DynamoDB**, **Memcached**, **Chord DHT**

---

## 3. Consensus Algorithms

How do distributed nodes **agree** on a single value even when some nodes fail or messages are lost?

### The Problem
```
3 nodes all receive different views of reality:
  Node A thinks: "primary is Server 1"
  Node B thinks: "primary is Server 2"
  Node C thinks: "primary is Server 1"

Without consensus → split-brain → data corruption
```

### Raft Consensus (Easier to Understand)

Raft works in terms of **leaders**, **followers**, and **candidates**.

**Leader Election:**
```
All nodes start as Followers.

If no heartbeat from Leader within timeout (150–300ms):
  1. Follower becomes Candidate
  2. Candidate increments term, votes for itself
  3. Sends RequestVote to all peers
  4. If receives majority votes → becomes Leader
  5. Leader sends heartbeats to maintain authority

Split vote → random timeout → restart election
```

**Log Replication (once leader elected):**
```
Client ──write──▶ Leader
  Leader appends to local log (uncommitted)
  Leader sends AppendEntries to all Followers
  When majority (N/2+1) acknowledge:
    Leader commits entry
    Leader notifies Followers to commit
    Leader responds to Client ✓
```

**Key Properties:**
- At most one leader per term
- Leader has all committed entries
- If leader crashes, new election (term increments)
- Raft needs quorum: (N/2 + 1) nodes available

```
5-node cluster: needs 3 nodes to make progress
3-node cluster: needs 2 nodes to make progress
```

### Paxos
Older, harder to understand, but equivalent to Raft.
- Multi-Paxos used in production (Google Chubby, Apache Zookeeper)
- Raft preferred for new systems (cleaner, easier to implement correctly)

### Where Consensus is Used
| System | Uses Consensus For |
|--------|-------------------|
| **etcd** | Kubernetes cluster state |
| **Zookeeper** | Distributed coordination |
| **CockroachDB** | Cross-shard transactions |
| **Consul** | Service discovery, health checks |
| **Kafka** (KRaft mode) | Controller election |

---

## 4. Leader Election

A simplified case of consensus — picking one node as the coordinator.

### Approaches

**Bully Algorithm:**
```
When coordinator fails:
  1. Node P notices coordinator is gone
  2. P sends ELECTION message to all nodes with higher ID
  3. Highest-ID node that responds becomes new leader
  4. Winner sends COORDINATOR message to all
```

**ZooKeeper-Based Election:**
```
All nodes create ephemeral sequential znode: /election/node_
  /election/node_0000001 (Node A — becomes leader, lowest seq)
  /election/node_0000002 (Node B — watches node_0000001)
  /election/node_0000003 (Node C — watches node_0000002)

Node A dies → its ephemeral znode deleted → Node B notified → Node B becomes leader
```

**Raft Election** (as described above — most modern systems use this).

---

## 5. Distributed Locks

Prevent multiple nodes from concurrently modifying the same resource.

### Why Not a Single DB?
```
Service A and B both try to acquire lock at same time:
  Both read: "lock is free"
  Both write: "lock acquired"
  → Both think they have the lock → data corruption!

Need atomic compare-and-swap.
```

### Redis-Based Distributed Lock (Redlock)
```redis
-- Acquire lock (atomic SET with NX + expiry)
SET lock:resource_123 client-uuid-abc NX EX 30

-- NX = only set if key doesn't exist (atomic check-and-set)
-- EX 30 = expire in 30 seconds (auto-release if holder crashes)

-- Release lock (only if WE own it — Lua script for atomicity)
if redis.call("get", KEYS[1]) == ARGV[1] then
  redis.call("del", KEYS[1])
  return 1
else
  return 0
end
```

**Redlock Algorithm** (multiple Redis nodes for fault tolerance):
1. Get current timestamp T1
2. Try to acquire lock on N/2+1 Redis nodes
3. Lock acquired only if majority nodes succeed AND total elapsed time < lock TTL
4. If failed: release from all nodes, retry with backoff

### ZooKeeper-Based Distributed Lock
```
1. Create ephemeral sequential znode: /locks/resource_/lock_
   → /locks/resource_/lock_0000001 (Client A)
   → /locks/resource_/lock_0000002 (Client B)
   → /locks/resource_/lock_0000003 (Client C)

2. Lowest sequence number = lock holder (Client A)
3. Client B watches lock_0000001 (predecessor)
4. When lock_0000001 deleted: Client B gets watch event → acquires lock
```

---

## 6. Quorum

A quorum is the minimum number of nodes that must agree for an operation to be valid.

```
For a cluster of N nodes:
  Write quorum (W) + Read quorum (R) > N  →  reads always see latest write

Example: N=5, W=3, R=3 → W+R=6 > 5 ✓ (strong consistency)
         N=5, W=1, R=5 → W+R=6 > 5 ✓ (slow reads, fast writes)
         N=5, W=3, R=1 → W+R=4 < 5 ✗ (eventual consistency only)
```

### Common Quorum Configurations
| W | R | N | Characteristic |
|---|---|---|---------------|
| N | 1 | N | Fast reads, slow writes |
| 1 | N | N | Fast writes, slow reads |
| N/2+1 | N/2+1 | N | Balanced, strong consistency |

Used by: **Cassandra** (tunable), **DynamoDB** (tunable), **Riak**

---

## 7. Vector Clocks & Conflict Resolution

When two nodes independently update the same data, how do you know which is newer?

### Lamport Timestamps
```
Logical clock: each event increments a counter
On send: attach counter
On receive: max(local, received) + 1

Node A: sends at t=2 → Node B: receives, sets t=max(1,2)+1=3
```
Limitation: only tells you "happened-before" — can't detect concurrent events.

### Vector Clocks
```
Each node maintains a vector of counters, one per node.

Node A: [A=1, B=0, C=0] sends to B
Node B: receives → [A=1, B=1, C=0] (increments own, merges A's)
Node B: sends to C
Node C: receives → [A=1, B=1, C=1]

Concurrent events (can't compare):
  Node A: [A=2, B=0, C=0]
  Node B: [A=1, B=1, C=0]
  Neither is strictly "after" the other → CONFLICT!
```

**Conflict Resolution Strategies:**
- Last-Write-Wins (LWW): use wall clock (risky — clocks drift)
- Let client merge: Amazon Dynamo shopping cart (union of items)
- Application-specific logic (e.g., CRDT data structures)

---

## 8. Two-Phase Commit (2PC)

Distributed protocol to ensure all-or-nothing across multiple databases/services.

```
Phase 1 (Prepare):
  Coordinator ──PREPARE──▶ Participant A
  Coordinator ──PREPARE──▶ Participant B
  Coordinator ◀── VOTE_YES── Participant A (ready to commit)
  Coordinator ◀── VOTE_YES── Participant B

Phase 2 (Commit):
  Coordinator ──COMMIT──▶ Participant A (if all VOTE_YES)
  Coordinator ──COMMIT──▶ Participant B
         OR
  Coordinator ──ROLLBACK──▶ All (if any VOTE_NO)
```

**Problems with 2PC:**
- **Blocking**: if coordinator crashes after PREPARE, participants are stuck (holding locks)
- **SPOF**: coordinator is single point of failure
- **Performance**: 2 round trips per transaction

**Alternative**: Saga Pattern (see Microservices doc) — compensating transactions instead of distributed locks.

---

## 9. Gossip Protocol

Nodes periodically share state with random peers (like rumors spreading through a group).

```
Node A knows: "Server X is down"
  → tells Node B and Node D
  → B tells E and C
  → D tells F and A
  → Eventually all nodes know "Server X is down"
```

**Properties:**
- Eventually consistent propagation
- Resilient (no single point of failure)
- Scales well (O(log N) rounds to reach all nodes)

Used by: **Cassandra** (cluster membership), **DynamoDB**, **Redis Cluster**

---

## 10. CAP in Practice

| System | CP or AP | Why |
|--------|---------|-----|
| ZooKeeper | CP | Returns error during partition |
| etcd | CP | Raft consensus — won't serve stale data |
| Cassandra | AP | Always accepts writes; eventual consistency |
| DynamoDB | AP (tunable) | Eventually consistent by default |
| CockroachDB | CP | Distributed ACID |
| MongoDB | CP (default) | Primary must be available for writes |

---

## Key Takeaways

1. **Consistent hashing** + virtual nodes = even distribution, minimal resharding
2. **Raft** for leader election and consensus — understand quorum requirement (N/2+1)
3. **Distributed locks** via Redis SET NX EX — always set TTL to auto-expire on crash
4. **Quorum reads/writes**: tune W+R > N for strong consistency; W+R ≤ N for performance
5. **2PC** has blocking problem — prefer **Saga** for distributed transactions
6. **Gossip** for cluster membership and health propagation at scale
7. **Vector clocks** for detecting concurrent updates; define clear conflict resolution strategy

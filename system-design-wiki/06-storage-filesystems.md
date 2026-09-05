# 06 — Storage & File Systems

## TL;DR
> Object storage (S3) for files at scale. Block storage for databases/VMs.  
> HDFS for distributed big-data processing. CDN to serve large files fast.  
> Always use pre-signed URLs — never serve files through your app server.

---

## 1. Types of Storage

| Type | Abstraction | Access | Use Cases |
|------|-------------|--------|-----------|
| **Block Storage** | Raw disk blocks | Block-level I/O | Databases, VM disks, OS |
| **File Storage** | Files + directories | File path (NFS, SMB) | Shared file systems, home dirs |
| **Object Storage** | Objects with metadata | HTTP REST API | Images, videos, backups, big data |

---

## 2. Object Storage (S3-compatible)

Store data as **objects** in **buckets**. Each object has:
- A **key** (unique identifier / "path")
- The **data** (binary blob, any size)
- **Metadata** (content-type, custom tags, etc.)

### Core Operations
```
PUT   /bucket/photos/user123/avatar.jpg   → upload object
GET   /bucket/photos/user123/avatar.jpg   → download object
DELETE /bucket/photos/user123/avatar.jpg  → delete object
LIST  /bucket/photos/user123/             → list objects by prefix
```

### Why Object Storage for User Uploads?
```
❌ Naive approach:
Client ──upload──▶ App Server ──▶ Local disk
  Problem: doesn't scale (disk fills up, files not shared across app instances)

✅ Correct approach (Pre-signed URLs):
1. Client ──request upload URL──▶ App Server
2. App Server ──▶ S3: generate pre-signed PUT URL (valid 15min)
3. App Server ──▶ Client: pre-signed URL
4. Client ──PUT directly──▶ S3 (bypasses app server!)
5. S3 triggers event ──▶ App Server: "upload complete, path=..."
```

### Pre-signed URL Pattern
```
App Server generates:
  URL: https://bucket.s3.amazonaws.com/uploads/user123/file.jpg
  Signed with: expiry=15min, method=PUT, content-type=image/jpeg

Benefits:
  - App server not in upload path (saves bandwidth + CPU)
  - File goes directly to S3
  - URL expires so can't be misused
```

### Object Storage Tiers (S3 Storage Classes)
| Tier | Access | Cost | Use For |
|------|--------|------|---------|
| Standard | Frequent | $$$ | Actively accessed data |
| Standard-IA | Infrequent | $$ | Backups, disaster recovery |
| Glacier Instant | Rare (ms retrieval) | $ | Archives, compliance |
| Glacier Flexible | Rare (min-hours retrieval) | $¢ | Long-term archival |
| Deep Archive | Very rare (12-hr retrieval) | ¢¢ | Regulatory, 7+ yr retention |

### Lifecycle Policies (Auto-tiering)
```
0–30 days:   Standard (frequent access)
30–90 days:  Standard-IA (infrequent access)
90–365 days: Glacier Instant Retrieval
365+ days:   Glacier Deep Archive
```

---

## 3. Block Storage

Low-level storage that presents raw disk blocks to an OS or application.

```
VM / App ──▶ Block Device (/dev/sda) ──▶ Volume ──▶ Physical Disk
```

- **EBS** (AWS Elastic Block Store) — attach to EC2 instances
- **Azure Disk**, **GCP Persistent Disk**
- Databases use block storage (PostgreSQL writes to block device directly)

### Block Storage Types
| Type | IOPS | Latency | Use For |
|------|------|---------|---------|
| HDD (magnetic) | ~100–500 IOPS | ~10ms | Cold data, backups |
| SSD (general) | ~3,000–16,000 IOPS | ~0.1ms | General databases |
| NVMe SSD | ~1M+ IOPS | <0.01ms | High-performance DBs, analytics |

### RAID (Redundant Array of Independent Disks)
| Level | How | Read | Write | Fault Tolerance |
|-------|-----|------|-------|----------------|
| RAID 0 | Stripe across disks | Fast | Fast | None (any disk fails = data loss) |
| RAID 1 | Mirror to 2 disks | Fast | Slower | Survive 1 disk failure |
| RAID 5 | Stripe + parity | Fast | Medium | Survive 1 disk failure |
| RAID 10 | Mirror + stripe | Very fast | Fast | Survive multiple disk failures |

---

## 4. File Storage (Network File Systems)

Shared file systems accessible over a network.

- **NFS** (Network File System) — Linux-to-Linux
- **SMB/CIFS** — Windows file sharing (Samba for Linux)
- **EFS** (AWS Elastic File System) — NFS as managed service
- **Azure Files** — SMB-compatible managed file share

### Use Cases
- Shared configuration files across app servers
- ML training data shared across GPU nodes
- Legacy apps that expect a filesystem interface

---

## 5. HDFS (Hadoop Distributed File System)

A distributed file system designed for very large files and batch processing.

```
                    ┌─────────────┐
                    │  NameNode   │ ← metadata (file → block locations)
                    └─────────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
    ┌──────────┐    ┌──────────┐    ┌──────────┐
    │ DataNode │    │ DataNode │    │ DataNode │
    │ Block A  │    │ Block B  │    │ Block C  │
    │ Block D  │    │ Block A* │    │ Block B* │ ← replicas (factor 3)
    └──────────┘    └──────────┘    └──────────┘
```

**Key Properties:**
- Files split into large blocks (128 MB or 256 MB default)
- Each block replicated 3× across DataNodes
- NameNode holds metadata; DataNodes hold actual data
- Write-once, read-many (optimized for streaming reads)
- Not suited for low-latency random reads

**HDFS vs Object Storage:**
| Feature | HDFS | S3 / Object Storage |
|---------|------|---------------------|
| Latency | Low (local network) | Higher (HTTP) |
| Throughput | Very high (streaming) | High |
| Cost | High (own hardware) | Low (pay per GB) |
| Integration | Hadoop ecosystem | Universal |
| Modifications | Append-only | Replace whole object |

---

## 6. Content Delivery for Large Files

### Video Delivery Pattern
```
Upload:
  User ──▶ App Server (metadata) + Pre-signed URL
  User ──PUT video──▶ S3 (origin bucket)
  S3 Event ──▶ Transcoding Service (FFmpeg, AWS Elemental)
             → generate 360p, 720p, 1080p, 4K variants
             → store in delivery bucket

Playback:
  User ──▶ App Server: "give me video 123"
  App ──▶ DB: lookup video URL
  App ──▶ Client: CloudFront CDN URL
  Client ──▶ CloudFront Edge (near user) → streams from edge
  Edge ──▶ S3 origin (on first request)
```

### Adaptive Bitrate Streaming (HLS/DASH)
- Video split into 2–10 second segments
- Player downloads segment list (manifest)
- Player switches quality based on network speed
```
manifest.m3u8
  → 360p/segment_001.ts
  → 360p/segment_002.ts
  → 720p/segment_001.ts  (switch to higher quality when bandwidth allows)
  → 1080p/segment_001.ts
```

---

## 7. Data Replication & Durability

### How S3 Achieves 11 Nines Durability (99.999999999%)
- Data split across multiple physical facilities (AZs)
- Each object stored with Reed-Solomon erasure coding
- Periodic integrity checks (background scrubbing)
- Data automatically repaired on detected corruption

### Replication Factor
```
Replication factor = 3 (typical for HDFS, Cassandra)
  Block A → Server 1, Server 2, Server 3

Durability ≈ 1 - (failure_rate)^replication_factor
```

### Cross-Region Replication
```
Primary Region (us-east-1) ──async replication──▶ DR Region (eu-west-1)
  S3 bucket                                         S3 bucket replica
```
- Protects against entire region failure
- RPO (Recovery Point Objective): how much data can be lost → ~minutes with async replication
- RTO (Recovery Time Objective): how fast can you recover → depends on DNS failover speed

---

## 8. Storage Performance Metrics

| Metric | Description | Typical Values |
|--------|-------------|----------------|
| **IOPS** | I/O Operations Per Second | HDD: 100, SSD: 10K–1M |
| **Throughput** | Data transferred per second | HDD: 100 MB/s, SSD: 500 MB/s–7 GB/s |
| **Latency** | Time per single I/O | HDD: 10ms, SSD: 0.1ms, NVMe: 0.02ms |
| **Durability** | % of time data is intact | S3: 99.999999999% |
| **Availability** | % of time storage is accessible | S3: 99.99% |

---

## Key Takeaways

1. **Object storage** (S3) for files > a few MB; never serve files through app server
2. **Pre-signed URLs** — let clients upload/download directly from S3
3. **Block storage** for databases; object storage for user content and backups
4. Use **S3 lifecycle policies** to auto-tier old data to cheaper storage classes
5. **CDN** in front of S3 for all read traffic — dramatically reduces latency + cost
6. **HDFS** for Hadoop batch jobs; **object storage** is the modern replacement
7. Design for **3× replication** minimum; cross-region replication for disaster recovery

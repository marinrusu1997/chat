# Messaging Requirements for ScyllaDB

This document extracts only the **messaging-specific functional requirements** from modern chat applications (Telegram, WhatsApp, Slack, Discord, etc.) and translates them into technical requirements for ScyllaDB.

---

## 1. Messaging-Specific Functional Requirements

### Message Lifecycle
- Send and receive messages (text, media, files).
- Delivery receipts (sent, delivered, read indicators).
- Reactions (emoji, likes, etc.).
- Editing/deleting messages.
- Quoting/replying to messages.
- Forwarding messages.

### Chat Types
- One-to-one chats.
- Group chats.
- Channels/broadcasts (one-to-many).

### Ordering & History
- Guaranteed ordering of messages within a chat.
- Retrieval of last N messages.
- Jump-to-message (fetching around a specific message).
- Pagination when scrolling history.

### Ephemeral Behavior
- Auto-delete (disappearing messages, self-destruct timers).

### Read State
- Track per-user read/unread state.
- Sync read state across devices.

---

## 2. Technical Impact on ScyllaDB

### 2.1 Message Storage
- **Table:** `messages_by_chat (chat_id, timestamp, message_id, sender_id, content, metadata)`.
- **Partition key:** `chat_id` (messages grouped per chat).
- **Clustering key:** `(timestamp, message_id)` (ensures order + uniqueness).
- **Writes:** Append-only inserts for new messages.
- **Edits/Deletes:** Represented as new events or flags.
- **Reactions:** Stored as separate rows or in a companion table.

---

### 2.2 Delivery Receipts
- Show per-message delivery/read status.
- **Option A:** Per-user-per-message status:
    - Table: `message_status (chat_id, message_id, user_id, status, updated_at)`.
    - Very write-heavy.
- **Option B:** Per-user read pointer:
    - Table: `chat_read_state (chat_id, user_id, last_read_message_id, updated_at)`.
    - Scalable: one row per user per chat.
- ⚖️ Requires design decision.

---

### 2.3 Message Ordering & History
- **Ordering:** Clustering on `(timestamp, message_id)` guarantees per-chat order.
- **History queries:**
    - `SELECT * FROM messages_by_chat WHERE chat_id = ? ORDER BY timestamp DESC LIMIT 50;`
    - `SELECT * FROM messages_by_chat WHERE chat_id = ? AND timestamp >= ? LIMIT 50;`
- **Hot partitions:** Very active chats may overload a single partition.
    - Strategy: add bucketing (e.g., `(chat_id, day)` as partition key).

---

### 2.4 Chat Types
- Same `messages_by_chat` table serves 1:1, group, and channel chats.
- Differences (participants, permissions) stored in PostgreSQL metadata.
- **For channels (broadcasts):**
    - **Fan-out-on-read:** Store once per channel; users read from the same partition.
    - **Fan-out-on-write:** Duplicate messages per user inbox; higher storage cost, faster read.
- ⚖️ Requires design decision.

---

### 2.5 Replies, Forwards, Quoting
- Store references to other messages:
    - Fields: `reply_to_message_id`, `forwarded_from_chat_id`.
- Requires **random access lookup** by message_id.
- **Secondary table:** `message_by_id (chat_id, message_id)` → message content.

---

### 2.6 Ephemeral Messages
- Messages with self-destruct timers.
- **ScyllaDB Feature:** Native TTL on rows.
- Messages expire automatically without manual cleanup.

---

### 2.7 Read State
- Show unread counters and sync across devices.
- Efficient strategy: store **per-user last read message_id**.
- Table: `chat_read_state (chat_id, user_id, last_read_message_id, updated_at)`.
- Unread count = difference between last message_id in chat and last read message_id for user.

---

## 3. Key Design Decisions Ahead
1. **Delivery receipts strategy:**
    - Per-message-per-user vs. per-user read pointer.
2. **Hot partition handling:**
    - Simple `chat_id` partition vs. bucketing `(chat_id, day)`.
3. **Fan-out strategy for channels:**
    - Fan-out-on-read vs. fan-out-on-write.
4. **Storage of reactions and edits:**
    - Inline with message vs. separate event tables.

---
## 4. Decision Matrix for Messaging Design Choices

This section compares alternative strategies for key messaging design decisions in ScyllaDB.

| **Decision Area**               | **Option**                             | **Pros**                                                                                                     | **Cons**                                                                              | **When to Use**                                                             |
|---------------------------------|----------------------------------------|--------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------|-----------------------------------------------------------------------------|
| **Delivery Receipts**           | **Per-message-per-user**               | - Fine-grained visibility (per user, per message)<br>- Needed for double-ticks style (WhatsApp)              | - Extremely write-heavy in large groups<br>- High storage overhead                    | If exact per-user delivery state must be tracked (e.g., WhatsApp semantics) |
|                                 | **Per-user read pointer**              | - Much lighter on storage<br>- Simple model (last read message per user)<br>- Easy unread counts             | - Cannot show per-message delivery state<br>- Less precise                            | If "last read" semantics are enough (e.g., Telegram, Slack style)           |
| **Hot Partition Handling**      | **Partition by chat_id**               | - Simple schema<br>- All messages in one partition<br>- Natural per-chat ordering                            | - Hotspot risk for very active chats/channels<br>- Potential imbalance across cluster | Small/medium group chats; low write volume per chat                         |
|                                 | **Partition by (chat_id, bucket/day)** | - Spreads writes across partitions<br>- Avoids hot partition issue<br>- Better cluster balance               | - Slightly more complex queries<br>- Must manage bucketing logic                      | Large/high-throughput chats or channels                                     |
| **Fan-out Strategy (Channels)** | **Fan-out-on-read**                    | - Messages stored once<br>- Storage efficient<br>- Scales well for medium/large chats                        | - Reads heavier for large subscriber bases<br>- Clients may need more logic           | When channel size is moderate to large; saves storage                       |
|                                 | **Fan-out-on-write (inbox per user)**  | - Reads are very fast (each user has their own inbox)<br>- Unread state trivial                              | - Massive storage overhead (duplicate messages)<br>- Heavy write amplification        | When ultra-low read latency per user is critical and storage is cheap       |
| **Reactions & Edits**           | **Inline in message row**              | - Simple schema<br>- No joins<br>- Fewer tables                                                              | - Row size grows with edits/reactions<br>- Frequent overwrites                        | Small groups or apps with light reaction/edit traffic                       |
|                                 | **Separate event tables**              | - Append-only writes (no overwrites)<br>- Scales better with frequent edits/reactions<br>- Clear audit trail | - Requires joins/extra reads for full message state                                   | Apps with heavy use of reactions/edits, or auditability requirements        |

## 5. Chosen Strategies

Based on the decision matrix, the following strategies have been selected:

- **Delivery Receipts:**  
  Use **per-user read pointer** (`chat_read_state` table).  
  → Efficient and scalable, tracks last read message per user per chat.

- **Hot Partition Handling:**  
  Use **partition by (chat_id, day)**.  
  → Spreads writes across buckets, avoids hot partitions for high-traffic chats.

- **Fan-out Strategy (Channels):**  
  Use **fan-out-on-read**.  
  → Messages stored once per channel; storage efficient and scalable.

- **Reactions & Edits:**  
  Store them **inline in the message row**.  
  → Simple schema, fewer tables, acceptable since reaction/edit volume is not extreme.

# Messaging Design Decision Matrix (Extended)

This matrix captures additional architectural choices for implementing messaging in ScyllaDB.

---

## 1. Message Ordering & Pagination

| Decision Area              | Options | Pros | Cons | When to Use |
|-----------------------------|---------|------|------|-------------|
| Ordering & Pagination       | **Timestamp-based** | Natural for Scylla (clustering key), simple queries, easy pagination | Clock skew can misorder messages slightly, not a strict total order | If "eventual ordering" is acceptable |
|                             | **Message ID (TimeUUID/ULID)** | Monotonic IDs ensure consistent ordering, no reliance on system clock | Slightly more complex ID generation | If strict ordering across replicas/devices is required |

---

## 2. Message Retention & Deletion

| Decision Area              | Options | Pros | Cons | When to Use |
|-----------------------------|---------|------|------|-------------|
| Retention & Deletion        | **Hard delete (remove row)** | Saves storage, clean queries | Expensive in Scylla (tombstones), no audit trail | If compliance requires data to be erased |
|                             | **Soft delete (flag `is_deleted`)** | Preserves history, avoids tombstone storms | Extra query filter, larger storage footprint | If audit, moderation, or recovery is needed |
|                             | **TTL per message** | Automatic expiration, no manual cleanup | Dangerous if misconfigured, not flexible | For ephemeral messaging (e.g., auto-expire after X days) |

---

## 3. User Inbox / Chat List

| Decision Area              | Options | Pros | Cons | When to Use |
|-----------------------------|---------|------|------|-------------|
| User Inbox / Chat List      | **Derive from messages on read** | No extra storage | Very expensive (scans), poor UX | Never in production-scale apps |
|                             | **Dedicated "inbox" table** | Fast retrieval of chat list, supports unread counts & last message | Requires denormalization on write | Always for scalable apps (WhatsApp/Telegram style) |

---

## 4. Search / Filtering

| Decision Area              | Options | Pros | Cons | When to Use |
|-----------------------------|---------|------|------|-------------|
| Search / Filtering          | **Scylla only (limited filters)** | Simpler, fewer dependencies | Can’t do full-text search | If search is not a requirement |
|                             | **External index (Elasticsearch/OpenSearch)** | Full-text search, flexible filters | Adds operational complexity, eventual consistency | If in-chat search or global search is a must |
|                             | **Hybrid (store message metadata in Scylla, text index elsewhere)** | Best of both worlds | More moving parts | If you need both scalable messaging + powerful search |

---

## 5. Attachments / Media

| Decision Area              | Options | Pros | Cons | When to Use |
|-----------------------------|---------|------|------|-------------|
| Attachments / Media         | **Store blob in Scylla** | Simpler, single system | Terrible for large files, stresses cluster | Rarely, only for tiny media (like emoji packs) |
|                             | **Store only metadata in Scylla + blob in S3/CDN** | Scales well, standard practice | Requires integration with external storage | Always, for images, files, voice, video |
|                             | **Separate attachment table (metadata only)** | Avoids bloating message rows | More queries | If attachments are frequent & large |

---

## 6. Multi-device Synchronization

| Decision Area              | Options | Pros | Cons | When to Use |
|-----------------------------|---------|------|------|-------------|
| Multi-device Sync           | **Single read pointer per user** | Simple model, fewer rows | Devices may show "read" state inconsistently | If most users use only 1 device |
|                             | **Per-device read pointer** | Correct multi-device sync, better UX | More rows, more writes | If multi-device usage is common (Telegram, Messenger) |

---

## 7. Message IDs

| Decision Area              | Options | Pros | Cons | When to Use |
|-----------------------------|---------|------|------|-------------|
| Message IDs                 | **TimeUUID** | Native to Scylla, ordered clustering | Harder for debugging | If natural Scylla support is desired |
|                             | **ULID** | Lexicographically sortable, human-readable | Needs custom generator | If you want developer-friendlier IDs |
|                             | **Snowflake-style** | High control, sharding hints possible | More infra complexity | If you want global uniqueness + shard hints |

---

## 8. Consistency vs Availability

| Decision Area              | Options | Pros | Cons | When to Use |
|-----------------------------|---------|------|------|-------------|
| Consistency Level           | **QUORUM** | Balance between safety and performance | May still allow stale reads | Default for messaging apps |
|                             | **LOCAL_QUORUM** | Low latency within a DC | Not cross-DC consistent | Multi-DC deployments |
|                             | **ALL** | Strict consistency | High latency, low availability | Only for compliance-heavy use cases |

---

## 9. Analytics / Metrics

| Decision Area              | Options | Pros | Cons | When to Use |
|-----------------------------|---------|------|------|-------------|
| Analytics / Metrics         | **Inline counters in Scylla** | Simple, local | Scylla counters have write contention issues | Avoid for high-volume messaging |
|                             | **Events stream to Kafka/ClickHouse** | Scales, allows real-time + historical analytics | Adds infra | If analytics is a core requirement |
|                             | **Batch ETL from Scylla to warehouse** | Simpler ops | Delayed insights | If you only need periodic reporting |

---

## 6. Final Architecture Choices (Your Decisions)

These are the choices made for message-focused behavior and ScyllaDB implementation.

- **Message Ordering & Pagination**
  - **Choice:** Message ID based ordering using **TimeUUID**.
  - **Implication:** TimeUUID allows ordered clustering and avoids clock-skew issues of pure timestamp-based ordering.

- **Message Retention & Deletion**
  - **Choice:** **Hard delete** (remove rows).
  - **Implication:** Rows are deleted from the data model; requires tombstone-aware operations and compaction tuning in ScyllaDB.

- **User Inbox / Chat List**
  - **Choice:** **Dedicated inbox table** (denormalized per-user chat list with last message, unread counts).
  - **Implication:** Fast reads for chat list at the cost of denormalization and extra writes on message send.

- **Search / Filtering**
  - **Choice:** External search index: **Elasticsearch / OpenSearch** (sync from ScyllaDB).
  - **Implication:** Full-text and rich search capabilities via an external index; requires CDC/streaming sync (e.g., Kafka).

- **Attachments / Media**
  - **Choice:** Store **metadata in ScyllaDB**; blobs stored in **S3 / MinIO / CDN**.
  - **Implication:** Message rows remain small; retrieval requires fetching media from external storage via URL/presigned URL.

- **Multi-Device Synchronization**
  - **Choice:** **Single read pointer per user** (not per-device).
  - **Implication:** Simpler reads/writes; slight UX caveat if user reads on multiple devices simultaneously (last-read is shared across devices).

- **Message IDs**
  - **Choice:** **TimeUUID** (used as message_id).
  - **Implication:** Ordered, Scylla-friendly IDs; works well as part of clustering key.

- **Consistency vs Availability**
  - **Choice:** **LOCAL_QUORUM** (reads/writes prefer local datacenter quorum).
  - **Implication:** Lower cross-datacenter latency while preserving reasonable safety; choose suitable RF and locality.

- **Analytics / Metrics**
  - **Choice:** **Inline counters in ScyllaDB**.
  - **Implication:** Simple implementation but be mindful of Scylla counters contention; consider mitigating patterns if volume grows.

---

## 7. Operational Notes & Recommended Mitigations

### Hard Deletes (tombstone considerations)
- Hard deletes create tombstones. If deletes are frequent, this can lead to read amplification and compaction pressure.
- **Mitigations:**
  - Use **time-bucketed partitions** (you already use `(chat_id, day)` bucketing) — small partition sizes reduce tombstone impact.
  - Schedule bulk delete / compaction during off-peak windows where possible.
  - Tune `gc_grace_seconds` and compaction strategy appropriately for your retention policy and consistency needs.
  - Consider soft-delete + background compaction jobs for very large/deleted volumes if tombstones become problematic.

### Inline Counters (analytics)
- Scylla counters are simple but suffer write contention on hot partitions if you increment the same counter very frequently (e.g., global counters or per-chat counters on very active chats).
- **Mitigations:**
  - Use **time-bucketed counters** (per minute/hour/day) and aggregate downstream, rather than a single global counter.
  - Consider staggering writes (sharded counter pattern) or moving to event streaming (Kafka → ClickHouse) when analytics scale grows.
  - If analytics becomes critical, you can hybridize: inline counters for small-scale metrics, event stream for heavy analytics.

### Elasticsearch / OpenSearch Sync
- Plan for a **CDC pipeline** (Scylla CDC → Kafka → Elasticsearch) or a change-stream processor; ensure near-real-time eventual consistency and be prepared for reindexing strategies on schema changes.

---

## 8. Next Step
I will now draft the **first-pass ScyllaDB schema (CQL)** that implements these choices:
- Tables: `messages_by_chat` (bucketed by day), `chat_read_state`, `inbox_by_user`, `message_by_id` (for direct lookup), `attachments_metadata`, maybe `message_revisions` if needed for edit history (you chose inline edits, so edits will update the message row).
- Use **TimeUUID** for `message_id` clustering and `LOCAL_QUORUM` as the recommended consistency for operations.
- Include example CQL definitions and example common queries (insert, fetch last N, pagination, update read pointer, delete message).

Proceeding to draft the schema now unless you want any quick adjustments to the decisions above.

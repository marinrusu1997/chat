# Technical Requirements for ScyllaDB

This document translates the functional features of a messaging app into technical requirements for ScyllaDB.

## 1. High Write Throughput
- Requirement: Users may send millions of messages per second globally.
- ScyllaDB Role:
    - Store raw chat messages with metadata.
    - Handle high concurrent writes without bottlenecks.

## 2. Low Latency Reads
- Requirement: Instant message retrieval and history scrolling.
- ScyllaDB Role:
    - Query last N messages in a chat (sorted by timestamp).
    - Query messages around a specific message for “jump to message”.
    - Fast pagination with clustering keys.

## 3. Message Ordering & Consistency
- Requirement: Messages must appear in order per chat.
- ScyllaDB Role:
    - Use composite partition keys (chat_id) and clustering keys (timestamp/message_id).
    - Guarantee sequential consistency at the partition level.

## 4. Scalability for Large Groups/Channels
- Requirement: Millions of members in a single channel.
- ScyllaDB Role:
    - Partition and shard messages to distribute load.
    - Support fan-out strategies (write once per channel, replicate pointers for subscribers).

## 5. Read Scaling
- Requirement: A single message may be read by millions.
- ScyllaDB Role:
    - Store references once, with fan-out-on-read model.
    - Use secondary indexes or materialized views for user inboxes.

## 6. Durability & Reliability
- Requirement: No message loss.
- ScyllaDB Role:
    - Replication factor across nodes/datacenters.
    - Tunable consistency levels (e.g., QUORUM writes).

## 7. Time-to-Live (TTL) for Ephemeral Messages
- Requirement: Auto-delete after a set time.
- ScyllaDB Role:
    - Native TTL support for message expiration.

## 8. Search & Filtering
- Requirement: Search by keyword, sender, date.
- ScyllaDB Role:
    - Not suitable for full-text search.
    - Store structured metadata for quick filtering.
    - Integrate with Elasticsearch/OpenSearch for full-text search.

## 9. Presence & Typing Indicators
- Requirement: Real-time updates, very transient.
- ScyllaDB Role:
    - Not optimal (too ephemeral).
    - Use in-memory stores (Redis, Hazelcast) instead.

## 10. Audit, Moderation, Bots
- Requirement: Bots need fast access to message streams.
- ScyllaDB Role:
    - Use Change Data Capture (CDC) to stream messages into Kafka for processing.

---

## Core Data Stored in ScyllaDB
- **Messages**: per chat, ordered by time.
- **Message metadata**: delivery status, reactions, edits, deletions.
- **Per-user inbox pointers**: read/unread state, last synced message.

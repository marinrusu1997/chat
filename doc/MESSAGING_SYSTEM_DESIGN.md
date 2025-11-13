# Messaging System Design

This document describes the architecture and design of the **distributed messaging and notification system** built on **Kafka** and **ScyllaDB**, supporting efficient routing, delivery receipts, horizontal scalability, and secondary push notification channels.

---

## Table of Contents
1. [Overview](#overview)  
2. [Message Routing Layer](#message-routing-layer)  
3. [Partitioning & Scaling Strategy](#partitioning--scaling-strategy)  
4. [Message Synchronization (Offline & Reconnect Flow)](#message-synchronization-offline--reconnect-flow)  
5. [Message Receipts](#message-receipts)  
6. [Second Channel Delivery (Push Notifications)](#second-channel-delivery-push-notifications)  
7. [Presence Service Caveats](#presence-service-caveats)  
8. [Reliability & Cleanup](#reliability--cleanup)  

---

## Overview

This design enables:
- **Scalable** and **fault-tolerant** message routing via Kafka.
- **Low-latency** message delivery for connected users.
- **Eventual delivery** to offline users via APN/FCM push notifications.
- **Real-time synchronization** of message states (`RECEIVED`, `DELIVERED`, `READ`).
- **Idempotent and self-healing** operations across distributed replicas.

---

## Message Routing Layer

### Kafka as Backbone
All inter-replica message routing uses **Kafka** for durability, ordering, and reliability.

### Topics
To avoid topic explosion, we define a limited set of high-throughput topics:

| Topic                | Purpose                                              |
|----------------------|------------------------------------------------------|
| `user-inbox`         | Direct user-to-user messages                         |
| `group-inbox`        | Group messages                                       |
| `delivery-receipts`  | Internal system topic for ACK and receipt updates    |
| `user-notifications` | Notifications to senders about delivery/read updates |

---

## Partitioning & Scaling Strategy

### Sticky Partition Mapping
- Each `user_id` or `group_id` is mapped to a **sticky partition** via **consistent hashing**.
- Ensures all messages for a given recipient are ordered and routed to the same partition.

### Consumer Subscription
- Each **app replica** dynamically subscribes only to partitions corresponding to connected users.
- Reduces unnecessary load and cross-replica traffic.

### Hot Partition Mitigation
- Implement **consistent hashing with virtual nodes (vnodes)**.  
  Example: 1000 partitions with 10–50 virtual nodes for even load distribution.

---

## Horizontal Scaling

### Adding or Removing Replicas
- Adding replicas → create new partitions and redistribute users via consistent hashing.
- Removing replicas → reassign affected partitions.

### Switchover Procedure
1. When topology changes:
   - Replica records a **highWaterMark offset** for current partitions.
   - Continues consuming until offsets reach the highWaterMark.
2. In parallel:
   - Starts consuming from new assigned partitions.
3. Producers maintain a **1-minute grace period**, during which messages are sent to both old and new partitions.

✅ This ensures **zero data loss** and **no message duplication**.

---

## Message Synchronization (Offline & Reconnect Flow)

### When User Disconnects
- Replica continues consuming the user’s partition if other users share it.
- Messages for disconnected users trigger **secondary channel delivery** (see [Push Notifications](#second-channel-delivery-push-notifications)).

### When User Reconnects
Client performs a **sync-up with ScyllaDB**:
1. Fetch latest N messages (e.g., 50).
2. For each message:
   - If already cached → send `"READ"` ACK if newly read.
   - If missing → send `"DELIVERED"` or `"READ"` depending on visibility.
   - Mixed cases → send batch ACKs.

Replica updates delivery states and reconciles them in ScyllaDB.

---

## Message Receipts

### Delivery Pipeline
Receipts are produced to the `delivery-receipts` topic with statuses:

| Status      | Trigger                                                     |
|-------------|-------------------------------------------------------------|
| `RECEIVED`  | Replica stored message in ScyllaDB and produced to Kafka    |
| `DELIVERED` | Message successfully sent via WebSocket and ACKed by client |
| `READ`      | User explicitly read message (via WebSocket or API)         |

### Offline Behavior
- Offline users → only `RECEIVED` status recorded initially.
- Upon reconnection, `"DELIVERED"` and `"READ"` statuses are reconciled.

### ScyllaDB Tables

#### `message-receipts`
| Column                             | Description                           |
|------------------------------------|---------------------------------------|
| `chat_id`, `message_id`, `user_id` | Primary keys                          |
| `status`                           | `"RECEIVED"`, `"DELIVERED"`, `"READ"` |
| `channel`                          | `"ws"`, `"apn"`, etc.                 |
| `updated_at`                       | Timestamp                             |
| TTL                                | 7 days                                |

#### `chat-read-pointers`
| Column                 | Description               |
|------------------------|---------------------------|
| `chat_id`, `user_id`   | Primary keys              |
| `last_read_message_id` | Latest fully read message |
| `updated_at`           | Timestamp                 |

### Reconciliation Logic
- Update status only when new state > current state (RECEIVED < DELIVERED < READ).
- Updating to `"READ"` also updates `chat-read-pointers`.
- When a message receipt changes, a notification is published to Kafka topic `user-notifications`.

### Sender Notification
- Sender’s replica consumes `user-notifications` and pushes updates via WebSocket.
- Offline senders receive updates on next sync.

---

## Second Channel Delivery (Push Notifications)

When WebSocket delivery fails, a **secondary delivery channel** (APN/FCM) ensures message delivery.

### `push-notifications` Table (ScyllaDB)

| Column       | Type        | Description                                     |
|--------------|-------------|-------------------------------------------------|
| `id`         | `text`      | `{type}#{user_id}#{notification_id/message_id}` |
| `deliver_at` | `timestamp` | Scheduled delivery time; `NULL` = delivered     |
| `state`      | `text`      | `"PENDING"` / `"DELIVERED"`                     |
| `payload`    | `json`      | Notification content                            |
| `created_at` | `timestamp` | Record creation time                            |
| TTL          | ~7 days     |

---

### Replica Write Rules

#### On WebSocket Failure
```sql
INSERT INTO push_notifications (...) VALUES (...) IF NOT EXISTS;
```
- `deliver_at = now() + 15s` (configurable)
- If Presence Service reports user as online → delay = `now() + 30s`

#### On WebSocket Success
```sql
UPDATE push_notifications 
SET deliver_at = NULL, state = 'DELIVERED'
WHERE id = ... 
IF state = 'PENDING';
```

#### Conflict Handling

| Existing            | New                        | Action |
|---------------------|----------------------------|--------|
| `PENDING` + failed  | Do nothing                 |
| `PENDING` + success | Overwrite → mark delivered |
| `DELIVERED` + any   | Do nothing                 |

---

### Background Worker

A worker service periodically scans `push-notifications` and delivers pending notifications.

**Loop (every ~5s):**
1. Query pending rows:
   ```sql
   SELECT * FROM push_notifications
   WHERE deliver_at <= now()
     AND state = 'PENDING';
   ```
2. Group notifications by `user_id` / chat to avoid notification spam.
3. Retrieve APN/FCM tokens.
4. Send notifications.
5. On successful delivery or invalid token:
   ```sql
   UPDATE push_notifications
   SET deliver_at = NULL, state = 'DELIVERED'
   WHERE id = ... IF state = 'PENDING';
   ```
6. Update `message-receipts` to `status='DELIVERED'`, `channel='apn'`.

All rows expire automatically after TTL.

---

## Presence Service Caveats

- Presence data may **lag** (e.g., due to replica crash or network delay).
- Never fully rely on Presence Service to suppress push notifications.
- Safer approach:
  - Always enqueue a push-notification record after WebSocket failure.
  - If user is "online" → delay notification slightly (e.g., 30s).
  - If still undelivered → send push anyway.

✅ Guarantees **eventual delivery** despite transient inconsistencies.

---

## Reliability & Cleanup

- **TTL expiration (~7 days)** for automatic cleanup across all tables:
  - `message-receipts`
  - `chat-read-pointers`
  - `push-notifications`
- **Grace period (1 min)** for Kafka producers during scaling ensures message continuity.
- **Idempotent updates (CAS in ScyllaDB)** prevent duplicates.
- **Tombstone markers** in `push-notifications` prevent redundant push notifications.

---

## Summary Checklist

| Feature                                  | Implemented   |
|------------------------------------------|---------------|
| Message routing (Kafka)                  | ✅             |
| Sticky partitioning (consistent hashing) | ✅             |
| Hot partition mitigation (vnodes)        | ✅             |
| Horizontal scaling & switchover          | ✅             |
| Reconnection & Scylla sync               | ✅             |
| Delivery receipts pipeline               | ✅             |
| Sender notifications                     | ✅             |
| Secondary push delivery                  | ✅             |
| Conditional writes (CAS)                 | ✅             |
| Presence lag safety                      | ✅             |
| TTL cleanup & deduplication              | ✅             |

---

## Notes

- **Kafka + ScyllaDB** form the core backbone for routing, state, and durability.
- The system is designed for **resilience, consistency, and low-latency** delivery.
- Eventual consistency is guaranteed across replicas and delivery channels.
- Components are modular, allowing independent scaling:
  - Kafka brokers
  - App replicas
  - Push notification workers
  - Presence service

---

**End of Document**

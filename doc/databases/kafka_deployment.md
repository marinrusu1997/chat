# 📄 Kafka 4.1 KRaft Cluster Specification: High-Scale Chat Backend

This document details the configuration for a minimal (3-node), production-simulating Apache Kafka 4.1 cluster. It is designed specifically for a **high-scale, high-churn chat application** (Telegram/Discord analogy), incorporating advanced security, metadata management, and operational resilience.

## 1. Architectural & Operational Resilience

The deployment uses the minimal viable highly available **KRaft** architecture (`broker,controller` combined roles) to reduce local resource consumption while ensuring resilience.

| Aspect               | Configuration Setting       | Value / Strategy                             | Rationale                                                                                             |
|:---------------------|:----------------------------|:---------------------------------------------|:------------------------------------------------------------------------------------------------------|
| **Node Count**       | Total Nodes                 | **3**                                        | Minimal count for fault tolerance (maintains quorum $\ge 2$).                                         |
| **Node Role**        | `process.roles`             | `broker,controller`                          | Combined roles for local efficiency.                                                                  |
| **Consensus**        | `controller.quorum.voters`  | `1@kafka1:9093,2@kafka2:9093,3@kafka3:9093`  | Defines the KRaft metadata quorum.                                                                    |
| **Cluster ID**       | `cluster.id`                | Generated UUID (e.g., `21u3-a0b9-8c7d-e6f5`) | Unique identifier for the cluster.                                                                    |
| **File Limits (OS)** | `ulimit -n`                 | **$\ge$ 100,000**                            | **CRITICAL for high-churn.** Must be set high in Docker to handle millions of partition file handles. |
| **Topic Creation**   | `auto.create.topics.enable` | `false`                                      | Prevents misconfigurations and enforces explicit creation via Admin Client.                           |
| **Topic Deletion**   | `delete.topic.enable`       | `true`                                       | Enables application logic to clean up old user/group topics.                                          |

***

## 2. Security & Authentication Layer (SASL\_SSL + mTLS)

All communication is encrypted via **TLS/SSL**. Authentication uses **SASL/SCRAM** for clients and **mTLS** for inter-broker/controller communication.

| Listener Role             | Port   | Protocol   | Authentication               | Broker Setting                                                            |
|:--------------------------|:-------|:-----------|:-----------------------------|:--------------------------------------------------------------------------|
| **Client Access**         | `9092` | `SASL_SSL` | **SASL/SCRAM-SHA-512**       | Secure, challenge-response authentication for all application principals. |
| **Internal (KRaft/IB)**   | `9093` | `SSL`      | **mTLS (Certificate-Based)** | Encrypted, mutually authenticated communication between Kafka processes.  |
| **Protocol Enforcement**  | N/A    | N/A        | N/A                          | `ssl.protocol=TLSv1.3`                                                    | Enforces modern, secure TLS standard. |
| **Internal Auth Control** | N/A    | N/A        | N/A                          | `ssl.client.auth=required` (on 9093) enforces mTLS.                       |

### Security Principals (Least Privilege Enforced)

| Principal Name          | Role                 | Access Type                                 | Future Service Path                                |
|:------------------------|:---------------------|:--------------------------------------------|:---------------------------------------------------|
| `User:root_admin`       | Emergency Admin      | **Unrestricted** (via `super.users`)        | Troubleshooting only.                              |
| `User:chat_admin_plane` | Topic Manager        | **`CREATE`, `DELETE`** on Cluster           | Admin Client component (Future Dedicated Service). |
| `User:chat_producer`    | Message Outbound     | **`WRITE`** on all relevant topic patterns. | Application Producer component.                    |
| `User:chat_consumer`    | Message Inbound (RT) | **`READ`** on real-time and system topics.  | Application Consumer component.                    |
| `User:async_worker`     | Inter-Service Worker | **`READ`, `WRITE`** only on Async topics.   | Dedicated background service logic.                |

***

## 3. Topic Architecture, Guarantees & Quotas

### Topic Categories & Configuration

| Category                 | Naming Pattern                 | Partitions | Cleanup Policy | Rationale                                                                                    |
|:-------------------------|:-------------------------------|:-----------|:---------------|:---------------------------------------------------------------------------------------------|
| **DM/Group Inboxes**     | `user-inbox-*`, `chat-group-*` | **1**      | **`compact`**  | Essential for high-churn: minimizes metadata overhead; retains latest message (inbox state). |
| **System/Notifications** | `sys-push-notify-*`            | $\ge 6$    | `delete`       | High parallelism required. Time-based retention.                                             |
| **Async Communication**  | `async-service-a-*`            | $\ge 3$    | `delete`       | Standard microservice communication. Partition count for parallelism.                        |

### Data Integrity Guarantees (Client-Side)

| Client Setting   | Principal            | Value                     | Rationale                                                                       |
|:-----------------|:---------------------|:--------------------------|:--------------------------------------------------------------------------------|
| **Idempotence**  | `producer`, `worker` | `enable.idempotence=true` | **Exactly-once writing** guarantee.                                             |
| **Durability**   | `producer`, `worker` | `acks=all`                | Prevents data loss on leader failure.                                           |
| **Transactions** | `async_worker`       | `transactional.id` set    | Achieves **Exactly-Once-Semantics (EOS)** for service-to-service communication. |

### Broker Resource Quotas

Quotas protect the cluster from resource exhaustion by any single client principal.

| Quota Target              | Setting                 | Value (Example)         | Rationale                                                                                                  |
|:--------------------------|:------------------------|:------------------------|:-----------------------------------------------------------------------------------------------------------|
| **Producer Throttling**   | `producer_byte_rate`    | E.g., `100MB/sec`       | Throttles individual principals to prevent network saturation.                                             |
| **Consumer Throttling**   | `consumer_byte_rate`    | E.g., `200MB/sec`       | Throttles consumers to balance I/O for all clients.                                                        |
| **Controller Throttling** | `controller_rate_limit` | E.g., `20 requests/sec` | **CRITICAL.** Limits frequency of Admin API calls (`CREATE`/`DELETE` topic) to protect metadata stability. |

***

## 4. Authorization via Access Control Lists (ACLs)

ACLs enforce all privilege separation using **Prefix-Based Pattern matching** for dynamic topic types.

| Principal               | Resource Type | Resource Name Pattern                             | Operation(s)       |
|:------------------------|:--------------|:--------------------------------------------------|:-------------------|
| `User:chat_admin_plane` | **Cluster**   | `*`                                               | `CREATE`, `DELETE` |
| `User:chat_producer`    | Topic         | `user-inbox-` (PREFIXED), `sys-` (PREFIXED), etc. | `WRITE`            |
| `User:chat_consumer`    | Topic         | `user-inbox-` (PREFIXED), `sys-` (PREFIXED), etc. | `READ`, `DESCRIBE` |
| `User:async_worker`     | Topic         | `async-` (PREFIXED)                               | `READ`, `WRITE`    |
| All Data Principals     | Group         | `<group_prefix>-*` (PREFIXED)                     | `READ`, `DESCRIBE` |
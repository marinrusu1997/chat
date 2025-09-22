# Redis v8 — Production-Scale Performance Strategies

This document summarizes key strategies for running Redis v8 at production scale with a focus on **performance, reliability, scalability, availability, and consistency**.

---

## 1. Deployment Model
- Use **Redis Cluster** to shard horizontally and avoid single-node RAM limits.
- Design keys with hash tags (`{…}`) so related data lives on the same slot when needed.
- For extreme throughput/low latency, consider **Redis Enterprise / managed services**.

---

## 2. Redis 8 Features
- Enable **async I/O threading** (Redis 8 adds major optimizations).
- Benchmark with your workload to validate gains.
- For Redis Query Engine (RQE) / Search:
  - Use CURSOR/LIMIT for pagination.
  - Avoid `LOAD *`.
  - Enable query threading for long queries.

---

## 3. Data Modeling & Commands
- Choose **compact data types** (strings, hashes, streams, JSON as needed).
- Avoid large multi-key operations (`KEYS`, `SMEMBERS` on huge sets).
- Use **pipelining and batching** for throughput.
- Avoid **blocking commands** on large collections.

---

## 4. Memory Efficiency & Eviction
- Set `maxmemory` and an appropriate **eviction policy** (`allkeys-lru`, `volatile-lru`, etc.).
- Use `MEMORY USAGE` / `MEMORY STATS` to find big keys.
- Break down or compress large values.
- Monitor fragmentation with `MEMORY STATS`.

---

## 5. Persistence & Durability
- For highest throughput: **RDB snapshots** only, or disable persistence.
- For stronger durability: **AOF (appendonly)** with `everysec`.
- Schedule AOF rewrites and RDB snapshots off-peak to avoid I/O contention.

---

## 6. Replication & Failover
- Use replicas for reads and failover.
- Monitor replication lag (`INFO replication`).
- Tune `repl-backlog-size`.
- Test failover procedures regularly.

---

## 7. OS / Kernel / Hardware
- Favor **high single-thread performance CPUs**.
- Enable Redis 8 multi-core optimizations (I/O threads).
- Linux sysctls:
  - `vm.overcommit_memory=1`
  - `vm.swappiness=0`
  - Increase `net.core.somaxconn`
- Monitor NUMA and CPU placement.

---

## 8. Client & Networking
- Use connection pooling or long-lived connections.
- Reduce connection churn.
- Enable TCP keepalive.
- Avoid excessive fanout patterns — prefer pub/sub or streams.

---

## 9. Monitoring & Benchmarking
- Monitor via `INFO`, latency metrics, slowlog, replication lag, evicted keys.
- Use latency monitoring and `SLOWLOG` for analysis.
- Benchmark using `redis-benchmark` or realistic workloads.

---

## 10. Operational Pitfalls to Avoid
- Large keys and big replies — paginate or stream results.
- Blocking main thread with long Lua scripts or expensive commands.
- Unbounded memory growth — always configure `maxmemory`.

---

## 11. Security & ACLs
- Use ACLs, TLS, and network protection.
- Redis 8 tightened ACLs — review configs before upgrade.
- Enable protected-mode when appropriate.

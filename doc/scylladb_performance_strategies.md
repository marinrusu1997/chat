# ScyllaDB Performance Strategies

## 🔑 1. Data Modeling for Performance

-   **Denormalize**: Like Cassandra, ScyllaDB favors wide tables with
    pre-joined data. Minimize the number of queries needed per request.
-   **Partitioning**:
    -   Choose partition keys carefully to avoid **hotspots** (all
        writes going to the same node).
    -   Ensure partitions are **not too large** (ideally \<100MB per
        partition). Huge partitions kill performance.
-   **Clustering keys**: Use clustering keys for efficient range
    queries, but keep them ordered based on access patterns.
-   **Avoid tombstone floods**: Don't model data in a way that generates
    millions of deletes (e.g., inserting TTL for every record in a hot
    partition).

## ⚙️ 2. Query Optimization

-   **Single-table queries**: Query only by partition key whenever
    possible.
-   **Materialized views & secondary indexes**: Use with caution ---
    they can introduce write amplification. Often better to design
    tables specifically for each query pattern.
-   **Prepared statements**: Always use prepared queries to reduce
    parsing overhead.

## 🚀 3. Hardware & Deployment Tuning

-   **Shard-per-core architecture**: ScyllaDB uses one shard per CPU
    core. Performance scales linearly with the number of vCPUs.
-   **NUMA awareness**: Pin ScyllaDB shards to cores to avoid cross-NUMA
    latency.
-   **Fast storage**: NVMe SSDs or local SSDs are strongly recommended.
-   **Networking**:
    -   Use **10Gbps+ NICs** for cluster communication.
    -   Keep latency low and avoid noisy neighbors (especially in
        cloud).

## 🛠️ 4. Configuration & Tuning

-   **I/O tuning**: Use Scylla's `scylla_io_setup` tool to auto-tune
    disk schedulers and queues.
-   **Compaction strategy**:
    -   **SizeTieredCompactionStrategy (STCS)** → good for mostly write
        workloads.
    -   **LeveledCompactionStrategy (LCS)** → good for read-heavy
        workloads.
    -   **TimeWindowCompactionStrategy (TWCS)** → good for time-series
        data.
-   **Cache tuning**:
    -   Adjust **row cache** and **key cache** for your workload.
    -   Row cache helps read-heavy workloads with repeated queries.
-   **Commitlog settings**: Ensure commitlog is on fast storage.

## 📊 5. Monitoring & Diagnostics

-   **Scylla Monitoring Stack (Prometheus + Grafana)**:
    -   Watch metrics like **latency, pending compactions, tombstone
        scans, cache hit ratios, dropped mutations**.
    -   Look for uneven load across shards/nodes.
-   **Performance Advisor (Scylla Enterprise / Cloud)**: Provides
    automated tuning advice.
-   **nodetool / cqlsh tracing**: Debug hotspots and slow queries.

## 🌀 6. Cluster Management Best Practices

-   **Replication factor**: Use RF=3 for production (balances fault
    tolerance and performance).
-   **Repair & Rebalancing**: Run repairs periodically to avoid read
    amplification from inconsistencies.
-   **Avoid large batch writes**: Use batches only for atomicity across
    partitions, not bulk loading.
-   **Bulk loads**:
    -   Use Scylla's **SSTableloader** or **Scylla Migrator** for large
        imports.
    -   Avoid stressing the cluster with unthrottled writes.

## 📚 7. Application-Side Optimizations

-   **Async drivers**: Use ScyllaDB drivers with async I/O (e.g., Python
    asyncio, Node.js async, Go).
-   **Connection pooling**: Maintain a healthy pool of connections per
    shard.
-   **Token-aware load balancing**: Ensure queries are routed directly
    to the node responsible for a partition.

------------------------------------------------------------------------

✅ **Summary**:\
ScyllaDB performance comes down to:\
- Good **data model** (balanced partitions, minimal tombstones)\
- **Hardware & configuration tuning** (NVMe, proper compaction, caches)\
- **Monitoring & load balancing** (avoid hotspots, watch for uneven
shards)\
- **Application alignment** (async, prepared statements, token-aware
routing)

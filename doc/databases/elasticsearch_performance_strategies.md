# Elasticsearch Performance Optimization Strategies

## 1 — General principles
- **Measure first.** Identify hotspots with metrics (CPU, GC, disk I/O, network), Elasticsearch slow logs, and tracing before changing configuration.  
- **Test changes in a staging environment** with realistic data and queries; many optimizations trade off one thing for another (latency vs throughput, write vs read).

## 2 — Hardware & OS
- **Use fast low-latency storage (NVMe/SSD).**
- **Give the OS filesystem cache breathing room.** Don’t allocate all RAM to the JVM.
- **Network:** use high bandwidth / low-latency links between nodes.
- **Disable swapping:** configure `vm.swappiness=1` and ensure swap is off for ES processes.

## 3 — JVM & memory
- **Heap sizing:** set `-Xms`/`-Xmx` equal; generally use **no more than 50% of system RAM** for the JVM heap and avoid exceeding ≈31–32GB to keep compressed pointers.
- **Off-heap usage:** ES relies on OS file cache and native memory; therefore leave RAM for the OS.
- **Garbage collection:** use modern GC (G1 is default for supported JDKs); monitor GC pause times.

## 4 — Cluster topology & node roles
- **Separate node roles:** data-hot, data-warm, master, ingest, coordinating.
- **Dedicated master-eligible nodes** (small heap).
- **Avoid colocating heavy workloads** (e.g., ML, ingest heavy pipelines) on master nodes.

## 5 — Shard & index design
- **Shard size:** aim for **10–50 GB** per shard depending on workload.
- **Limit shard count:** avoid creating many tiny indices/shards.
- **Prefer time-based indices + ILM** for logs and metrics.

## 6 — Mappings & documents
- **Explicit mappings:** disable unused dynamic fields.
- **Avoid high-cardinality fields** in aggregations.
- **Use `keyword` vs `text` appropriately.**
- **Disable `_source` or store selectively** if retrieval isn’t needed.

## 7 — Indexing performance
- **Bulk API:** use the Bulk API with tuned sizes.
- **Disable refresh during heavy indexing:** `index.refresh_interval: -1` during bulk loads.
- **Disable replicas temporarily** during large indexing jobs.
- **Control translog and sync interval.**
- **Parallelize clients:** run multiple concurrent bulk workers.

## 8 — Refresh, merges, and force-merge
- **Refresh interval trade-off:** frequent refreshes increase IO; use `-1` for bulk.
- **Merges:** merging is IO heavy; let ES control merges.
- **Force-merge** to `max_num_segments=1` for read-only indices.

## 9 — Queries and aggregations
- **Profile slow queries** with Profile API and slow logs.
- **Avoid heavy scripts in hot paths.**
- **Use filters** for boolean checks (cacheable).
- **Use `doc_values` for sorting/aggregations.**
- **Page carefully:** prefer `search_after` instead of deep paging; use `scroll` for full scans.

## 10 — Caching & performance helpers
- **Query cache:** good for repeated identical queries.
- **Request cache:** good for heavy aggregations on read-only data.
- **Fielddata:** avoid on `text` fields.

## 11 — Ingest pipelines & analyzers
- **Pre-process docs with ingest pipelines** or ETL outside ES.
- **Keep analyzers simple** to reduce indexing CPU.

## 12 — Storage & compression
- **Use `best_compression`** for colder data.
- **Avoid storing large blobs** in ES.

## 13 — Monitoring & observability
- **Collect GC / node / cluster metrics.**
- **Enable slowlogs** for queries and indexing.

## 14 — Operations & lifecycle
- **Index Lifecycle Management (ILM):** roll, shrink, freeze, delete.
- **Snapshot/restore:** schedule regular backups.
- **Rolling restarts** for upgrades/config changes.

## 15 — Security & access control
- **Security features add CPU cost.** Test performance impact when enabling.

## 16 — Troubleshooting bottlenecks
- **High GC/heap usage:** reduce shards, avoid heavy aggregations, tune bulk sizes.
- **High disk I/O:** check merges, refreshes, disk health.
- **CPU bound:** profile queries, precompute expensive logic.

## 17 — Practical checklist
1. Verify hardware (SSDs, RAM, NIC).  
2. Check JVM heap: `-Xms=-Xmx` and keep ≤50% RAM (and ≤~31–32GB).  
3. Reassess shard sizing and shrink if many small shards exist.  
4. Use Bulk API with tuned batch sizes; disable refresh for bulk loads.  
5. Profile slow queries and aggregations; optimize mappings.  
6. Implement ILM for time-series/log data.

## 18 — References
- Elastic official docs: Optimize performance, Size your shards, Tune for indexing speed, JVM heap & GC guidance.
- Blogs & community guides: Opster, Sematext, Elastic community posts.

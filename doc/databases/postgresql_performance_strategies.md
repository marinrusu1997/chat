# PostgreSQL Performance Strategies

This document summarizes strategies and best practices for improving the performance of PostgreSQL.

---

## 1. Schema and Data Modeling
- Normalize vs. Denormalize appropriately.
- Use correct data types (e.g., `integer` vs. `bigint`).
- Partition large tables (range, list, hash).
- Avoid wide rows and use vertical partitioning if needed.

---

## 2. Indexes
- Choose index type wisely:
  - **B-Tree**: Equality/range queries.
  - **GIN**: Full-text search, JSONB.
  - **BRIN**: Large sequential/append-only tables.
  - **Hash**: Equality checks.
- Use covering indexes (`INCLUDE`) to eliminate lookups.
- Use partial indexes for subsets of data.
- Create multi-column indexes for common filters.
- Remove unused indexes to avoid write overhead.

---

## 3. Query Optimization
- Analyze queries with `EXPLAIN (ANALYZE, BUFFERS)`.
- Avoid `SELECT *` — fetch only needed columns.
- Batch inserts/updates instead of row-by-row.
- Use CTEs/materialized views where beneficial.
- Eliminate unnecessary joins or optimize subqueries.
- Run `VACUUM` and `ANALYZE` regularly.

---

## 4. Vacuuming and Autovacuum
- Tune autovacuum workers and thresholds.
- Use `VACUUM FULL` or `CLUSTER` for severe bloat.
- Use `pg_repack` for online cleanup.

---

## 5. Configuration Tuning
- Adjust PostgreSQL settings based on system resources:
  - `shared_buffers`: ~25–40% of RAM.
  - `work_mem`: 4–64MB per operation.
  - `maintenance_work_mem`: 512MB–2GB for VACUUM/CREATE INDEX.
  - `effective_cache_size`: ~50–75% of RAM.
  - `wal_buffers`: 16MB+ for write-heavy workloads.
  - `max_parallel_workers_per_gather`: Enable parallel queries.

---

## 6. Concurrency & Locking
- Use connection pooling (`PgBouncer`, `Pgpool-II`).
- Reduce long transactions that block autovacuum.
- Use `READ COMMITTED` isolation unless stricter levels are required.

---

## 7. Hardware and OS Level
- Prefer faster disks (NVMe SSDs > SATA SSDs > HDDs).
- Add more RAM for caching.
- Use multiple CPU cores for parallel queries.
- Tune OS settings (e.g., `vm.swappiness`, `dirty_background_bytes`).
- Place WAL (`pg_wal`) on a separate disk.

---

## 8. Sharding and Scaling Out
- Apply logical sharding by tenant, region, or time.
- Use Foreign Data Wrappers (FDW) for federated setups.
- Consider **Citus** for distributed workloads.
- Use **TimescaleDB** for time-series data.

---

## 9. Monitoring and Observability
- Enable `pg_stat_statements` to track slow queries.
- Use `auto_explain` to log bad plans.
- Monitor with Prometheus + Grafana.
- Watch cache hit ratio (should be > 99%).
- Regularly check for bloated indexes or excessive sequential scans.

---

## Rule of Thumb
1. Fix schema and indexes first.  
2. Tune queries next.  
3. Adjust PostgreSQL config after that.  
4. Finally, consider hardware upgrades or sharding.

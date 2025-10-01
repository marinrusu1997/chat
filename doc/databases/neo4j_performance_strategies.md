# Neo4j Performance Strategies

## 1. Data Modeling
- Use the right graph model: favor relationships over deeply nested properties.
- Avoid overloading nodes with too many properties.
- Use labels effectively to segment data and narrow queries.
- Use relationship types for fast traversals instead of property filtering.
- Keep relationship directions consistent.

## 2. Indexes & Constraints
- Create indexes on frequently queried properties.
- Use composite indexes for multi-property lookups.
- Define unique constraints (creates automatic indexes).
- Use full-text indexes for search-heavy workloads.

## 3. Query Optimization
- Profile queries with `EXPLAIN` and `PROFILE`.
- Avoid `MATCH (n) RETURN n` (full scans).
- Prefer pattern-based traversals instead of property filtering.
- Use `OPTIONAL MATCH` carefully.
- Reduce the use of `DISTINCT` unless needed.
- Limit returned data (`LIMIT`, `SKIP`, projections).
- Avoid Cartesian products by ensuring relationships exist.

## 4. Transactions
- Keep transactions short and focused.
- Avoid batching too much in a single transaction.
- For large inserts/updates, use periodic commits.

## 5. Caching & Memory
- Ensure sufficient heap and page cache.
- Use query result caching where possible.
- Tune `dbms.memory.pagecache.size` based on dataset size.
- Warm up cache with important queries after startup.

## 6. Batching & Import
- For initial imports, use `neo4j-admin import`.
- For large-scale updates, batch inserts/updates (e.g., 10k nodes per batch).
- Use `UNWIND` for efficient batch inserts.

## 7. Monitoring & Diagnostics
- Monitor queries with Query Logging.
- Use `CALL dbms.listQueries()` for slow query detection.
- Enable metrics reporting (Prometheus/Grafana).
- Regularly review slow query logs.

## 8. Deployment & Scaling
- Scale vertically (RAM + fast SSDs).
- Use read replicas for read-heavy workloads.
- Distribute queries across cluster members with routing drivers.
- Avoid heavily contended shared hardware.

## 9. Best Practices
- Test with production-like datasets.
- Run load tests with realistic queries.
- Refactor queries and data model iteratively.
- Keep Neo4j updated for better performance.

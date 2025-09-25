# Elasticsearch Deployment Strategies

## 1 — Deployment Models

### 1.1 Self-managed
- **Bare metal:** Ultimate performance, dedicated SSD/NVMe, large RAM.
- **VMs (on-prem or cloud IaaS):** Most common option (AWS, GCP, Azure, OpenStack, etc.).
- **Containers (Docker/Kubernetes):** Increasingly popular with Elastic Cloud on Kubernetes (ECK).
- **Orchestration:** Helm charts, ECK, or custom manifests handle scaling, rolling upgrades, resilience.

### 1.2 Managed Service
- **Elastic Cloud (by Elastic):** Official managed Elasticsearch service (AWS, GCP, Azure).
- **AWS OpenSearch Service:** Amazon’s fork of Elasticsearch, fully managed.
- **Third-party providers:** Aiven, Bonsai, Qbox, etc.

---

## 2 — Typical Cluster Layout

### Node Roles
- **Master nodes (dedicated):** Usually 3 for quorum (small VMs). Handle cluster state.
- **Hot data nodes:** High-performance (SSD, large heap) for recent data, indexing, queries.
- **Warm data nodes:** Slower, cheaper hardware for older but still searchable data.
- **Cold data nodes:** Cheapest tier (object storage, searchable snapshots).
- **Ingest nodes:** Run pipelines for enrichment and parsing.
- **Coordinating nodes (optional):** Front-door query routers.

### Hot-Warm-Cold Tiering
- Balances cost vs performance: recent data → hot, older → warm, archived → cold.

---

## 3 — Scaling Strategies
- **Vertical scaling:** Increase RAM, CPU, faster disks.
- **Horizontal scaling:** Add nodes/shards and rebalance indices.
- **Shard management:** Plan shard counts carefully; use ILM rollover to keep indices balanced.

---

## 4 — High Availability
- **Replication:** At least 1 replica per shard for redundancy.
- **Dedicated master nodes (3+)** to prevent split brain.
- **Availability zones:** Spread nodes across AZs/racks.
- **Snapshot backups:** Regular snapshots to external storage (S3, GCS, etc.).

---

## 5 — Security & Networking
- **Transport encryption (TLS) and authentication** with X-Pack security.
- **Private networking:** Deploy inside VPCs or private subnets.
- **Access control:** Reverse proxy, API gateway, or Kibana with authentication.

---

## 6 — Deployment Automation
- **Infrastructure-as-Code (IaC):** Terraform, Ansible, Pulumi, etc.
- **Config management:** Ansible, Chef, Puppet, or Kubernetes manifests.
- **CI/CD pipelines:** Automate index templates, mappings, ILM policies, dashboards.

---

## 7 — Operations
- **Rolling upgrades:** Restart/replace nodes one at a time.
- **Monitoring & alerting:** Elastic Stack (Kibana + Beats) or Prometheus + Grafana.
- **Capacity planning:** Track shard count, disk usage, heap pressure.
- **Chaos testing:** Validate resilience by simulating node failures.

---

## 8 — Example Production Setups

### Kubernetes + ECK
- Roles split across StatefulSets.
- Storage provisioned with CSI (SSD for hot, HDD for warm).
- Autoscaling possible (experimental for data nodes).

### Classic VMs
- 3 master nodes (small VMs).
- 6–12 hot data nodes (SSD-backed).
- Optional warm nodes with larger/cheaper disks.
- Load balancer → coordinating nodes → data nodes.

### Elastic Cloud (Managed)
- Roles and tiers managed automatically by Elastic.
- Easy scaling and built-in observability.

---

## 9 — Rules of Thumb for Production
- Separate **master** and **data roles**.
- Always run **at least 3 master nodes**.
- Use **SSD/NVMe** for hot tier nodes.
- Enable **replication and snapshots**.
- Automate deployment and monitoring.

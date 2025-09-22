#!/bin/bash
set -e

# ==================== DIAGNOSTIC BLOCK (using /proc) ====================
echo "========================================================="
echo "EXECUTING ENTRYPOINT SCRIPT..."
echo "  - My PID is: $$"
echo "  - My Parent's PID is: $PPID"

# Get details about the parent process directly from the /proc filesystem
if [ -d "/proc/$PPID" ]; then
    echo "--- Parent Process Details ---"
    echo "  - Parent command name: $(cat /proc/$PPID/comm)"
    echo "  - Parent command line: $(cat /proc/$PPID/cmdline | tr '\0' ' ')"
fi
echo "========================================================="
# ========================================================================

# Import dependencies
. /usr/local/etc/redis/scripts/logger.sh

# ==============================================================================
# Redis Node Entrypoint Script
#
# - Starts the Redis server in the background.
# - If this container is 'redis-node-1', it acts as the cluster initiator.
#   - Waits for all nodes to be ready.
#   - Checks if the cluster is already initialized.
#   - Creates the cluster if needed.
# - Waits for the Redis server process, proxying signals for graceful shutdown.
# ==============================================================================

# --- Cluster Initialization Logic ---
# This logic only runs on the designated initiator node.
run_check() {
  local name="$1"
  local actual="$2"
  local expected="$3"

  # The core comparison logic remains the same.
  if [ "$actual" = "$expected" ]; then
    # On success, log an INFO message.
    log_info "$name" "✅ Check PASSED. (Value: ${actual})"
  else
    # On failure, log a FATAL message. This also handles exiting the script.
    log_fatal "$name" "❌ Check FAILED. Expected: '${expected}', Got: '${actual}'"
  fi
}

verify_cluster_health() {
  # 1. Assign arguments to named local variables
  local total_nodes=$1
  local replication_factor=$2
  local admin_password=$3
  local coordinator_node_host=$4
  local coordinator_node_port=$5

  # 2. Validate inputs using the logger for error handling
  if [ $((total_nodes % 2)) -ne 0 ]; then
    log_fatal "InputValidation" "❌ Total number of nodes must be an even number. Received: $total_nodes"
  fi

  log_info "Verification" "⏳ Starting cluster health checks. Waiting for 10s..."
  sleep 10

  # 3. Dynamically calculate expected master counts from arguments
  local expected_masters=$((total_nodes / (1 + replication_factor)))

  # Capture CLUSTER INFO and CLUSTER NODES output once for efficiency
  local cluster_info
  cluster_info=$(REDISCLI_AUTH="$admin_password" redis-cli -h "$coordinator_node_host" -p "$coordinator_node_port" CLUSTER INFO)
  local node_info
  node_info=$(REDISCLI_AUTH="$admin_password" redis-cli -h "$coordinator_node_host" -p "$coordinator_node_port" CLUSTER NODES)

  # --- Run all checks ---
  local cluster_state
  cluster_state=$(echo "$cluster_info" | grep 'cluster_state' | cut -d: -f2 | tr -d '\r')
  run_check "Overall Cluster State" "$cluster_state" "ok"

  local slots_ok
  slots_ok=$(echo "$cluster_info" | grep 'cluster_slots_ok' | cut -d: -f2 | tr -d '\r')
  run_check "Healthy Slot Count" "$slots_ok" "16384"

  local known_nodes
  known_nodes=$(echo "$cluster_info" | grep 'cluster_known_nodes' | cut -d: -f2 | tr -d '\r')
  run_check "Total Node Count" "$known_nodes" "$total_nodes"

  local master_count
  master_count=$(echo "$cluster_info" | grep 'cluster_size' | cut -d: -f2 | tr -d '\r')
  run_check "Master Node Count" "$master_count" "$expected_masters"

  local failed_nodes
  failed_nodes=$(echo "$node_info" | grep 'fail' | wc -l)
  run_check "Failed Node Count" "$failed_nodes" "0"

  # Verify replica topology for each master
  local master_ids
  master_ids=$(echo "$node_info" | grep 'master' | cut -d' ' -f1)

  local extracted_master_ids_count
  extracted_master_ids_count=$(echo "$master_ids" | wc -l)
  run_check "Parsed Master ID Count" "$extracted_master_ids_count" "$expected_masters"

  for master_id in $master_ids; do
    local replica_count_for_master
    replica_count_for_master=$(REDISCLI_AUTH="$admin_password" redis-cli -h "$coordinator_node_host" -p "$coordinator_node_port" CLUSTER REPLICAS "$master_id" | wc -l)
    run_check "Replicas for master $master_id" "$replica_count_for_master" "$replication_factor"
  done

  log_info "Verification" "✅ All cluster health checks passed successfully."
}

log_cluster_status() {
  local admin_password=$1
  local host=$2
  local port=$3

  log_info "ClusterStatus" "⏳ Fetching final cluster state..."

  # Capture info and nodes output for clean logging
  local cluster_info
  cluster_info=$(REDISCLI_AUTH="$admin_password" redis-cli -h "$host" -p "$port" CLUSTER INFO)
  local cluster_nodes
  cluster_nodes=$(REDISCLI_AUTH="$admin_password" redis-cli -h "$host" -p "$port" CLUSTER NODES)

  # Use a multi-line log message for readability
  log_info "ClusterStatus" "
    --------------------- ℹ️ CLUSTER INFO ----------------------
    ${cluster_info}
    --------------------- 📍 CLUSTER NODES ---------------------
    ${cluster_nodes}
    -------------------------------------------------------"
}

initialize_cluster() {
  # 1. Assign arguments to local variables
  local coordinator_node=$1 # coordinator_node_hostname (string)
  local total_nodes=$2 # total_nodes (integer)
  local replication_factor=$3 # replication_factor (integer)
  declare -n redis_passwords_ref="$4" # redis_passwords (map reference)
  local node_base_name=$5 # node_base_name (string, e.g., "redis-node")
  local port=$6 # redis_port (integer, e.g., 6379)

  # 2. Check if this container is the designated coordinator.
  # If not, do nothing and exit the function successfully.
  if [ "$(hostname)" != "$coordinator_node" ]; then
    log_debug "ClusterInitialization" "This node ($(hostname)) is not the coordinator ($coordinator_node). Skipping initialization."
    return 0
  fi

  log_info "ClusterInitialization" "This node is the coordinator. Starting initialization process..."

  # 3. Dynamically generate the list of all node addresses
  local redis_nodes=""
  local i
  for i in $(seq 1 "$total_nodes"); do
    redis_nodes="$redis_nodes$node_base_name-$i:$port "
  done
  redis_nodes=${redis_nodes% } # Trim trailing space

  # 4. Wait for all nodes to become responsive
  log_info "ClusterInitialization" "⏳ Waiting for all $total_nodes nodes to be online..."
  for node_addr in $redis_nodes; do
    local host
    host=$(echo "$node_addr" | cut -d: -f1)
    local port
    port=$(echo "$node_addr" | cut -d: -f2)

    until (REDISCLI_AUTH="${redis_passwords_ref["admin"]}" redis-cli -h "$host" -p "$port" PING | grep -q "PONG") || true; do
      log_debug "NodeAvailability" "  - Pinging $host:$port..."
      sleep 2
    done
    log_info "ClusterInitialization" "✅ Node $host:$port is online."
  done

  # 5. Check if the cluster is already initialized to ensure idempotency
  log_info "ClusterInitialization" "⏳ Checking cluster status..."
  local known_nodes
  known_nodes=$(REDISCLI_AUTH="${redis_passwords_ref["admin"]}" redis-cli -h "$coordinator_node" -p "$port" CLUSTER INFO | grep 'cluster_known_nodes' | cut -d: -f2 | tr -d '\r')

  if [ "$known_nodes" -eq "1" ]; then
    log_warn "ClusterInitialization" "⏳ Cluster not initialized. Proceeding with creation. Waiting 10s for nodes to be able to form a cluster..."
    sleep 10

    if ! echo "yes" | REDISCLI_AUTH="${redis_passwords_ref["admin"]}" redis-cli -h "$coordinator_node" -p "$port" --cluster create $redis_nodes --cluster-replicas "$replication_factor"; then
        log_fatal "Create" "Cluster creation command failed."
    fi
    log_info "ClusterInitialization" "✅ Cluster created successfully with $total_nodes nodes and replication factor $replication_factor."
  else
    log_info "ClusterInitialization" "Cluster already has $known_nodes nodes. Skipping creation."
  fi

  # 6. Verify the cluster health after creation or if it already existed
  verify_cluster_health "$total_nodes" "$replication_factor" "${redis_passwords_ref["admin"]}" "$coordinator_node" "$port"

  # 7. Print the final state of the cluster
  log_cluster_status "${redis_passwords_ref["admin"]}" "$coordinator_node" "$port"
}

startup () {
  local redis_pid
  local redis_command_args="$*"
  local coordinator="redis-node-1"
  local total_nodes=6
  local replication_factor=1
  local node_base_name="redis-node"
  local redis_port=6379
  declare -A redis_passwords=(
    ["admin"]="$(cat "/usr/local/etc/redis/secrets/default.pass")"
    ["replicator"]="$(cat "/usr/local/etc/redis/secrets/replicator.pass")"
  )

  # --- Graceful Shutdown Handler ---
  # This trap ensures that 'docker stop' triggers a clean Redis shutdown,
  # allowing it to persist data before exiting.
  trap 'shutdown' TERM INT
  shutdown() {
    log_warn "Shutdown" "Received signal, initiating graceful shutdown..."
    # Extract port from the command arguments to send the SHUTDOWN command
    local port
    port=$(echo "$redis_command_args" | grep -oP -- '--port \K\d+')
    port=${port:-$redis_port}

    # Use redis-cli SHUTDOWN for a graceful stop
    if ! REDISCLI_AUTH="${redis_passwords["admin"]}" redis-cli -h "$(hostname)" -p "$port" SHUTDOWN; then
        log_error "Shutdown" "SHUTDOWN command failed. Killing process."
        kill -TERM "$redis_pid"
    fi
    wait "$redis_pid" # Wait for the process to terminate
    log_info "Shutdown" "Redis server has stopped."
  }

  # 1. Start Redis server in the background
  redis-server "$@" &
  redis_pid=$!
  log_info "Startup" "Redis server process started with PID: $redis_pid"

  # 2. Run the one-time cluster initialization logic
  # This function internally checks if it's the coordinator before acting.
  initialize_cluster \
    "$coordinator" \
    "$total_nodes" \
    "$replication_factor" \
    "redis_passwords" \
    "$node_base_name" \
    "$redis_port"

  # 3. Wait for the Redis process to exit
  # This keeps the container running and allows the trap to work.
  wait "$redis_pid"
  log_info "Shutdown" "Redis server has stopped."
}

startup "$@"

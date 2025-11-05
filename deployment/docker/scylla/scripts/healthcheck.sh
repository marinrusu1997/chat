#!/usr/bin/env bash
set -Eeuo pipefail

declare -r LOCK_DIR="/var/lib/scylla/locks"
declare -r REPAIR_LOCK_FILE="$LOCK_DIR/repair.lock"
declare -r MIN_NUMBER_OF_NODES=3
declare -r MAINTENANCE_HOSTNAME="scylla-node-1"
declare -r MAINTENANCE_LOG_FILE="/var/lib/scylla/maintenance.log"

NODETOOL_STATUS=""
if ! NODETOOL_STATUS=$(nodetool status 2>/dev/null); then
	echo "--> [healthcheck] nodetool failed to connect to JMX"
	exit 1
fi
if ! echo "$NODETOOL_STATUS" | grep -q "Datacenter:"; then
	echo "--> [healthcheck] unexpected nodetool output"
	exit 1
fi
declare -r NODETOOL_STATUS

is_node_healthy() {
	local -r host_ip=$(hostname -i)
	local -r status=$(echo "$NODETOOL_STATUS" | grep -E "UN|UJ|UL|UM" | grep "$host_ip" || true)

	if echo "$status" | grep -q "UN"; then
		return 0 # healthy (true)
	else
		return 1 # unhealthy (false)
	fi
}

is_cluster_ready() {
	local -r nodes_un=$(echo "$NODETOOL_STATUS" | awk '/UN/ {count++} END {print count+0}')
	local -r nodes_total=$(echo "$NODETOOL_STATUS" | grep -E 'UN|UJ|UL|UM|DN|DJ|DL|DM' | wc -l)

	if [[ "$nodes_total" -lt "$MIN_NUMBER_OF_NODES" ]]; then
		echo "--> [healthcheck] not enough nodes discovered ($nodes_total < $MIN_NUMBER_OF_NODES)"
		return 1
	fi

	if [[ "$nodes_un" -eq "$nodes_total" ]]; then
		return 0 # healthy (true)
	else
		return 1 # unhealthy (false)
	fi
}

should_run_daily_repair() {
	local -r now=$(date +%s)
	local last_run=0

	if [[ -f "$REPAIR_LOCK_FILE" ]]; then
		last_run=$(stat -c %Y "$REPAIR_LOCK_FILE")
	else
		touch "$REPAIR_LOCK_FILE"
		return 1 # "pretend we don't need to run" and schedule first run
	fi

	if ((now - last_run > 86400)); then
		touch "$REPAIR_LOCK_FILE" # update timestamp
		return 0                  # need to run
	fi
	return 1 # don't need to run
}

if ! is_node_healthy; then
	echo "--> [healthcheck] local node unhealthy"
	exit 1
fi

if is_cluster_ready; then
	if [[ "$(hostname)" == "$MAINTENANCE_HOSTNAME" ]]; then
		mkdir -p "$LOCK_DIR"

		if should_run_daily_repair; then
			nohup bash -c "nodetool cluster repair && nodetool repair && nodetool cleanup && nodetool compact" >"$MAINTENANCE_LOG_FILE" 2>&1 &
		fi
	fi
fi

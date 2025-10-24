#!/bin/bash

# ==============================================================================
# Redis Cluster Node Health Check Script
#
# This script checks the health of a Redis Cluster node by verifying that the
# cluster state is 'ok'. It securely reads the replicator password from a
# file and uses the REDISCLI_AUTH environment variable to authenticate.
#
# It exits with status 0 on success and 1 on failure.
# ==============================================================================

. /usr/local/etc/redis/scripts/logger.sh
. /usr/local/etc/redis/scripts/client.sh

# --- Configuration ---
# Path to the file containing the replicator's password.
PASS_FILE="/usr/local/etc/redis/secrets/default.pass"

# 1. Check if the password file exists and is readable.
if [ ! -r "$PASS_FILE" ]; then
	log_fatal "AdminPasswordFile" "❌ Password file not found or not readable at $PASS_FILE"
fi

# 2. Read the password from the file.
ADMIN_PASS=$(cat "$PASS_FILE")

# 3. Perform the health check.
log_info "HealthCheck" "⏳ Performing health check..."
if redis_cli "$ADMIN_PASS" CLUSTER INFO | grep -q "cluster_state:ok"; then
	log_info "HealthCheck" "✅ PASSED: Cluster state is ok."
	exit 0
else
	log_fatal "HealthCheck" "❌ FAILED: Could not verify cluster state."
fi

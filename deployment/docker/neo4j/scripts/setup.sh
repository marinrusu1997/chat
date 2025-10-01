#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# This script runs on the first startup of the container.
# It waits for Neo4j to be available and then sets up users, roles,
# a new database, and the initial schema.

# Import dependencies
. /tmp/scripts/logger.sh

LOCK_FILE="/data/neo4j-initialized.lock"
if [ -f "$LOCK_FILE" ]; then
  log_info "Initialization" "🚀 Neo4j has already been initialized. Exiting setup script."
  exit 0
fi

# --- Wait for Neo4j to be ready ---
log_info "Initialization" "⏳ Waiting for Neo4j to start..."
until cypher-shell -u neo4j -p "$SCRIPT_PASSWORD_NEO4J" "RETURN 1" > /dev/null 2>&1; do
  sleep 2
done
log_info "Initialization" "✅ Neo4j is ready!"

log_info "Initialization" "⏳ Setting up schema in 'chatdb' database..."
cypher-shell -u neo4j -p "$SCRIPT_PASSWORD_NEO4J" -d chatdb --file /schema/chat.cypher
log_info "Initialization" "✅ Schema setup complete. Initialization finished."

# Create a lock file to indicate that initialization has been done
touch "$LOCK_FILE"
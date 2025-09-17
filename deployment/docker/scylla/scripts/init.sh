#!/bin/sh

# Exit immediately if a command exits with a non-zero status.
set -e

# Wait for Scylla to be ready
echo "$(date '+%F %T') ⏳ Waiting for Scylla to be ready..."
while ! cqlsh scylla-node1 -u cassandra -p cassandra -e 'describe cluster'; do
  sleep 2
done
echo "$(date '+%F %T') ✅ Scylla is ready."

# === Security Setup ===
echo "$(date '+%F %T') 📝 Applying security configuration..."

# Change the default superuser password using the variable from the .env file
cqlsh scylla-node1 -u cassandra -p cassandra \
  -e "ALTER ROLE cassandra WITH PASSWORD = '$SCYLLA_SUPERUSER_PASSWORD';"

# Create the application user with the password from the .env file
cqlsh scylla-node1 -u cassandra -p "$SCYLLA_SUPERUSER_PASSWORD" \
  -e "CREATE ROLE IF NOT EXISTS '$APP_USER_USERNAME' WITH PASSWORD = '$APP_USER_PASSWORD' AND LOGIN = true;"
cqlsh scylla-node1 -u cassandra -p "$SCYLLA_SUPERUSER_PASSWORD" \
  -e "CREATE SERVICE LEVEL IF NOT EXISTS realtime_workload WITH SHARES = 1000 AND timeout = 1000ms;"
cqlsh scylla-node1 -u cassandra -p "$SCYLLA_SUPERUSER_PASSWORD" \
  -e "LIST ALL SERVICE_LEVELS;"
cqlsh scylla-node1 -u cassandra -p "$SCYLLA_SUPERUSER_PASSWORD" \
  -e "LIST ALL ATTACHED SERVICE_LEVELS;"
cqlsh scylla-node1 -u cassandra -p "$SCYLLA_SUPERUSER_PASSWORD" \
  -e "ATTACH SERVICE LEVEL realtime_workload TO '$APP_USER_USERNAME';"
cqlsh scylla-node1 -u cassandra -p "$SCYLLA_SUPERUSER_PASSWORD" \
  -e "LIST EFFECTIVE SERVICE LEVEL OF '$APP_USER_USERNAME';"

echo "$(date '+%F %T') ✅ Security configuration applied."

# === Schema and Permissions Setup ===
echo "$(date '+%F %T') 📝 Applying schema and permissions from init.cql..."

# Run the main schema script, which now includes the GRANT statement
cqlsh scylla-node1 -u cassandra -p "$SCYLLA_SUPERUSER_PASSWORD" -f /tmp/init.cql
cqlsh scylla-node1 -u cassandra -p "$SCYLLA_SUPERUSER_PASSWORD" \
  -e "GRANT ALL PERMISSIONS ON KEYSPACE chat_app TO '$APP_USER_USERNAME';"

echo "$(date '+%F %T') ✅ Initialization complete."
#!/bin/sh

# Exit immediately if a command exits with a non-zero status.
set -e

LOCK_FILE="/tmp/initializer.lock"
SCYLLA_SUPERUSER_USERNAME=cassandra
SCYLLA_HOST=scylla-node1

if [ ! -f "$LOCK_FILE" ]; then
    # Wait for Scylla to be ready
    echo "$(date '+%F %T') ⏳ Waiting for Scylla to be ready..."
    while ! cqlsh $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME -p cassandra -e 'describe cluster'; do
      sleep 2
    done
    echo "$(date '+%F %T') ✅ Scylla is ready."

    # === Security Setup ===
    echo "$(date '+%F %T') 📝 Applying security configuration..."

    # Change the default superuser password using the variable from the .env file
    cqlsh $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME -p cassandra \
      -e "ALTER ROLE $SCYLLA_SUPERUSER_USERNAME WITH PASSWORD = '$SCYLLA_SUPERUSER_PASSWORD';"

    # Create the application user with the password from the .env file
    cqlsh $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME -p "$SCYLLA_SUPERUSER_PASSWORD" \
      -e "CREATE ROLE IF NOT EXISTS '$APP_USER_USERNAME' WITH PASSWORD = '$APP_USER_PASSWORD' AND LOGIN = true;"
    cqlsh $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME -p "$SCYLLA_SUPERUSER_PASSWORD" \
      -e "CREATE SERVICE LEVEL IF NOT EXISTS realtime_workload WITH SHARES = 1000 AND timeout = 2000ms;"
    cqlsh $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME -p "$SCYLLA_SUPERUSER_PASSWORD" \
      -e "LIST ALL SERVICE_LEVELS;"
    cqlsh $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME -p "$SCYLLA_SUPERUSER_PASSWORD" \
      -e "LIST ALL ATTACHED SERVICE_LEVELS;"
    cqlsh $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME -p "$SCYLLA_SUPERUSER_PASSWORD" \
      -e "ATTACH SERVICE LEVEL realtime_workload TO '$APP_USER_USERNAME';"
    cqlsh $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME -p "$SCYLLA_SUPERUSER_PASSWORD" \
      -e "LIST EFFECTIVE SERVICE LEVEL OF '$APP_USER_USERNAME';"

    echo "$(date '+%F %T') ✅ Security configuration applied."

    # === Schema and Permissions Setup ===
    echo "$(date '+%F %T') 📝 Applying schema and permissions from init.cql..."

    # Run the main schema script, which now includes the GRANT statement
    cqlsh $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME -p "$SCYLLA_SUPERUSER_PASSWORD" -f /tmp/init.cql
    cqlsh $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME -p "$SCYLLA_SUPERUSER_PASSWORD" \
      -e "GRANT ALL PERMISSIONS ON KEYSPACE chat_db TO '$APP_USER_USERNAME';"

    echo "$(date '+%F %T') ✅ Initialization complete."
    touch "$LOCK_FILE"
fi

echo "$(date '+%F %T') ⏳ Performing repair..."
nodetool --host $SCYLLA_HOST -u $SCYLLA_SUPERUSER_USERNAME --password "$SCYLLA_SUPERUSER_PASSWORD" cluster repair chat_db
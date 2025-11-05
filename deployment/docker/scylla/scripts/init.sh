#!/bin/bash
set -Eeuo pipefail

declare -r LOCK_DIR="/var/lib/scylla/locks"
declare -r INITIALIZATION_LOCK_FILE="$LOCK_DIR/initialization.lock"
declare -r INIT_CQL_FILE="/usr/local/bin/scylla/init.cql"
declare -r SCYLLA_SUPERUSER_USERNAME=cassandra
declare -r SCYLLA_HOST=scylla-node-1
declare -r SCYLLA_PORT=9142

cqlsh_scylla() {
	cqlsh "$SCYLLA_HOST" "$SCYLLA_PORT" -C --ssl --cqlshrc /etc/scylla/client/cqlshrc -u cassandra "$@"
}

if [ ! -f "$INITIALIZATION_LOCK_FILE" ]; then
	echo "$(date '+%F %T') ⏳ Initializing ScyllaDB..."

	echo "$(date '+%F %T') ⏳ Waiting for Scylla to be ready..."
	while ! cqlsh_scylla -p cassandra -e 'describe cluster'; do
		sleep 2
	done
	echo "$(date '+%F %T') ✅ Scylla is ready."

	echo "$(date '+%F %T') 📝 Applying security configuration..."

	# Change the default superuser password using the variable from the .env file
	cqlsh_scylla -p cassandra -e "ALTER ROLE $SCYLLA_SUPERUSER_USERNAME WITH PASSWORD = '$SCYLLA_SUPERUSER_PASSWORD';"

	# Create the application user with the password from the .env file
	cqlsh_scylla -p "$SCYLLA_SUPERUSER_PASSWORD" \
		-e "CREATE ROLE IF NOT EXISTS '$APP_USER_USERNAME' WITH PASSWORD = '$APP_USER_PASSWORD' AND LOGIN = true;"
	cqlsh_scylla -p "$SCYLLA_SUPERUSER_PASSWORD" \
		-e "CREATE SERVICE LEVEL IF NOT EXISTS realtime_workload WITH SHARES = 1000 AND timeout = 2000ms;"
	cqlsh_scylla -p "$SCYLLA_SUPERUSER_PASSWORD" -e "LIST ALL SERVICE_LEVELS;"
	cqlsh_scylla -p "$SCYLLA_SUPERUSER_PASSWORD" -e "LIST ALL ATTACHED SERVICE_LEVELS;"
	cqlsh_scylla -p "$SCYLLA_SUPERUSER_PASSWORD" -e "ATTACH SERVICE LEVEL realtime_workload TO '$APP_USER_USERNAME';"
	cqlsh_scylla -p "$SCYLLA_SUPERUSER_PASSWORD" -e "LIST EFFECTIVE SERVICE LEVEL OF '$APP_USER_USERNAME';"

	echo "$(date '+%F %T') ✅ Security configuration applied."

	# === Schema and Permissions Setup ===
	echo "$(date '+%F %T') 📝 Applying schema and permissions from $INIT_CQL_FILE..."

	# Run the main schema script, which now includes the GRANT statement
	cqlsh_scylla -p "$SCYLLA_SUPERUSER_PASSWORD" -f "$INIT_CQL_FILE"
	cqlsh_scylla -p "$SCYLLA_SUPERUSER_PASSWORD" -e "GRANT ALL PERMISSIONS ON KEYSPACE chat_db TO '$APP_USER_USERNAME';"

	mkdir -p "$LOCK_DIR"
	touch "$INITIALIZATION_LOCK_FILE"
fi

echo "$(date '+%F %T') ✅ Initialization complete"
exec tail -f /dev/null
exit 0

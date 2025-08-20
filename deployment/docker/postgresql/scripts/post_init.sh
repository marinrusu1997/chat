#!/usr/bin/env bash
set -e

# Loop over only files matching db_user_*.pass
for f in /run/secrets/db_user_*.pass; do
    # Skip if no files match
    [ -e "$f" ] || continue

    # Extract base filename without extension
    fname=$(basename "$f" .pass)

    # Remove the "db_user_" prefix to get the username
    username=${fname#db_user_}

    # Convert to uppercase
    uppername=$(echo "$username" | tr '[:lower:]' '[:upper:]')

    # Export environment variable for the username
    export "${uppername}_USERNAME"="$username"

    # Export environment variable for the password
    export "${uppername}_PASSWORD"="$(cat "$f")"
done

# Define the connection strings
pg_connect() {
    local user_var=$1   # e.g., "ADMIN"
    local db=$2         # e.g., "chat_db"
    shift 2             # shift away user_var and db

    # Collect extra flags until the last argument (command)
    local extra_flags=()
    while [[ $# -gt 1 ]]; do
        extra_flags+=("$1")
        shift
    done

    local command=$1    # last arg is SQL or file path

    # Get username and password
    local username_var="${user_var}_USERNAME" # e.g., "ADMIN_USERNAME", which is already exported above
    local password_var="${user_var}_PASSWORD" # e.g., "ADMIN_PASSWORD", which is already exported above
    local username=${!username_var} # indirect expansion, e.g. 'admin'
    local password=${!password_var} # indirect expansion, e.g. 'password'

    # Stop if any error occurs, don't read ~/.psqlrc, don't prompt for password, disable pager
    local flags="--set ON_ERROR_STOP=1 --no-psqlrc --no-password -P pager=off"

    # Ternary-like: choose -f or -c based on whether command is a file
    # shellcheck disable=SC2155
    local exec_flag=$([ -f "$command" ] && echo "-f" || echo "-c")

    # Execute psql
    PGPASSWORD="$password" psql -U "$username" -d "$db" $flags "${extra_flags[@]}" $exec_flag "$command"
}

goose_c() {
    # Collect extra flags until the last argument (command)
    local extra_flags=()
    while [[ $# -gt 1 ]]; do
        extra_flags+=("$1")
        shift
    done

    local command=$1    # last arg is command

    # Execute goose
    GOOSE_DRIVER="postgres" \
    GOOSE_DBSTRING="dbname=$CHAT_DB host=/var/run/postgresql user=$ADMIN_USERNAME password=$ADMIN_PASSWORD" \
    GOOSE_MIGRATION_DIR="/etc/patroni/migrations" \
    goose "${extra_flags[@]}" "$command"
}

# Database names
POSTGRES_DB="postgres"
CHAT_DB="chat_db"

# Create the application database
pg_connect ADMIN $POSTGRES_DB "CREATE DATABASE $CHAT_DB WITH OWNER \"$ADMIN_USERNAME\";"

# Initialize the chat_db with the provided SQL script
pg_connect ADMIN $CHAT_DB "/etc/patroni/init.sql"

# Create users and grant necessary permissions
pg_connect ADMIN $CHAT_DB "
    -- Create read-only user
    CREATE USER \"$CHAT_RO_USERNAME\" WITH PASSWORD '$CHAT_RO_PASSWORD';
    GRANT CONNECT ON DATABASE $CHAT_DB TO \"$CHAT_RO_USERNAME\";
    GRANT USAGE ON SCHEMA public TO \"$CHAT_RO_USERNAME\";
    GRANT SELECT ON ALL TABLES IN SCHEMA public TO \"$CHAT_RO_USERNAME\";
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO \"$CHAT_RO_USERNAME\";
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO \"$CHAT_RO_USERNAME\";
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO \"$CHAT_RO_USERNAME\";

    -- Create read-write user
    CREATE USER \"$CHAT_RW_USERNAME\" WITH PASSWORD '$CHAT_RW_PASSWORD';
    GRANT CONNECT ON DATABASE $CHAT_DB TO \"$CHAT_RW_USERNAME\";
    GRANT USAGE ON SCHEMA public TO \"$CHAT_RW_USERNAME\";
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO \"$CHAT_RW_USERNAME\";
    GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO \"$CHAT_RW_USERNAME\";
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO \"$CHAT_RW_USERNAME\";
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO \"$CHAT_RW_USERNAME\";

    -- Create pgmonitor user
    CREATE USER \"$PGMONITOR_USERNAME\" WITH PASSWORD '$PGMONITOR_PASSWORD';
    ALTER USER \"$PGMONITOR_USERNAME\" SET search_path TO aot, public, postgres_exporter, pg_catalog;
    GRANT CONNECT ON DATABASE $POSTGRES_DB TO \"$PGMONITOR_USERNAME\";
    GRANT pg_monitor, pg_read_all_settings, pg_read_all_stats, pg_stat_scan_tables TO \"$PGMONITOR_USERNAME\";

    -- pgpool_health user
    CREATE USER \"$PGPOOL_HEALTH_USERNAME\" WITH PASSWORD '$PGPOOL_HEALTH_PASSWORD';
    GRANT CONNECT ON DATABASE $POSTGRES_DB TO \"$PGPOOL_HEALTH_USERNAME\";
"

# Install required extensions and set up cron job for partition maintenance
pg_connect ADMIN $POSTGRES_DB "
    CREATE EXTENSION IF NOT EXISTS pg_cron;
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

    SELECT cron.schedule_in_database(
        'partman_maintenance',
        '@daily',
        'CALL partman.run_maintenance_proc(p_wait := 0, p_analyze := true);',
        '$CHAT_DB',
        '$ADMIN_USERNAME',
        true
    );

    SELECT cron.schedule_in_database(
        'chatting_device_maintenance',
        '@daily',
        'CALL public.prune_stale_chatting_devices();',
        '$CHAT_DB',
        '$ADMIN_USERNAME',
        true
    );
"

# Apply migrations
goose_c status
goose_c up
goose_c status
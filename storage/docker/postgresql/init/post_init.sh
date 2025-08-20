#!/bin/sh
set -e

APP_PASSWORD=$(cat /run/secrets/db_password)

# Define the connection strings
APPUSER_TO_POSTGRES_DB_CONN="user=appuser host=localhost dbname=postgres password=$APP_PASSWORD"
APPUSER_TO_CHAT_DB_CONN="user=appuser host=localhost dbname=chat_db password=$APP_PASSWORD"

echo "Running post_init.sh script..."

psql -v ON_ERROR_STOP=1 "$APPUSER_TO_POSTGRES_DB_CONN" <<-EOSQL
    CREATE ROLE pgpool_health_check WITH LOGIN PASSWORD '$APP_PASSWORD';
    CREATE DATABASE chat_db WITH OWNER appuser;
EOSQL

psql -v ON_ERROR_STOP=1 "$APPUSER_TO_POSTGRES_DB_CONN" <<-EOSQL
    CREATE EXTENSION IF NOT EXISTS pg_cron;
    CREATE EXTENSION IF NOT EXISTS dblink;
    SELECT cron.schedule(
        'session_partitions_maintenance',
        '0 * * * *',
        \$\$SELECT dblink_exec('dbname=chat_db host=localhost user=appuser password=$APP_PASSWORD', 'SELECT partman.run_maintenance()')\$\$
    );
    ALTER SYSTEM SET cron.database_name = 'postgres';
    SELECT pg_reload_conf();
EOSQL

psql -v ON_ERROR_STOP=1 "$APPUSER_TO_CHAT_DB_CONN" -f /etc/patroni/init.sql

psql -v ON_ERROR_STOP=1 "$APPUSER_TO_CHAT_DB_CONN" <<-EOSQL
    CREATE ROLE app_rw WITH LOGIN PASSWORD '$APP_PASSWORD';
    GRANT CONNECT ON DATABASE chat_db TO app_rw;
    GRANT USAGE ON SCHEMA public TO app_rw;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app_rw;
    GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO app_rw;
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_rw;
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO app_rw;
EOSQL

psql -v ON_ERROR_STOP=1 "$APPUSER_TO_POSTGRES_DB_CONN" <<-EOSQL
    CREATE SCHEMA patroni;
    CREATE EXTENSION IF NOT EXISTS plpython3u;
    CREATE FUNCTION patroni.restapi_query()
    RETURNS TABLE (
        backend_hostname TEXT,
        backend_port INT,
        backend_weight REAL,
        backend_role TEXT,
        backend_state TEXT,
        backend_lag INT
    )
    AS \$\$
        import json
        import urllib.request

        try:
            with urllib.request.urlopen('http://127.0.0.1:8008/cluster') as response:
                data = json.loads(response.read().decode('utf-8'))
                for member in data.get('members', []):
                    role = member.get('role', 'unknown')
                    state = member.get('state', 'unknown')
                    lag = member.get('lag')
                    if not isinstance(lag, int):
                        lag = 0

                    # Map Patroni roles to Pgpool roles
                    pgpool_role = 'primary' if role == 'leader' else 'standby'

                    # Return row for each member
                    yield (member.get('host'), member.get('port'), 1.0, pgpool_role, state, lag)
        except Exception as e:
            # In case of error, return nothing, Pgpool will see no backends
            return
    \$\$ LANGUAGE plpython3u;

    GRANT EXECUTE ON FUNCTION patroni.restapi_query() TO pgpool_health_check;
EOSQL
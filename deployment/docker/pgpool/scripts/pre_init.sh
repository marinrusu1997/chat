#!/bin/bash
set -e

# Regenerate pgpool pool_passwd file from mounted secrets
POOL_PASSWD_FILE="/etc/pgpool2/pool_passwd"
POOL_KEY_FILE="/var/lib/postgresql/.pgpoolkey"

# Truncate or create pool_passwd
: > "$POOL_PASSWD_FILE"

# Loop over all mounted .pass files for db users
for f in /run/secrets/db_external_user_*.pass; do
    [ -e "$f" ] || continue

    # Extract the username from the filename: db_external_user_<username>.pass
    user=$(basename "$f" .pass)
    user=${user#db_external_user_}

    # Add/update user in pool_passwd using SCRAM-SHA-256
    pg_enc --key-file="$POOL_KEY_FILE" --update-pass --username="$user" "$(cat "$f")"
done

# Secure permissions
chown postgres:postgres "$POOL_PASSWD_FILE"
chmod 400 "$POOL_PASSWD_FILE"

# Replace placeholders in the config file
POOL_CONF_TEMPLATE_FILE="/tmp/pgpool.conf.template"
POOL_CONF_FILE="/etc/pgpool2/pgpool.conf"
cp "$POOL_CONF_TEMPLATE_FILE" "$POOL_CONF_FILE"

USER_PG_POOL_HEALTH_PASSWORD=$(cat /run/secrets/db_user_pgpool_health.pass)
sed -i \
    -e "s/{{SR_CHECK_PASSWORD}}/$USER_PG_POOL_HEALTH_PASSWORD/" \
    -e "s/{{HEALTH_CHECK_PASSWORD}}/$USER_PG_POOL_HEALTH_PASSWORD/" \
    "$POOL_CONF_FILE"

# Secure permissions
chmod 0400 "$POOL_CONF_FILE"
chown postgres:postgres "$POOL_CONF_FILE"
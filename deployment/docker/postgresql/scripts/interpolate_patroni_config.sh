#!/usr/bin/env bash
set -euo pipefail

# Paths
TEMPLATE_FILE="/tmp/patroni.yml.template"   # mounted template
TARGET_FILE="/etc/patroni.yml"              # config file used by Patroni

# Copy the template to /etc
cp "$TEMPLATE_FILE" "$TARGET_FILE"

# Read passwords from mounted .path files
SUPERUSER_PASSWORD=$(cat /run/secrets/db_user_admin.pass)
REPLICATION_PASSWORD=$(cat /run/secrets/db_user_replicator.pass)
REWIND_PASSWORD=$(cat /run/secrets/db_user_rewinder.pass)
RESTAPI_PASSWORD=$(cat /run/secrets/patroni_user_admin.pass)

# Replace placeholders in the config file
sed -i \
    -e "s/{{SUPERUSER_PASSWORD_PLACEHOLDER}}/$SUPERUSER_PASSWORD/" \
    -e "s/{{REPLICATION_PASSWORD_PLACEHOLDER}}/$REPLICATION_PASSWORD/" \
    -e "s/{{REWIND_PASSWORD_PLACEHOLDER}}/$REWIND_PASSWORD/" \
    -e "s/{{RESTAPI_PASSWORD_PLACEHOLDER}}/$RESTAPI_PASSWORD/" \
    "$TARGET_FILE"

# Set permissions
chmod 0400 "$TARGET_FILE"
chown postgres:postgres "$TARGET_FILE"
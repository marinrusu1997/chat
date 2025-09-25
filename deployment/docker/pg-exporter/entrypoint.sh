#!/usr/bin/env bash
set -euo pipefail

# Paths
TEMPLATE_FILE="/tmp/postgres_exporter.yml.template"
TARGET_FILE="/etc/postgres_exporter/postgres_exporter.yml"
LOCK_FILE="/etc/postgres_exporter/setup-complete"

if [ ! -f "$LOCK_FILE" ]; then
  # Copy the template to /etc
  mkdir "$(dirname "$TARGET_FILE")"
  cp "$TEMPLATE_FILE" "$TARGET_FILE"

  # Read passwords from mounted .path files
  PG_MONITOR_PASSWORD=$(cat /run/secrets/db_user_pgmonitor.pass)

  # Replace placeholders in the config file
  sed -i \
      -e "s/{{PGMONITOR_PASSWORD_PLACEHOLDER}}/$PG_MONITOR_PASSWORD/" \
      "$TARGET_FILE"

  # Set permissions
  chmod 0444 "$TARGET_FILE"

  # Create lock file
  touch "$LOCK_FILE"
fi

/bin/postgres_exporter --config.file="$TARGET_FILE" \
                       --collector.database_wraparound \
                       --collector.long_running_transactions \
                       --collector.postmaster \
                       --collector.process_idle \
                       --collector.stat_activity_autovacuum \
                       --collector.stat_statements \
                       --collector.stat_wal_receiver \
                       --collector.statio_user_indexes \
                       --log.level=info \
                       --log.format=logfmt
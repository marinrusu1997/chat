#!/bin/bash
set -euo pipefail

echo "Executing $0 with PID $$"

# Set ownership and permissions
chown -R postgres:postgres /var/lib/postgresql/data
chmod 0700 /var/lib/postgresql/data

chown -R postgres:postgres /run/secrets/*.pass
chmod 0400 /run/secrets/*.pass

chown postgres:postgres /etc/patroni/on_role_change.sh
chmod 0500 /etc/patroni/on_role_change.sh

chown postgres:postgres /etc/patroni/on_start.sh
chmod 0500 /etc/patroni/on_start.sh

chown postgres:postgres /etc/patroni/on_stop.sh
chmod 0500 /etc/patroni/on_stop.sh

chown postgres:postgres /etc/patroni/pcp_config.sh
chmod 0500 /etc/patroni/pcp_config.sh

# Setup PCP password file
cp /tmp/configs/pcppass /var/lib/postgresql/.pcppass
chown postgres:postgres /var/lib/postgresql/.pcppass
chmod 0400 /var/lib/postgresql/.pcppass

# Interpolate Patroni config
bash /etc/patroni/interpolate_patroni_config.sh

# Trap signals
trap 'echo "SIGTERM received: stopping Patroni"; gosu postgres patronictl -c /etc/patroni.yml stop $(hostname); kill $PATRONI_PID; wait $PATRONI_PID' SIGTERM SIGINT
echo "Signal trap for 'SIGTERM SIGINT' was set"

# Run Patroni in the foreground (replace shell with Patroni)
exec gosu postgres patroni /etc/patroni.yml
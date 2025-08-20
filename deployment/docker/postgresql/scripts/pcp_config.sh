#!/bin/bash

# Shared configuration for PCP (pgpool) scripts
export PCP_USER="pcpuser"
export PCPPASSFILE="/var/lib/postgresql/.pcppass"
export PCP_HOST="pgpool"
export PCP_PORT="9898"

# Check for presence of Pgpool-II backend node id
if [[ -z "$PGPOOL_BACKEND_NODE_ID" ]]; then
    echo "$(date '+%F %T') ERROR: Pgpool-II backend node id is not set. Please set the PGPOOL_BACKEND_NODE_ID environment variable."
    exit 1
fi
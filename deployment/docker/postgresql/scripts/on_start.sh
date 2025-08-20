#!/bin/bash

echo -e "$(date '+%F %T') ON_START_HOOK: $@"

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source "${SCRIPT_DIR}/pcp_config.sh"

pcp_attach_node -h $PCP_HOST -p $PCP_PORT -U $PCP_USER --no-password --verbose $PGPOOL_BACKEND_NODE_ID
if [[ $? -eq 0 ]]; then
    echo -e "$(date '+%F %T') Attached this node to Pgpool-II with backend ID $PGPOOL_BACKEND_NODE_ID"
else
    echo "$(date '+%F %T') ERROR: Failed to attach this node to Pgpool-II with backend ID $PGPOOL_BACKEND_NODE_ID" >&2
    exit 1
fi



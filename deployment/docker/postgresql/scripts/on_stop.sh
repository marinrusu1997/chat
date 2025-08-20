#!/bin/bash

echo -e "$(date '+%F %T') ON_STOP_HOOK: $@"

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source "${SCRIPT_DIR}/pcp_config.sh"

pcp_detach_node -h $PCP_HOST -p $PCP_PORT -U $PCP_USER -n $PGPOOL_BACKEND_NODE_ID --gracefully --no-password --verbose
if [[ $? -eq 0 ]]; then
    echo -e "$(date '+%F %T') Detached this node from Pgpool-II with backend ID $PGPOOL_BACKEND_NODE_ID"
else
    echo "$(date '+%F %T') ERROR: Failed to detach this node from Pgpool-II with backend ID $PGPOOL_BACKEND_NODE_ID" >&2
    exit 1
fi



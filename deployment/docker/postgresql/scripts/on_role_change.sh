#!/bin/bash

echo -e "$(date '+%F %T') ON_ROLE_CHANGE_HOOK: $@"

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source "${SCRIPT_DIR}/pcp_config.sh"

if [ "$2" == "primary" ]; then
    pcp_promote_node -h $PCP_HOST -p $PCP_PORT -U $PCP_USER --no-password -n $PGPOOL_BACKEND_NODE_ID
    if [[ $? -eq 0 ]]; then
        echo -e "$(date '+%F %T') Informed Pgpool-II that this node is the new primary (backend ID $PGPOOL_BACKEND_NODE_ID)"
    else
        echo "$(date '+%F %T') ERROR: Failed to inform Pgpool-II that this node is the new primary (backend ID $PGPOOL_BACKEND_NODE_ID)" >&2
        exit 1
    fi
fi

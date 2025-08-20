#!/bin/bash

echo -e "$(date '+%F %T') FOLLOW_PRIMARY:\n"\
"  detached_node_id=$1\n"\
"  detached_node_host=$2\n"\
"  detached_node_port=$3\n"\
"  detached_node_data_dir=$4\n"\
"  new_primary_node_id=$5\n"\
"  new_primary_node_host=$6\n"\
"  new_primary_node_port=$7\n"\
"  new_primary_node_data_dir=$8\n"\
"  old_main_node_id=$9\n"\
"  old_primary_node_id=${10}\n"\
"  old_primary_node_host=${11}\n"\
"  old_primary_node_port=${12}\n"\
"---------------------------------------------"

# Configuration
PCP_USER="pcpuser"
PCP_PASSFILE="/var/lib/postgresql/.pcppass"
PCP_HOST="127.0.0.1"
PCP_PORT="9898"
export PCPPASSFILE="$PCP_PASSFILE"

# Script variables
DETACHED_NODE_ID="$1"
OLD_PRIMARY_NODE_ID="${10}"

if [[ -n "$DETACHED_NODE_ID" && -n "$OLD_PRIMARY_NODE_ID" && "$DETACHED_NODE_ID" != "$OLD_PRIMARY_NODE_ID" ]]; then
    pcp_attach_node -h $PCP_HOST -p $PCP_PORT -U $PCP_USER --no-password --verbose $DETACHED_NODE_ID
    if [[ $? -eq 0 ]]; then
        echo -e "$(date '+%F %T') Re-attached node $DETACHED_NODE_ID to Pgpool-II"
    else
        echo "$(date '+%F %T') ERROR: Failed to re-attach node $DETACHED_NODE_ID to Pgpool-II"
        exit 1
    fi
else
    echo "No re-attach is needed. The nodes (detached=$DETACHED_NODE_ID & old_primary=$OLD_PRIMARY_NODE_ID) are the same."
fi

# PCPPASSFILE="/var/lib/postgresql/.pcppass" pcp_node_info -h 127.0.0.1 -p 9898 -U pcpuser --no-password --all --verbose
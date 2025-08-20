#!/bin/bash

echo -e "$(date '+%F %T') FAILBACK:\n"\
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
#!/bin/bash

# turn on bash's job control
set -em

# Start the primary process and put it in the background
/startup/docker-entrypoint.sh neo4j &

# Start the helper process
/startup/setup.sh

# now we bring the primary process back into the foreground
# and leave it there
fg %1
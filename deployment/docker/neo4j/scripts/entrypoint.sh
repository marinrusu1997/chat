#!/bin/bash

# turn on bash's job control
set -em

# Set ownership to neo4j:neo4j
chown -R neo4j:neo4j /var/lib/neo4j/certificates/bolt

# Set directory permissions
chmod 755 /var/lib/neo4j/certificates/bolt
chmod 755 /var/lib/neo4j/certificates/bolt/trusted
chmod 755 /var/lib/neo4j/certificates/bolt/revoked

# Set file permissions
chmod 644 /var/lib/neo4j/certificates/bolt/public.crt
chmod 400 /var/lib/neo4j/certificates/bolt/private.key
chmod 644 /var/lib/neo4j/certificates/bolt/trusted/public.crt

# Set ownership to neo4j:neo4j
chown -R neo4j:neo4j /var/lib/neo4j/certificates/https

# Set directory permissions
chmod 755 /var/lib/neo4j/certificates/https
chmod 755 /var/lib/neo4j/certificates/https/trusted
chmod 755 /var/lib/neo4j/certificates/https/revoked

# Set file permissions
chmod 644 /var/lib/neo4j/certificates/https/public.crt
chmod 400 /var/lib/neo4j/certificates/https/private.key
chmod 644 /var/lib/neo4j/certificates/https/trusted/public.crt

# Start the primary process and put it in the background
/startup/docker-entrypoint.sh neo4j &

# Start the helper process
/startup/setup.sh

# now we bring the primary process back into the foreground
# and leave it there
fg %1

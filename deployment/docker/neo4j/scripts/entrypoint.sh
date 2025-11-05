#!/bin/bash
set -Eeuo pipefail

declare -r SRC_DIR="/tmp/certificates"
declare -r DST_DIR="/var/lib/neo4j/certificates"

if [[ ! -d "$DST_DIR/bolt" ]]; then
	echo "📂 Copying certificates from $SRC_DIR to $DST_DIR..."
	mkdir -p "$DST_DIR"
	cp -a "$SRC_DIR/." "$DST_DIR/"

	echo "👤 Setting ownership to neo4j:neo4j recursively..."
	chown -R neo4j:neo4j "$DST_DIR"

	echo "🔒 Setting directory permissions to 755..."
	find "$DST_DIR" -type d -exec chmod 755 {} \;

	echo "📜 Setting .crt files permissions to 644..."
	find "$DST_DIR" -type f -name "*.crt" -exec chmod 644 {} \;

	echo "🔑 Setting .key files permissions to 400..."
	find "$DST_DIR" -type f -name "*.key" -exec chmod 400 {} \;

	echo "✅ Certificates copied and permissions set successfully."
fi

# Start the primary process and put it in the background
/startup/docker-entrypoint.sh neo4j &

# Start the helper process
/startup/setup.sh

# now we bring the primary process back into the foreground
# and leave it there
fg %1

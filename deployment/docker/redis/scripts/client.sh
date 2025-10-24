#!/usr/bin/env bash

# Function to run redis-cli with TLS/mTLS support
# Usage: redis_cli PASSWORD [extra redis-cli args...]
# Example:
#   redis_cli "myPassword" -h redis-node-1 -p 6380 GET mykey

redis_cli() {
	local password="$1"
	shift

	# Default TLS options
	local tls_flags=(
		--tls
		--cacert "/usr/local/etc/redis/certs/ca/public.crt"
		--cert "/usr/local/etc/redis/certs/public.crt"
		--key "/usr/local/etc/redis/certs/private.key"
		--tls-ciphers "HIGH:!aNULL:!MD5"
	)

	# Run redis-cli with password, TLS flags, and additional arguments
	REDISCLI_AUTH="$password" redis-cli "${tls_flags[@]}" "$@"
}

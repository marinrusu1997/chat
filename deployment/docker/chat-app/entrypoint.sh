#!/usr/bin/env bash
set -Eeuo pipefail

declare -r BIN_PATH="/usr/local/bin/chat"
declare -r BUILD_LOG_PATH="/tmp/go-build.log"

GOEXPERIMENT=greenteagc go build -o /usr/local/bin/chat ./src >"$BUILD_LOG_PATH" 2>&1 || {
	echo "[entrypoint] build failed. Showing log:"
	cat "$BUILD_LOG_PATH"
	exit 1
}

exec "$BIN_PATH"

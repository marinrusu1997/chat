#!/bin/bash

# ==============================================================================
# Simple Bash Logger
#
# This script provides a set of functions for colored logging.
# To use, source this script in your own shell script:
#   source /path/to/logger.sh
#
# Then, call the log functions:
#   log_info "MyModule" "This is an informational message."
#   log_warn "Database" "Connection is getting slow."
#   log_fatal "Main" "Cannot continue."
# ==============================================================================

# --- Color Codes ---
# Using 'tput' is a more portable and robust way to handle colors.
# It checks the terminal's capabilities before printing color codes.
if tput setaf 1 > /dev/null 2>&1; then
  C_BLUE=$(tput setaf 4)
  C_GREEN=$(tput setaf 2)
  C_YELLOW=$(tput setaf 3)
  C_RED=$(tput setaf 1)
  C_MAGENTA=$(tput setaf 5)
  C_RESET=$(tput sgr0)
else
  # Fallback to raw ANSI codes if tput is not available
  C_BLUE='\033[0;34m'
  C_GREEN='\033[0;32m'
  C_YELLOW='\033[0;33m'
  C_RED='\033[0;31m'
  C_MAGENTA='\033[0;35m'
  C_RESET='\033[0m'
fi

# --- Internal Log Function ---
# This is the core function that formats and prints the log message.
# Usage: _log <color> <level_string> <module> <message>
_log() {
  local color="$1"
  local level="$2"
  local module="$3"
  local message="$4"

  # printf is used for its reliable formatting and handling of special characters.
  # %b tells printf to interpret backslash escapes (like our color codes).
  printf "%s %b[%-5s]%b [%s]: %s\n" \
    "$(date '+%F %T')" \
    "$color" \
    "$level" \
    "$C_RESET" \
    "$module" \
    "$message"
}

# --- Public Log Functions ---

# Logs a DEBUG message (blue).
# Usage: log_debug <module> <message>
log_debug() {
  _log "$C_BLUE" "DEBUG" "$1" "$2"
}

# Logs an INFO message (green).
# Usage: log_info <module> <message>
log_info() {
  _log "$C_GREEN" "INFO" "$1" "$2"
}

# Logs a WARN message (yellow).
# Usage: log_warn <module> <message>
log_warn() {
  _log "$C_YELLOW" "WARN" "$1" "$2"
}

# Logs an ERROR message (red).
# Usage: log_error <module> <message>
log_error() {
  # Direct errors to stderr for better practice in shell scripting
  _log "$C_RED" "ERROR" "$1" "$2" >&2
}

# Logs a FATAL message (magenta) and exits the script.
# Usage: log_fatal <module> <message>
log_fatal() {
  _log "$C_MAGENTA" "FATAL" "$1" "$2" >&2
  exit 1
}
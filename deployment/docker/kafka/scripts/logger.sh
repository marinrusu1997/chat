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

# --- Log Level Configuration ---
# Set the default minimum level to INFO. User can override this in the calling script.
: "${CURRENT_LOG_LEVEL:="INFO"}"

# Map log level names to severity numbers (Higher number = Higher severity)
declare -A LOG_LEVELS
LOG_LEVELS=(
	["DEBUG"]=10
	["INFO"]=20
	["WARN"]=30
	["ERROR"]=40
	["FATAL"]=50
)

# Define the codes for common styles, colors, and backgrounds
declare -A ANSI_CODES
ANSI_CODES=(
	# Styles
	[reset]="0"
	[bold]="1"
	[faint]="2"
	[italic]="3"
	[underline]="4"
	[blink]="5"
	[reverse]="7"

	# Foreground Colors
	[black]="30"
	[red]="31"
	[green]="32"
	[yellow]="33"
	[blue]="34"
	[magenta]="35"
	[cyan]="36"
	[white]="37"

	# Background Colors
	[on_black]="40"
	[on_red]="41"
	[on_green]="42"
	[on_yellow]="43"
	[on_blue]="44"
	[on_magenta]="45"
	[on_cyan]="46"
	[on_white]="47"
)

# Function to apply ANSI styles to text
# Usage: format_text "text to format" "style1" "style2" ...
format_text() {
	local text="$1"
	shift

	local codes_list=()
	local style_name

	# Loop through all remaining arguments (the style names)
	for style_name in "$@"; do
		code="${ANSI_CODES[$style_name]}"
		if [[ -n "$code" ]]; then
			codes_list+=("$code")
		fi
	done

	# Join codes with a semicolon (e.g., "1;3;32")
	local code_sequence
	code_sequence=$(
		IFS=';'
		echo "${codes_list[*]}"
	)

	# Print the formatted text: \e[CODE_SEQUENCE_m TEXT \e[0m
	# \e[...m starts the formatting, \e[0m resets it.
	printf "\e[%sm%s\e[0m" "$code_sequence" "$text"
}

# --- Color Codes ---
# Using 'tput' is a more portable and robust way to handle colors.
# It checks the terminal's capabilities before printing color codes.
if tput setaf 1 >/dev/null 2>&1; then
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

# --- Helper Function to Check Log Level ---
# Returns 0 if the message should be logged, 1 otherwise.
# Usage: check_level <message_level_string>
_check_level() {
	local message_level="$1"
	local current_level_numeric="${LOG_LEVELS[$CURRENT_LOG_LEVEL]}"
	local message_level_numeric="${LOG_LEVELS[$message_level]}"

	# Use the Bash arithmetic comparison
	if ((message_level_numeric >= current_level_numeric)); then
		return 0 # Log the message
	else
		return 1 # Skip the message
	fi
}

# --- Internal Log Function ---
# This is the core function that formats and prints the log message.
# Usage: _log <color> <level_string> <module> <message>
_log() {
	local color="$1"
	local level="$2"
	local module="$3"
	local message="$4"

	# Only proceed if the message level meets the CURRENT_LOG_LEVEL threshold
	_check_level "$level" || return 0

	# printf is used for its reliable formatting and handling of special characters.
	# %b tells printf to interpret backslash escapes (like our color codes).
	printf "%s [%-15s %-4s] %b[%-5s]%b [%-20s]: %s\n" \
		"$(date '+%F %T')" \
		"${BASH_SOURCE[2]##*/}" \
		"${BASH_LINENO[2]}" \
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

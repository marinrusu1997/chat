#!/usr/bin/env bash

# kafka-healthcheck.sh
# Usage: BOOTSTRAP_SERVERS="kafka1:9092,kafka2:9092,kafka3:9092" KAFKA_BIN=/opt/kafka/bin ./kafka-healthcheck.sh
set -Eeuo pipefail

# Import dependencies
. /opt/kafka/scripts/logger.sh

# Cleanup
declare -a __TRAP_HANDLERS

register_trap_handler() {
	local -r cmd_name="$1"

	if [ -z "$cmd_name" ]; then
		log_fatal "${FUNCNAME[0]}" "command is required"
	fi

	local -r regex_pattern=" ${cmd_name} "
	if [[ " ${__TRAP_HANDLERS[*]} " =~ $regex_pattern ]]; then
		log_fatal "${FUNCNAME[0]}" "Handler '$cmd_name' is already registered."
	else
		__TRAP_HANDLERS+=("$cmd_name")
		log_debug "${FUNCNAME[0]}" "Registered handler: $cmd_name"
	fi
}

# shellcheck disable=SC2034
__main_trap_handler() {
	for _cmd in "${__TRAP_HANDLERS[@]}"; do
		if ! eval "$_cmd"; then
			log_error "${FUNCNAME[0]}" "❌  Command '$_cmd' failed with exit code $?."
		fi
	done
}
trap __main_trap_handler EXIT INT TERM HUP

# Helper functions
get_script_full_path() {
	# Prefer BASH_SOURCE when available (handles `source` and executed scripts)
	local src="${BASH_SOURCE[0]:-$0}"

	# If it's a relative path or symlink, resolve iteratively
	# This avoids relying on readlink -f which is not portable to macOS
	while [ -L "$src" ]; do
		local -r target="$(readlink "$src")" || break
		if [[ "$target" = /* ]]; then
			src="$target"
		else
			src="$(dirname "$src")/$target"
		fi
	done

	# Convert to absolute (pwd -P resolves parent symlinks)
	local -r dir="$(cd -P "$(dirname -- "$src")" >/dev/null 2>&1 && pwd)"
	printf '%s/%s' "$dir" "$(basename -- "$src")"
}

sha256_of_string() {
	local -r s="$1"

	# Prefer sha256sum
	if command -v sha256sum >/dev/null 2>&1; then
		printf '%s' "$s" | sha256sum | awk '{print $1}'
		return
	fi

	# macOS: shasum -a 256
	if command -v shasum >/dev/null 2>&1; then
		printf '%s' "$s" | shasum -a 256 | awk '{print $1}'
		return
	fi

	# openssl fallback
	if command -v openssl >/dev/null 2>&1; then
		printf '%s' "$s" | openssl dgst -sha256 | awk '{print $NF}'
		return
	fi

	# python3 fallback (binary-safe)
	if command -v python3 >/dev/null 2>&1; then
		python3 - -- "$s" <<'PY'
import sys,hashlib
data = sys.argv[1].encode()
print(hashlib.sha256(data).hexdigest())
PY
		return
	fi

	log_fatal "${FUNCNAME[0]}" "❌  no sha256 tool found (install sha256sum/shasum/openssl/python3)"
}

clean_dirs() {
	for dir in "$@"; do
		if [ -d "$dir" ]; then
			rm -rf "${dir:?directory is required}"/* "$dir"/.* 2>/dev/null
		fi
	done
}

clean_files_csv() {
	local -r files_csv="$1"
	local -a files_to_delete=()

	# Read the CSV, replace commas with newlines, and read into a temporary array
	local -a temp_arr
	readarray -t temp_arr <<<"$(echo "$files_csv" | tr ',' '\n')"
	declare -ra temp_arr

	# Process the temporary array to handle potential whitespace and empty entries
	for file_path in "${temp_arr[@]}"; do
		# Trim whitespace from the element
		file_path=$(echo "$file_path" | xargs)

		# Only add non-empty, non-whitespace strings to the final array
		if [[ -n "$file_path" ]]; then
			files_to_delete+=("$file_path")
		fi
	done

	if ((${#files_to_delete[@]} > 0)); then
		if ! rm -rf "${files_to_delete[@]}"; then
			log_error "${FUNCNAME[0]}" "❌  Failed to delete files: ${files_to_delete[*]}"
		fi
	fi
}

compare_arrays() {
	local -rn expected_ref="$1"
	local -rn actual_ref="$2"
	local -r context="${3:-FUNCNAME[0]}"

	# compare lengths first
	if ((${#expected_ref[@]} != ${#actual_ref[@]})); then
		log_fatal "$context" "❌  Count mismatch. Got: ${#actual_ref[@]}, Expected: ${#expected_ref[@]}. Expected array: (${expected_ref[*]}), Actual array: (${actual_ref[*]})"
	fi

	# compare element by element
	for i in "${!expected_ref[@]}"; do
		if [[ "${expected_ref[i]}" != "${actual_ref[i]}" ]]; then
			log_fatal "$context" "❌  Element mismatch at index $i. Got: '${actual_ref[i]}', Expected: '${expected_ref[i]}'. Expected array: (${expected_ref[*]}), Actual array: (${actual_ref[*]})"
		fi
	done
}

delete_k_topics() {
	local -r topic_pattern="$1"
	if [ -z "$topic_pattern" ]; then
		log_fatal "${FUNCNAME[0]}" "❌  Deletion pattern must be provided as the first argument."
	fi

	local -r all_topics_output=$(kafka_exec kafka-topics --list 2>&1) ||
		{ log_fatal "${FUNCNAME[0]}" "❌  Failed to list topics for further deletion. Output:\n$all_topics_output"; }

	local -a all_topic_names
	readarray -t all_topic_names <<<"$all_topics_output"
	declare -ra all_topic_names

	local -a topics_to_delete=()
	for topic in "${all_topic_names[@]}"; do
		if [[ "$topic" =~ $topic_pattern ]]; then
			topics_to_delete+=("$topic")
		fi
	done

	if [ ${#topics_to_delete[@]} -eq 0 ]; then
		log_debug "${FUNCNAME[0]}" "No topics matching pattern '$topic_pattern' found to delete."
		return 0
	fi

	local topic_list_string
	topic_list_string="$(printf ",%s" "${topics_to_delete[@]}")"
	topic_list_string="${topic_list_string#,}" # Remove leading comma

	log_info "${FUNCNAME[0]}" "🗑️  Deleting topics '$topic_list_string' ..."
	local -r delete_output=$(kafka_exec kafka-topics --delete --topic "$topic_list_string" --if-exists 2>&1) ||
		{ log_fatal "${FUNCNAME[0]}" "❌  Failed to delete topics '$topic_list_string'. Output:\n$delete_output"; }
}

parse_csv_hosts() {
	local -r input="$1"
	local -a parsed
	readarray -t parsed < <(tr ',' '\n' <<<"$input" | sort)
	declare -ra parsed

	if ((${#parsed[@]} == 0)); then
		log_fatal "${FUNCNAME[0]}" "Input argument contains no hosts: $input"
	fi

	echo "${parsed[@]}"
}

# replace_host_parts: replace protocol, hostname, and/or port
# Usage:
#   replace_host_parts <host_string> [--protocol=PROT] [--host=HOST] [--port=PORT]
replace_host_parts() {
	local -r input="$1"
	shift

	# Parse optional flags
	local new_protocol="" new_host="" new_port=""
	for arg in "$@"; do
		case $arg in
		--protocol=*) new_protocol="${arg#*=}" ;;
		--host=*) new_host="${arg#*=}" ;;
		--port=*) new_port="${arg#*=}" ;;
		*) log_fatal "${FUNCNAME[0]}" "Unknown argument: $arg" ;;
		esac
	done

	local protocol="" host_port="" host="" port=""

	# Split protocol
	if [[ "$input" =~ ^([a-zA-Z0-9]+)://(.+)$ ]]; then
		protocol="${BASH_REMATCH[1]}"
		host_port="${BASH_REMATCH[2]}"
	else
		host_port="$input"
	fi

	# Split host and port
	if [[ "$host_port" =~ ^([^:]+):([0-9]+)$ ]]; then
		host="${BASH_REMATCH[1]}"
		port="${BASH_REMATCH[2]}"
	else
		host="$host_port"
	fi

	# Apply replacements only if flags were provided
	[[ -n "$new_protocol" ]] && protocol="$new_protocol"
	[[ -n "$new_host" ]] && host="$new_host"
	[[ -n "$new_port" ]] && port="$new_port"

	# Rebuild string
	local result=""
	[[ -n "$protocol" ]] && result+="$protocol://"
	result+="$host"
	[[ -n "$port" ]] && result+=":$port"

	echo "$result"
}

get_normalized_endpoints() {
	local list=""
	local default_list=""
	local delimiter=","
	local protocol=""
	local port=""

	# Parse flag-style args
	for arg in "$@"; do
		case "$arg" in
		--list=*) list="${arg#*=}" ;;
		--default=*) default_list="${arg#*=}" ;;
		--delimiter=*) delimiter="${arg#*=}" ;;
		--protocol=*) protocol="${arg#*=}" ;;
		--port=*) port="${arg#*=}" ;;
		*) log_fatal "${FUNCNAME[0]}" "Unknown argument: $arg" ;;
		esac
	done

	local input=""
	if [[ -n "$list" ]]; then
		input="$list"
	elif [[ -n "$default_list" ]]; then
		input="$default_list"
	else
		log_fatal "${FUNCNAME[0]}" "No endpoints provided via '--list' or '--default'"
	fi

	local -a raw_hosts
	readarray -t raw_hosts < <(echo "$input" | tr "$delimiter" '\n')
	declare -ra raw_hosts

	local -a endpoints=()
	for host in "${raw_hosts[@]}"; do
		local args=("$host")
		[[ -n "$protocol" ]] && args+=(--protocol="$protocol")
		[[ -n "$port" ]] && args+=(--port="$port")

		endpoints+=("$(replace_host_parts "${args[@]}")")
	done

	readarray -t endpoints < <(printf "%s\n" "${endpoints[@]}" | sort)
	if ((${#endpoints[@]} == 0)); then
		log_fatal "${FUNCNAME[0]}" "Input argument contains no hosts: $input"
	fi

	echo "${endpoints[@]}"
}

get_jmx_readonly_credentials() {
	local -r jmx_password_file="$1"
	local -r jmx_access_file="$2"
	local -r readonly_role="readonly"
	local jmx_username
	local jmx_password

	if [ -z "$jmx_password_file" ] || [ -z "$jmx_access_file" ]; then
		log_fatal "${FUNCNAME[0]}" "Missing path arguments. Usage: ${FUNCNAME[0]} <password_file> <access_file>"
	fi
	if [ ! -f "$jmx_password_file" ] || [ ! -f "$jmx_access_file" ]; then
		log_fatal "${FUNCNAME[0]}" "JMX config files not found: $jmx_password_file and/or $jmx_access_file"
	fi

	jmx_username=$(grep -E "[[:space:]]$readonly_role$" "$jmx_access_file" | head -n 1 | awk '{print $1}')
	if [ -z "$jmx_username" ]; then
		log_fatal "${FUNCNAME[0]}" "No user with '$readonly_role' role found in $jmx_access_file."
	fi

	jmx_password=$(grep "^${jmx_username}[[:space:]]" "$jmx_password_file" | awk '{print $2}')
	if [ -z "$jmx_password" ]; then
		log_fatal "${FUNCNAME[0]}" "Password for user '$jmx_username' not found in $jmx_password_file."
	fi

	echo "$jmx_username $jmx_password"
}

get_jmx_metric() {
	local -r bean=$1
	local -r domain=$2
	local -r attribute=$3

	local output
	output=$(
		echo "get -b ${bean} -d ${domain} -n -s -q ${attribute}" |
			java \
				-Djavax.net.ssl.trustStore="$CFG_JMX_TRUSTSTORE_PATH" \
				-Djavax.net.ssl.trustStorePassword="$CFG_JMX_TRUSTSTORE_PASSWORD" \
				-jar "$CFG_JMX_TERM_JAR_PATH" \
				-u "$CFG_JMX_USERNAME" \
				-p "$CFG_JMX_PASSWORD" \
				-l "$CFG_JMX_HOST" \
				-n \
				-e \
				-s \
				-v silent
	)

	output=$(echo "$output" | tr -d '\r' | tr -d '\n' | xargs)
	if [[ -z "$output" ]]; then
		log_fatal "${FUNCNAME[0]}" "Couldn't extract '${attribute}' from JMX bean '${bean}' with domain '${domain}'"
	fi

	echo "$output"
}

declare -rA KAFKA_EXEC_CONFIG_FLAGS=(
	["kafka-topics"]="--command-config"
	["kafka-configs"]="--command-config"
	["kafka-metadata-quorum"]="--command-config"
	["kafka-broker-api-versions"]="--command-config"
	["kafka-acls"]="--command-config"

	["kafka-console-consumer"]="--consumer.config"
	["kafka-consumer-groups"]="--command-config"
	["kafka-verifiable-consumer"]="--consumer.config"

	["kafka-console-producer"]="--producer.config"
	["kafka-verifiable-producer"]="--producer.config"
)

# Function to execute Kafka CLI commands transparently
# Usage: kafka_exec <command_name> [options...]
# Example: kafka_exec kafka-topics --bootstrap-server "$host" --list
kafka_exec() {
	local -rA kafka_generic_defaults=(
		["--bootstrap-server"]="${CFG_BOOTSTRAP_SERVER}"
	)
	local -rA kafka_cmd_specific_defaults=(
		["--command-config"]="${CFG_KAFKA_CLIENT_PROPERTIES_FILE}"
		["--consumer.config"]="${CFG_KAFKA_CLIENT_PROPERTIES_FILE}"
		["--producer.config"]="${CFG_KAFKA_CLIENT_PROPERTIES_FILE}"
	)

	local -r command_name="$1"
	shift # Remove the command name from the arguments list

	local -r full_command="${CFG_KAFKA_BIN}/${command_name}.sh"
	local -ra original_args=("$@") # Capture all remaining arguments
	local -a final_args=()

	local exit_code=0 # Variable to hold the command's exit status

	local -a cmd_specific_flags=()
	if [[ -z "${KAFKA_EXEC_CONFIG_FLAGS[$command_name]:+exists}" ]]; then
		log_fatal "${FUNCNAME[0]}" "Unknown command name: $command_name"
	fi
	cmd_specific_flags+=("${KAFKA_EXEC_CONFIG_FLAGS[$command_name]}")

	local -A provided_flags=()
	for arg in "${original_args[@]}"; do
		if [[ "$arg" =~ ^--[^[:space:]=]* ]]; then
			local flag_name="${arg%%=*}" # Extract the flag name (e.g., --bootstrap-server)
			provided_flags["$flag_name"]=1
		fi
	done

	_append_if_missing() {
		local -r flag="$1"
		local -rn defaults_map="$2"

		if [[ -z "${provided_flags[$flag]:-}" ]]; then
			local -r default_value="${defaults_map[$flag]}"
			if [[ -n "$default_value" ]]; then
				final_args+=("$flag" "$default_value")
			fi
		fi
	}
	for default_flag in "${!kafka_generic_defaults[@]}"; do
		_append_if_missing "$default_flag" kafka_generic_defaults
	done
	for flag_name in "${cmd_specific_flags[@]}"; do
		_append_if_missing "$flag_name" kafka_cmd_specific_defaults
	done

	final_args+=("${original_args[@]}")

	"$full_command" "${final_args[@]}"
	exit_code=$? # Capture the exit code immediately

	return $exit_code
}

# Configuration
declare -r CFG_SCRIPT_HASH="$(sha256_of_string "$(get_script_full_path)")"

declare -r CFG_CONTROLLER_PORT="${CONTROLLER_PORT:-9094}"
declare -r CFG_CONTROLLER_PROTOCOL="${CONTROLLER_PROTOCOL:-CONTROLLER}"

declare -r CFG_BOOTSTRAP_SERVER="${BOOTSTRAP_SERVER:-$(hostname):9091}"

declare -r CFG_KAFKA_BIN="${KAFKA_BIN:-/opt/kafka/bin}"
declare -r CFG_KAFKA_CLIENT_PROPERTIES_FILE="${KAFKA_CLIENT_PROPERTIES_FILE:-/opt/kafka/config/client.properties}"

declare -r CFG_JMX_HOST="${JMX_HOST:-$(hostname):2020}"
declare -r CFG_JMX_TERM_JAR_PATH="${JMX_TERM_JAR_PATH:-/opt/jmxterm/jmxterm.jar}"
declare -r CFG_JMX_TRUSTSTORE_PATH="${JMX_TRUSTSTORE_PATH:-/opt/kafka/config/certs/kafka.truststore.jks}"
declare -r CFG_JMX_TRUSTSTORE_PASSWORD="${JMX_TRUSTSTORE_PASSWORD:?Environment variable JMX_TRUSTSTORE_PASSWORD must be set}"
declare -r CFG_JMX_PASSWORD_FILE="${JMX_PASSWORD_FILE:-/opt/kafka/config/secrets/jmxremote.password}"
declare -r CFG_JMX_ACCESS_FILE="${JMX_ACCESS_FILE:-/opt/kafka/config/secrets/jmxremote.access}"
CFG_JMX_USERNAME=
CFG_JMX_PASSWORD=
if read -r CFG_JMX_USERNAME CFG_JMX_PASSWORD <<<"$(get_jmx_readonly_credentials "$CFG_JMX_PASSWORD_FILE" "$CFG_JMX_ACCESS_FILE")"; then
	if [ -z "$CFG_JMX_USERNAME" ] || [ -z "$CFG_JMX_PASSWORD" ]; then
		log_fatal "JMXCredentials" "❌  JMX username or password is empty"
	fi
else
	log_fatal "JMXCredentials" "❌  Failed to retrieve JMX credentials"
fi
declare -r CFG_JMX_USERNAME
declare -r CFG_JMX_PASSWORD

# Expectations
read -ra EXPECTED_VOTER_ENDPOINTS <<<"$(
	get_normalized_endpoints \
		"--list=kafka1,kafka2,kafka3" \
		"--protocol=$CFG_CONTROLLER_PROTOCOL" \
		"--port=$CFG_CONTROLLER_PORT"
)"
declare -ra EXPECTED_VOTER_ENDPOINTS
declare -r EXPECTED_CLUSTER_ID="${EXPECTED_CLUSTER_ID:-9fnudAZ8ROCSiRomvzgeWg}"
declare -r EXPECTED_QUORUM_MAX_LAG_THRESHOLD="${EXPECTED_QUORUM_MAX_LAG_THRESHOLD:-100}"
declare -r EXPECTED_QUORUM_MAX_LAG_TIME_THRESHOLD="${EXPECTED_QUORUM_MAX_LAG_TIME_THRESHOLD:-5000}"
declare -r EXPECTED_TOPIC_MIN_PARTITIONS="${EXPECTED_TOPIC_MIN_PARTITIONS:-2}"
declare -r EXPECTED_TOPIC_MIN_REPLICATION_FACTOR="${EXPECTED_TOPIC_MIN_REPLICATION_FACTOR:-${#EXPECTED_VOTER_ENDPOINTS[@]}}"
declare -r EXPECTED_CONSUMER_GROUP_MAX_LAG="${EXPECTED_CONSUMER_GROUP_MAX_LAG:-100}"

# Unset env variables that might interfere with Kafka commands
unset KAFKA_JMX_OPTS

# 1️⃣ Metadata quorum status (KRaft)
check_quorum() {
	local -r quorum_raw=$(kafka_exec kafka-metadata-quorum describe --status 2>&1) || {
		log_fatal "BrokerQuorum" "❌  Failed to run kafka-metadata-quorum.sh. Output: $quorum_raw"
	}

	local -r cluster_id=$(echo "$quorum_raw" | awk '/ClusterId:/ {print $2}')
	local -r leader_id=$(echo "$quorum_raw" | awk '/LeaderId:/ {print $2}')
	local -r leader_epoch=$(echo "$quorum_raw" | awk '/LeaderEpoch:/ {print $2}')
	local -r high_watermark=$(echo "$quorum_raw" | awk '/HighWatermark:/ {print $2}')
	local -r max_follower_lag=$(echo "$quorum_raw" | awk '/MaxFollowerLag:/ {print $2}')
	local -r max_follower_lag_time=$(echo "$quorum_raw" | awk '/MaxFollowerLagTimeMs:/ {print $2}')

	local -r current_voters_line=$(echo "$quorum_raw" | grep 'CurrentVoters')
	local -r current_observers_line=$(echo "$quorum_raw" | grep 'CurrentObservers')

	local -a actual_voter_endpoints
	readarray -t actual_voter_endpoints < <(echo "$current_voters_line" | grep -o 'CONTROLLER://[^"]*' | sort)
	declare -ra actual_voter_endpoints

	compare_arrays EXPECTED_VOTER_ENDPOINTS actual_voter_endpoints "BrokerVotersQuorum"

	if [[ -n "$EXPECTED_CLUSTER_ID" && "$cluster_id" != "$EXPECTED_CLUSTER_ID" ]]; then
		log_fatal "BrokerQuorum" "❌  ClusterId mismatch: got $cluster_id, expected $EXPECTED_CLUSTER_ID"
	fi
	if [[ "$leader_id" == "-1" || -z "$leader_id" ]]; then
		log_fatal "BrokerQuorum" "❌  No valid LeaderId found"
	fi
	if [[ -z "$leader_epoch" || "$leader_epoch" -lt 0 ]]; then
		log_fatal "BrokerQuorum" "❌  Invalid LeaderEpoch: $leader_epoch"
	fi
	if [[ -z "$high_watermark" || "$high_watermark" -lt 0 ]]; then
		log_fatal "BrokerQuorum" "❌  Invalid HighWatermark: $high_watermark"
	fi
	if ((max_follower_lag > EXPECTED_QUORUM_MAX_LAG_THRESHOLD)); then
		log_fatal "BrokerQuorum" "❌  MaxFollowerLag too high: $max_follower_lag (threshold $EXPECTED_QUORUM_MAX_LAG_THRESHOLD)"
	fi
	if ((max_follower_lag_time > EXPECTED_QUORUM_MAX_LAG_TIME_THRESHOLD)); then
		log_fatal "BrokerQuorum" "❌  MaxFollowerLagTimeMs too high: $max_follower_lag_time (threshold $EXPECTED_QUORUM_MAX_LAG_TIME_THRESHOLD)"
	fi
	if echo "$current_observers_line" | grep -q '\[.\+\]'; then
		log_fatal "BrokerQuorum" "❌  Unexpected CurrentObservers present: $current_observers_line"
	fi

	log_info "BrokerQuorum" "$(format_text "Quorum is healthy" "bold" "italic" "green"): cluster_id=$cluster_id leader=$leader_id voters=${actual_voter_endpoints[*]}"
}
check_quorum

# 2️⃣ Topics / partitions checks
check_kafka_topic_integrity() {
	# Input args
	local -r topic="$1"
	local -ri min_partitions="$2"
	local -ri min_replication_factor="$3"
	[[ -z "$topic" ]] && log_fatal "${FUNCNAME[0]}" "❌  No input argument for 'topic'"
	[[ -z "$min_partitions" ]] && log_fatal "${FUNCNAME[0]}" "❌  No input argument for 'min_partitions' for topic '$topic'"
	[[ -z "$min_replication_factor" ]] && log_fatal "${FUNCNAME[0]}" "❌  No input argument for 'min_replication_factor' for topic '$topic'"

	# Get topic description
	local -r desc=$(kafka_exec kafka-topics --describe --topic "$topic" 2>&1) ||
		{ log_fatal "${FUNCNAME[0]}" "❌  Failed to describe topic '$topic'. Output: $desc"; }
	[[ -z "$desc" ]] && log_fatal "${FUNCNAME[0]}" "❌  No description output for topic '$topic'"

	# Extract overall topic parameters
	local -i partition_count replication_factor

	if [[ $desc =~ PartitionCount:[[:space:]]*([0-9]+) ]]; then
		partition_count="${BASH_REMATCH[1]}"
	fi
	[[ -z "$partition_count" ]] && log_fatal "${FUNCNAME[0]}" "❌  Couldn't extract 'PartitionCount' from description of topic '$topic'"

	if [[ $desc =~ ReplicationFactor:[[:space:]]*([0-9]+) ]]; then
		replication_factor="${BASH_REMATCH[1]}"
	fi
	[[ -z "$replication_factor" ]] && log_fatal "${FUNCNAME[0]}" "❌  Couldn't extract 'ReplicationFactor' from description of topic '$topic'"

	declare -ri partition_count replication_factor

	# --- Collect errors ---
	local -a errors=()

	# Validate overall topic-level parameters
	if ((partition_count < min_partitions)); then
		errors+=("❌  Topic '$topic' has too few partitions: $partition_count (min $min_partitions)")
	fi
	if ((replication_factor < min_replication_factor)); then
		errors+=("❌  Topic '$topic' has too low replication factor: $replication_factor (min $min_replication_factor)")
	fi

	# Gather partition lines
	local -a partition_lines=()
	readarray -t partition_lines < <(grep -E 'Partition:[[:space:]]*[0-9]+' <<<"$desc")
	declare -ra partition_lines

	if ((${#partition_lines[@]} != partition_count)); then
		errors+=("❌  Topic '$topic' has a partition count mismatch: expected $partition_count, found ${#partition_lines[@]}")
	fi

	# Track partitions found for sequence validation
	local -a partitions_found=()

	for line in "${partition_lines[@]}"; do
		if [[ $line =~ Partition:[[:space:]]*([0-9]+).*Leader:[[:space:]]*([0-9-]+).*Replicas:[[:space:]]*([0-9,]+).*Isr:[[:space:]]*([0-9,]*) ]]; then
			local -i partition="${BASH_REMATCH[1]}"
			local leader="${BASH_REMATCH[2]}"

			partitions_found+=("$partition")

			local -a replicas isr
			readarray -t replicas <<<"$(echo "${BASH_REMATCH[3]}" | tr ',' '\n')"
			readarray -t isr <<<"$(echo "${BASH_REMATCH[4]}" | tr ',' '\n')"

			# Check leader existence
			if [[ "$leader" -eq -1 ]]; then
				errors+=("❌  Topic '$topic' has a partition $partition with no leader!")
			fi

			# Check replica count
			if ((${#replicas[@]} != replication_factor)); then
				errors+=("❌  Topic '$topic' has a partition $partition with wrong replica count: ${#replicas[@]} (expected $replication_factor)")
			fi

			# Check under-replication
			if ((${#isr[@]} < ${#replicas[@]})); then
				errors+=("❌  Topic '$topic' has a partition $partition which is under-replicated: ISR ${#isr[@]} < Replicas ${#replicas[@]}")
			fi
		else
			errors+=("❌  Topic '$topic'. Failed to parse line: $line")
		fi
	done

	# --- Check for non-sequential partitions ---
	if ((${#partitions_found[@]} > 0)); then
		readarray -t partitions_found < <(printf "%s\n" "${partitions_found[@]}" | sort -n)

		local -i expected=0
		for p in "${partitions_found[@]}"; do
			if ((p != expected)); then
				errors+=("❌  Topic '$topic' has a missing or out-of-sequence partition: expected $expected, found $p")
				expected=$((p + 1))
			else
				expected+=1
			fi
		done
	fi

	# --- Report summary ---
	if ((${#errors[@]} > 0)); then
		log_fatal "TopicCheck" $"❌  Topic '$topic' has the following errors:$(printf "\n ")$(printf "\t- %s\n" "${errors[@]}")"
	else
		log_info "TopicCheck" "$(format_text "Topic is healthy" "bold" "italic" "green") '$topic': $partition_count partitions, replication factor $replication_factor"
	fi
}

# 3️⃣ Consumer groups checks
check_consumer_group() {
	local group="$1"
	local topic="$2"
	local max_lag="$3"
	[[ -z "$group" ]] && log_fatal "${FUNCNAME[0]}" "❌  No input argument for 'group'"
	[[ -z "$topic" ]] && log_fatal "${FUNCNAME[0]}" "❌  No input argument for 'topic'"
	[[ -z "$max_lag" ]] && log_fatal "${FUNCNAME[0]}" "❌  No input argument for 'max_lag'"

	# Run kafka-consumer-groups.sh for the group
	local output
	output=$(kafka_exec kafka-consumer-groups --group "$group" --describe 2>&1) ||
		{ log_fatal "${FUNCNAME[0]}" "❌  Failed to describe consumer group '$group'. Output: $output"; }
	[[ -z "$output" ]] && log_fatal "${FUNCNAME[0]}" "❌  No description output for consumer group '$group'"

	# Handle no active members
	if grep -q "has no active members" <<<"$output"; then
		log_warn "ConsumerGroup" "⚠️  Consumer group '$group' has no active members"
		return 0
	fi

	# Array to accumulate errors
	local errors=()

	# Parse the table output
	while read -r line; do
		# Skip header or empty lines
		[[ "$line" =~ ^GROUP ]] && continue
		[[ -z "$line" ]] && continue

		# Fields: GROUP TOPIC PARTITION CURRENT-OFFSET LOG-END-OFFSET LAG CONSUMER-ID HOST CLIENT-ID
		read -r _ t p curr _ lag _ _ _ <<<"$line"

		# Skip lines for other topics if somehow present
		[[ "$t" != "$topic" ]] && continue

		# Check for uncommitted offset
		if [[ "$curr" == "-" ]]; then
			errors+=("❌  Partition $p has no committed offset")
			continue
		fi

		# Check lag threshold
		if ((lag > max_lag)); then
			errors+=("❌  Partition $p lag is $lag (max allowed $max_lag)")
		fi

		# Optional: check negative lag (should not happen)
		if ((lag < 0)); then
			errors+=("❌  Partition $p has negative lag $lag")
		fi
	done <<<"$output"

	# Log fatal if any errors
	if ((${#errors[@]} > 0)); then
		log_fatal "ConsumerGroup" "$(printf "❌  Consumer group '$group' for topic '$topic' is unhealthy:\n %s\n" "${errors[@]}")"
	else
		log_info "ConsumerGroup" "✅  Consumer group '$group' is healthy for topic '$topic'"
	fi
}

# 4️⃣ End-to-end produce/consume smoke test
e2e_smoke_test() {
	_e2e_smoke_test_impl() {
		# Generate unique names
		local -r smoke_prefix="e2e-smoke-test"
		local -r topic_pattern="^${smoke_prefix}-TOPIC-[0-9]+-[0-9]+$"
		local -r smoke_topic="$smoke_prefix-TOPIC-$(date +%s)-$RANDOM"
		local -r msg_key="$smoke_prefix-MSG-KEY-$RANDOM"
		local -r msg_value="$smoke_prefix-MSG-VALUE-$RANDOM"
		local -r msg_header="source=$smoke_prefix-HEADER-VALUE"
		local -r consumer_group="$smoke_prefix-CONSUMER-GROUP-$(date +%s)-$RANDOM"
		local -r consumption_dir="/tmp/$smoke_prefix/consumer"
		local -r consumption_log="$consumption_dir/${smoke_topic}.out"
		local -r consumption_err="$consumption_dir/${smoke_topic}.err"

		# Cleanup any old artifacts
		clean_dirs "$consumption_dir"
		mkdir -p "$consumption_dir"
		delete_k_topics "$topic_pattern"

		# Create topic
		local -r topic_creation_output=$(kafka_exec kafka-topics --create --topic "$smoke_topic" --if-not-exists) ||
			{ log_fatal "E2ESmokeTest" "❌  Failed to create smoke topic '$smoke_topic': $topic_creation_output"; }
		log_debug "E2ESmokeTest" "Smoke topic '$smoke_topic' created"
		check_kafka_topic_integrity "$smoke_topic" "$EXPECTED_TOPIC_MIN_PARTITIONS" "$EXPECTED_TOPIC_MIN_REPLICATION_FACTOR"

		# shellcheck disable=SC2317
		cleanup_e2e_smoke() {
			local -r files_csv="$1"
			local -r topics_csv="$2"

			clean_files_csv "$files_csv"
			if ! kafka_exec kafka-topics --delete --topic "$topics_csv" --if-exists; then
				log_warn "${FUNCNAME[0]}" " Failed to deletee smoke topic '$topics_csv' via cleanup function."
			fi

			log_debug "${FUNCNAME[0]}" "✅  Deleted temporary files '$files_csv' and topics '$topics_csv' via cleanup function."
		}
		# shellcheck disable=SC2064
		trap "cleanup_e2e_smoke '$consumption_log,$consumption_err' '$smoke_topic'" EXIT INT TERM HUP

		# Produce message
		echo "$msg_key:$msg_value,$msg_header" | kafka_exec kafka-console-producer \
			--topic "$smoke_topic" \
			--compression-codec snappy \
			--max-block-ms 5000 \
			--message-send-max-retries 2 \
			--sync \
			--producer-property acks=all \
			--property parse.key=true \
			--property key.separator=: \
			--property headers.delimiter=, \
			--property headers.separator==
		log_debug "E2ESmokeTest" "Message produced to '$smoke_topic'"

		# Consume message
		kafka_exec kafka-console-consumer \
			--topic "$smoke_topic" \
			--group "$consumer_group" \
			--from-beginning \
			--max-messages 1 \
			--timeout-ms 5000 >"$consumption_log" 2>"$consumption_err" ||
			{
				local -r consumption_output="$(cat "$consumption_log")"
				local -r error_output="$(cat "$consumption_err")"
				log_fatal "E2ESmokeTest" "❌  Failed to consume from smoke topic '$smoke_topic'. Consumption: $consumption_output Error: $error_output"
			}

		check_consumer_group "$consumer_group" "$smoke_topic" "$EXPECTED_CONSUMER_GROUP_MAX_LAG"
		if ! grep -q "$msg_value" "$consumption_log"; then
			log_fatal "E2ESmokeTest" "❌  Failed to consume from smoke topic '$smoke_topic'. Consumption: $(cat "$consumption_log") Error: $(cat "$consumption_err")"
		fi

		# All good
		log_info "E2ESmokeTest" "$(format_text "E2E produce/consume successful" "bold" "italic" "green") from smoke topic '$smoke_topic'"
	}

	(
		log_debug "E2ESmokeTest" "Waiting to acquire lock..."
		flock 98 || log_fatal "E2ESmokeTest" "❌  Failed to acquire lock."
		_e2e_smoke_test_impl
	) 98>"/tmp/$CFG_SCRIPT_HASH-${FUNCNAME[0]}-lock"
}

# 5️⃣ JMX checks
check_kafka_kraft_health() {
	log_debug "KafkaKRaftHealth" "Checking Kafka KRaft JMX metrics at '${CFG_JMX_HOST}'"

	local -r numeric_regex='^[0-9]+(\.[0-9]+)?$'
	local -a jmx_metrics=()
	# ====================================================================
	# 1. KRaft Controller Health Checks (Critical)
	# ====================================================================
	local -ri active_controllers_count=$(get_jmx_metric \
		"kafka.controller:name=ActiveControllerCount,type=KafkaController" \
		"kafka.controller" \
		"Value")

	local -ri offline_partitions_count=$(get_jmx_metric \
		"kafka.controller:name=OfflinePartitionsCount,type=KafkaController" \
		"kafka.controller" \
		"Value")
	if ((offline_partitions_count != 0)); then
		log_fatal "KafkaKRaftHealth" "❌  OfflinePartitionsCount = $offline_partitions_count (expected 0)"
	fi
	jmx_metrics+=("OfflinePartitionsCount = ${offline_partitions_count}")

	local -ri metadata_error_count=$(get_jmx_metric \
		"kafka.controller:name=MetadataErrorCount,type=KafkaController" \
		"kafka.controller" \
		"Value")
	if ((metadata_error_count != 0)); then
		log_fatal "KafkaKRaftHealth" "❌  MetadataErrorCount = ${metadata_error_count} (expected 0)"
	fi
	jmx_metrics+=("MetadataErrorCount = ${metadata_error_count}")

	# ====================================================================
	# 2. KRaft Raft Quorum Health Checks (Critical)
	# ====================================================================
	local -r current_raft_leader_id=$(get_jmx_metric \
		"kafka.server:type=raft-metrics" \
		"kafka.server" \
		"current-leader")
	if (($(echo "$current_raft_leader_id < 0" | bc -l))); then
		log_fatal "KafkaKRaftHealth" "❌  KRaft Current Leader ID = ${current_raft_leader_id} (No Leader/Quorum failure)"
	fi
	jmx_metrics+=("KRaft Current Leader ID = ${current_raft_leader_id}")

	local -r current_raft_state=$(get_jmx_metric \
		"kafka.server:type=raft-metrics" \
		"kafka.server" \
		"current-state")
	if [[ "$current_raft_state" == "leader" ]]; then
		if ((active_controllers_count != 1)); then
			log_fatal "KafkaKRaftHealth" "🚨 Mismatch: State is 'leader', but ActiveControllerCount is $active_controllers_count. Expected 1."
		fi
	elif [[ "$current_raft_state" == "follower" ]]; then
		if ((active_controllers_count != 0)); then
			log_fatal "KafkaKRaftHealth" "🚨 Mismatch: State is 'follower', but ActiveControllerCount is $active_controllers_count. Expected 0."
		fi
	else
		log_fatal "KafkaKRaftHealth" "❌  KRaft Current State = ${current_raft_state}"
	fi
	jmx_metrics+=("KRaft Current State = ${current_raft_state}")
	jmx_metrics+=("ActiveControllerCount = ${active_controllers_count}")

	local -r raft_commit_latency_avg=$(get_jmx_metric \
		"kafka.server:type=raft-metrics" \
		"kafka.server" \
		"commit-latency-avg")
	if ! [[ "$raft_commit_latency_avg" =~ $numeric_regex ]]; then
		log_error "KafkaKRaftHealth" "🚨  Raft Commit Latency value '$raft_commit_latency_avg' is invalid/non-numeric. Cannot compare."
	elif [[ "$(echo "$raft_commit_latency_avg > 1000" | bc -l)" -eq 1 ]]; then
		log_fatal "KafkaKRaftHealth" "❌  Raft Commit Latency: AVG=${raft_commit_latency_avg}ms (higher than 1000ms)"
	fi
	jmx_metrics+=("Raft Commit Latency: AVG=${raft_commit_latency_avg} ms")

	# ====================================================================
	# 3. Partition Replication Health Checks
	# ====================================================================
	local -ri under_replicated_partitions=$(
		get_jmx_metric \
			"kafka.server:name=UnderReplicatedPartitions,type=ReplicaManager" \
			"kafka.server" \
			"Value"
	)
	if ((under_replicated_partitions != 0)); then
		log_fatal "KafkaKRaftHealth" "❌  UnderReplicatedPartitions = $under_replicated_partitions (expected 0)"
	fi
	jmx_metrics+=("UnderReplicatedPartitions = ${under_replicated_partitions}")

	local -ri under_min_isr_partition_count=$(get_jmx_metric \
		"kafka.server:name=UnderMinIsrPartitionCount,type=ReplicaManager" \
		"kafka.server" \
		"Value")
	if ((under_min_isr_partition_count != 0)); then
		log_fatal "KafkaKRaftHealth" "❌  UnderMinIsrPartitionCount = $under_min_isr_partition_count (expected 0)"
	fi
	jmx_metrics+=("UnderMinIsrPartitionCount = ${under_min_isr_partition_count}")

	# ====================================================================
	# 4. Network Checks
	# ====================================================================
	local -ri request_queue_size=$(get_jmx_metric \
		"kafka.network:name=RequestQueueSize,type=RequestChannel" \
		"kafka.network" \
		"Value")
	if ((request_queue_size > 100)); then
		log_warn "KafkaKRaftHealth" "⚠️  High RequestQueueSize = $request_queue_size (possible broker overload)"
	else
		jmx_metrics+=("RequestQueueSize = ${request_queue_size}")
	fi

	# All good
	log_info "KafkaKRaftHealth" $"$(format_text "JMX checks passed" "bold" "italic" "green"):$(printf "\n ")$(printf "\t\t\t\t\t\t\t- %s\n" "${jmx_metrics[@]}")"
}
check_kafka_kraft_health

# 6 Defining ACL's
define_acl() {
	_define_acl_impl() {
		local -r marker_file="/opt/kafka/config/acl_initialized.marker"

		if [ ! -f "$marker_file" ]; then
			log_info "AclDefinition" "Marker file not found: ${marker_file}. Running ACL setup."

			local -r user_inbox_topic_prefix="msg-user-inbox-"
			local -r group_chat_topic_prefix="msg-group-chat-"
			local -r system_alerts_topic="msg-system-alerts"
			local -r chat_messages_consumer_group_prefix="msg-chat"

			local -r schema_registry_schemas_topic="_csr_schemas"
			local -r schema_registry_group_id="schema-registry-grp"

			# ====================================================================
			# 1. chat_admin
			# ====================================================================
			local -r chat_admin_principal="User:chat_admin"
			# 1. CLUSTER management
			kafka_exec kafka-acls \
				--cluster "$CLUSTER_ID" \
				--add \
				--operation Create \
				--operation Alter \
				--operation Describe \
				--operation DescribeConfigs \
				--operation AlterConfigs \
				--allow-host '*' \
				--allow-principal "$chat_admin_principal"

			# 2. TOPIC management
			kafka_exec kafka-acls \
				--topic "$user_inbox_topic_prefix" \
				--resource-pattern-type PREFIXED \
				--add \
				--operation Create \
				--operation Delete \
				--operation Alter \
				--operation Describe \
				--operation DescribeConfigs \
				--operation AlterConfigs \
				--allow-host '*' \
				--allow-principal "$chat_admin_principal"

			kafka_exec kafka-acls \
				--topic "$group_chat_topic_prefix" \
				--resource-pattern-type PREFIXED \
				--add \
				--operation Create \
				--operation Delete \
				--operation Alter \
				--operation Describe \
				--operation DescribeConfigs \
				--operation AlterConfigs \
				--allow-host '*' \
				--allow-principal "$chat_admin_principal"

			kafka_exec kafka-acls \
				--topic "$system_alerts_topic" \
				--resource-pattern-type LITERAL \
				--add \
				--operation Create \
				--operation Delete \
				--operation Alter \
				--operation Describe \
				--operation DescribeConfigs \
				--operation AlterConfigs \
				--allow-host '*' \
				--allow-principal "$chat_admin_principal"

			# 3. GROUP management
			kafka_exec kafka-acls \
				--group "$chat_messages_consumer_group_prefix" \
				--resource-pattern-type PREFIXED \
				--add \
				--operation Read \
				--operation Delete \
				--operation Describe \
				--allow-host '*' \
				--allow-principal "$chat_admin_principal"

			# ====================================================================
			# 2. chat_prod_cons
			# ====================================================================
			local -r chat_prod_cons_principal="User:chat_prod_cons"
			# 1. TOPIC access
			kafka_exec kafka-acls \
				--topic "$user_inbox_topic_prefix" \
				--resource-pattern-type PREFIXED \
				--add \
				--operation Read \
				--operation Write \
				--operation Describe \
				--allow-host '*' \
				--allow-principal "$chat_prod_cons_principal"

			kafka_exec kafka-acls \
				--topic "$group_chat_topic_prefix" \
				--resource-pattern-type PREFIXED \
				--add \
				--operation Read \
				--operation Write \
				--operation Describe \
				--allow-host '*' \
				--allow-principal "$chat_prod_cons_principal"

			kafka_exec kafka-acls \
				--topic "$system_alerts_topic" \
				--resource-pattern-type LITERAL \
				--add \
				--operation Read \
				--operation Write \
				--operation Describe \
				--allow-host '*' \
				--allow-principal "$chat_prod_cons_principal"

			kafka_exec kafka-acls \
				--cluster "$CLUSTER_ID" \
				--add \
				--operation IdempotentWrite \
				--allow-host '*' \
				--allow-principal "$chat_prod_cons_principal"

			# 2. GROUP access
			kafka_exec kafka-acls \
				--group "$chat_messages_consumer_group_prefix" \
				--resource-pattern-type PREFIXED \
				--add \
				--operation Read \
				--operation Describe \
				--allow-host '*' \
				--allow-principal "$chat_prod_cons_principal"

			# ====================================================================
			# 3. schema_registry
			# ====================================================================
			local -r schema_registry_principal="User:schema_registry"
			# 1. CLUSTER access
			kafka_exec kafka-acls \
				--cluster "$CLUSTER_ID" \
				--add \
				--operation Create \
				--operation Describe \
				--allow-host '*' \
				--allow-principal "$schema_registry_principal"

			# 2. TOPIC access
			kafka_exec kafka-acls \
				--topic "$schema_registry_schemas_topic" \
				--add \
				--operation Create \
				--operation Describe \
				--operation DescribeConfigs \
				--operation Read \
				--operation Write \
				--allow-host '*' \
				--allow-principal "$schema_registry_principal"

			kafka_exec kafka-acls \
				--topic __consumer_offsets \
				--add \
				--operation Describe \
				--allow-host '*' \
				--allow-principal "$schema_registry_principal"

			# 3. GROUP access
			kafka_exec kafka-acls \
				--group "schema-registry" \
				--add \
				--operation Read \
				--operation Describe \
				--allow-host '*' \
				--allow-principal "$schema_registry_principal"

			touch "${marker_file}"
			log_info "AclDefinition" "ACL setup complete. Marker file created."
		else
			log_debug "AclDefinition" "Marker file found. Skipping ACL setup."
		fi

		log_info "AclDefinition" "Current ACLs for all principals:"
		kafka_exec kafka-acls --list
	}

	(
		log_debug "AclDefinition" "Waiting to acquire lock..."
		flock 99 || log_fatal "AclDefinition" "❌  Failed to acquire lock."
		_define_acl_impl
	) 99>"/tmp/$CFG_SCRIPT_HASH-${FUNCNAME[0]}-lock"
}
define_acl

log_info "HealthCheck" "$(format_text "Kafka cluster is healthy" "bold" "italic" "underline" "green")"

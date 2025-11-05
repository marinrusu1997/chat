#!/usr/bin/env bash
set -Eeuo pipefail

# Import dependencies
. /etc/elasticsearch/scripts/logger.sh

declare -r VAR_LIB_DIR="/var/lib/elasticsearch"
declare -r INITIALIZATION_LOCK_FILE="$VAR_LIB_DIR/initialized.lock"
declare -r CERTS_DIR="/usr/share/elasticsearch/config/certs"
declare -r CA_CERTS_DIR="$CERTS_DIR/ca"
declare -r CA_CERT="$CA_CERTS_DIR/ca.crt"
declare -r CA_KEY="$CA_CERTS_DIR/ca.key"
declare -r CERTS_BLUEPRINT="/etc/elasticsearch/instances.yml"
declare -r SCHEMAS_DIR="/etc/elasticsearch/schemas"
declare -r CHAT_MESSAGES_SCHEMA_DIR="$SCHEMAS_DIR/chat-messages"
declare -r ES_HOST="es-coordinating-1:9200"

es_api_request() {
	local -r method="$1"
	local -r endpoint="$2"
	local -r payload="$3"
	local -r description="$4"
	local -r expected_status_code="${5:-200}"

	local curl_opts=(-s -w "\n%{http_code}" -X "$method" --cacert "$CA_CERT"
		-u "${ELASTICSEARCH_ELASTIC_USERNAME}:${ELASTICSEARCH_ELASTIC_PASSWORD}")

	log_info "API" "Executing ${method} ${endpoint} for ${description}"

	if [[ "$method" == "PUT" || "$method" == "POST" ]] && [[ -n "$payload" ]]; then
		curl_opts+=(-H "Content-Type: application/json")
		if [[ "$payload" == @* ]]; then
			curl_opts+=(--data-binary "$payload")
		else
			curl_opts+=(-d "$payload")
		fi
	fi

	local -r response=$(curl "${curl_opts[@]}" "https://${ES_HOST}/${endpoint}")
	local -r status_code=$(echo "$response" | tail -n1)
	local -r body=$(echo "$response" | sed '$d')

	# If expected_status_code is -1, just return the actual code.
	if [ "$expected_status_code" -eq -1 ]; then
		echo "$status_code"
		return
	fi

	# Otherwise, check if the actual status matches the expected one.
	if [ "$status_code" -ne "$expected_status_code" ]; then
		log_fatal "API" "❌ Failed execution of ${method} ${endpoint} for ${description}. Expected status ${expected_status_code}, got ${status_code}. Response: ${body}"
	fi
}

validate_vars() {
	for var_name in "$@"; do
		# Use indirect expansion to get the value of the variable whose name is var_name
		local value="${!var_name}"
		if [[ -z "$value" ]]; then
			log_fatal "Environment" "❌ The environment variable ${var_name} is not set. Please define it in the .env file."
		elif [[ "${#value}" -lt 2 ]]; then
			log_fatal "Environment" "❌ The environment variable ${var_name} is too short. It must be at least 2 characters long."
		fi
	done
	log_info "Environment" "✅ All required environment variables are set."
}

if [ -f "$INITIALIZATION_LOCK_FILE" ]; then
	log_info "Initialization" "✅ Initialization already completed. Exiting."
	exec tail -f /dev/null
	exit 0
fi

validate_vars \
	"ELASTICSEARCH_ELASTIC_USERNAME" \
	"ELASTICSEARCH_ELASTIC_PASSWORD" \
	"ELASTICSEARCH_KIBANA_SYSTEM_USERNAME" \
	"ELASTICSEARCH_KIBANA_SYSTEM_PASSWORD" \
	"ELASTICSEARCH_KIBANA_ADMIN_USERNAME" \
	"ELASTICSEARCH_KIBANA_ADMIN_PASSWORD" \
	"ELASTICSEARCH_CHAT_APP_USERNAME" \
	"ELASTICSEARCH_CHAT_APP_PASSWORD" \
	"ELASTICSEARCH_FILEBEAT_WRITER_USERNAME" \
	"ELASTICSEARCH_FILEBEAT_WRITER_PASSWORD" \
	"ELASTICSEARCH_FILEBEAT_MONITORING_USERNAME" \
	"ELASTICSEARCH_FILEBEAT_MONITORING_PASSWORD"

if [ ! -f "$CA_CERT" ]; then
	log_info "Certifications" "Creating certificates."

	bin/elasticsearch-certutil ca --silent --pem -out "$CERTS_DIR/ca.zip"
	unzip "$CERTS_DIR/ca.zip" -d "$CERTS_DIR"
	rm "$CERTS_DIR/ca.zip"

	bin/elasticsearch-certutil cert --silent --pem -out "$CERTS_DIR/certs.zip" --in "$CERTS_BLUEPRINT" \
		--ca-cert "$CA_CERT" --ca-key "$CA_KEY"
	unzip "$CERTS_DIR/certs.zip" -d "$CERTS_DIR"
	rm "$CERTS_DIR/certs.zip"

	chown -R root:root "$CERTS_DIR"
	find "$CERTS_DIR" -type d -exec chmod 750 \{\} \;
	find "$CERTS_DIR" -type f -exec chmod 640 \{\} \;
fi

log_info "ClusterAvailability" "⏳ Waiting for Elasticsearch availability..."
sleep 15
until curl -s --cacert "$CA_CERT" \
	-u "${ELASTICSEARCH_ELASTIC_USERNAME}:${ELASTICSEARCH_ELASTIC_PASSWORD}" \
	https://${ES_HOST}/_cluster/health | grep -q -E '\"status\":\"(yellow|green)\"'; do
	log_warn "ClusterAvailability" "Elasticsearch is not available yet. Retrying in 15 seconds..."
	sleep 15
done

log_info "Users" "⏳ Setting built-in $ELASTICSEARCH_KIBANA_SYSTEM_USERNAME user password..."
until (
	curl -s -X POST --cacert "$CA_CERT" -u "${ELASTICSEARCH_ELASTIC_USERNAME}:${ELASTICSEARCH_ELASTIC_PASSWORD}" \
		-H "Content-Type: application/json" https://${ES_HOST}/_security/user/${ELASTICSEARCH_KIBANA_SYSTEM_USERNAME}/_password \
		-d "{\"password\":\"${ELASTICSEARCH_KIBANA_SYSTEM_PASSWORD}\"}" | grep -q "^{}"
); do
	log_warn "Users" "Could not set $ELASTICSEARCH_KIBANA_SYSTEM_USERNAME password. Retrying in 10 seconds..."
	sleep 10
done

PAYLOAD='{
   "cluster": ["all"],
   "indices": [
     {
       "names": [ "*" ],
       "privileges": ["all"]
     }
   ],
   "applications": [
     {
       "application": "kibana-.kibana",
       "privileges": ["all"],
       "resources": ["*"]
     }
   ]
}'
es_api_request "PUT" "_security/role/kibana_admin_role" "$PAYLOAD" "create kibana_admin_role" 200

PAYLOAD='{
    "cluster": ["monitor"],
    "indices": [
      {
        "names": [ "chat-messages-" ],
        "privileges": ["read", "write"]
      }
    ]
}'
es_api_request "PUT" "_security/role/chat_app_role" "$PAYLOAD" "create chat_app_role" 200

PAYLOAD="{
  \"password\": \"${ELASTICSEARCH_KIBANA_ADMIN_PASSWORD}\",
  \"roles\": [ \"kibana_admin_role\" ]
}"
es_api_request "PUT" "_security/user/${ELASTICSEARCH_KIBANA_ADMIN_USERNAME}" "$PAYLOAD" "create $ELASTICSEARCH_KIBANA_ADMIN_USERNAME user" 200

PAYLOAD="{
  \"password\": \"${ELASTICSEARCH_CHAT_APP_PASSWORD}\",
  \"roles\": [ \"chat_app_role\" ]
}"
es_api_request "PUT" "_security/user/${ELASTICSEARCH_CHAT_APP_USERNAME}" "$PAYLOAD" "create $ELASTICSEARCH_CHAT_APP_USERNAME user" 200

PAYLOAD='{
  "cluster": ["monitor", "read_ilm", "manage_ilm"],
  "indices": [
    {
      "names": [ "filebeat-*" ],
      "privileges": ["create_doc", "create_index", "manage_ilm", "auto_configure"]
    }
  ]
}'
es_api_request "PUT" "_security/role/filebeat_writer_role" "$PAYLOAD" "create filebeat_writer_role" 200

PAYLOAD="{
  \"password\": \"${ELASTICSEARCH_FILEBEAT_WRITER_PASSWORD}\",
  \"roles\": [ \"filebeat_writer_role\" ]
}"
es_api_request "PUT" "_security/user/${ELASTICSEARCH_FILEBEAT_WRITER_USERNAME}" "$PAYLOAD" "create $ELASTICSEARCH_FILEBEAT_WRITER_USERNAME user" 200

PAYLOAD='{
  "cluster": ["monitor"],
  "indices": [
    {
      "names": [ ".monitoring-beats-*" ],
      "privileges": ["create_doc", "create_index"]
    }
  ]
}'
es_api_request "PUT" "_security/role/beats_monitoring_role" "$PAYLOAD" "create beats_monitoring_role" 200

PAYLOAD="{
  \"password\": \"${ELASTICSEARCH_FILEBEAT_MONITORING_PASSWORD}\",
  \"roles\": [ \"beats_admin\", \"beats_monitoring_role\" ]
}"
es_api_request "PUT" "_security/user/${ELASTICSEARCH_FILEBEAT_MONITORING_USERNAME}" "$PAYLOAD" "create $ELASTICSEARCH_FILEBEAT_MONITORING_USERNAME user" 200

PAYLOAD='{
    "description":"Adds a timestamp to documents",
    "processors":[{
      "set":{
        "field":"@timestamp",
        "value":"{{_ingest.timestamp}}"
      }
    }]
}'
es_api_request "PUT" "_ingest/pipeline/add_timestamp" "$PAYLOAD" "create add_timestamp pipeline" 200

# Setup chat messages index, templates and ILM
PAYLOAD="@$CHAT_MESSAGES_SCHEMA_DIR/ilm.json"
es_api_request "PUT" "_ilm/policy/90_day_retention_policy" "$PAYLOAD" "create ILM 90_day_retention_policy" 200

PAYLOAD="@$CHAT_MESSAGES_SCHEMA_DIR/settings.json"
es_api_request "PUT" "_component_template/chat_messages_settings_template" "$PAYLOAD" "create chat_messages_settings_template" 200

PAYLOAD="@$CHAT_MESSAGES_SCHEMA_DIR/mappings.json"
es_api_request "PUT" "_component_template/chat_messages_mappings_template" "$PAYLOAD" "create chat_messages_mappings_template" 200

PAYLOAD="@$CHAT_MESSAGES_SCHEMA_DIR/template.json"
es_api_request "PUT" "_index_template/chat_messages_idx_template" "$PAYLOAD" "create chat_messages_idx_template" 200

es_api_request "PUT" "_data_stream/chat-messages-" "" "create chat-messages data stream" 200
curl --cacert "$CA_CERT" -u "${ELASTICSEARCH_ELASTIC_USERNAME}:${ELASTICSEARCH_ELASTIC_PASSWORD}" \
	https://${ES_HOST}/_data_stream/chat-messages-

touch "$INITIALIZATION_LOCK_FILE"
log_info "Initialization" "✅ All done!"
exec tail -f /dev/null
exit 0

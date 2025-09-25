#!/bin/bash
set -e

# Import dependencies
. /tmp/scripts/logger.sh

LOCK_FILE="config/certs/setup-complete"
ES_HOST="es-coordinating-1:9200"

es_api_request() {
  local method="$1"
  local endpoint="$2"
  local payload="$3"
  local description="$4"
  local expected_status_code="${5:-200}"

  local curl_opts=(-s -w "\n%{http_code}" -X "$method" --cacert config/certs/ca/ca.crt -u "elastic:${ELASTICSEARCH_ELASTIC_PASSWORD}")

  log_info "API" "Executing ${method} ${endpoint} for ${description}"

  if [[ "$method" == "PUT" || "$method" == "POST" ]] && [[ -n "$payload" ]]; then
    curl_opts+=(-H "Content-Type: application/json")
    if [[ "$payload" == @* ]]; then
      curl_opts+=(--data-binary "$payload")
    else
      curl_opts+=(-d "$payload")
    fi
  fi

  RESPONSE=$(curl "${curl_opts[@]}" "https://${ES_HOST}/${endpoint}")

  STATUS_CODE=$(echo "$RESPONSE" | tail -n1)
  BODY=$(echo "$RESPONSE" | sed '$d')

  # If expected_status_code is -1, just return the actual code.
  if [ "$expected_status_code" -eq -1 ]; then
    echo "$STATUS_CODE"
    return
  fi

  # Otherwise, check if the actual status matches the expected one.
  if [ "$STATUS_CODE" -ne "$expected_status_code" ]; then
    log_fatal "API" "❌ Failed execution of ${method} ${endpoint} for ${description}. Expected status ${expected_status_code}, got ${STATUS_CODE}. Response: ${BODY}";
  fi;
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

if [ -f "$LOCK_FILE" ]; then
  log_info "Initialization" "✅ Initialization already completed. Exiting."
  exit 0
fi

validate_vars \
      "ELASTICSEARCH_ELASTIC_PASSWORD" \
      "ELASTICSEARCH_KIBANA_SYSTEM_USERNAME" \
      "ELASTICSEARCH_KIBANA_SYSTEM_PASSWORD" \
      "ELASTICSEARCH_KIBANA_ADMIN_USERNAME" \
      "ELASTICSEARCH_KIBANA_ADMIN_PASSWORD" \
      "ELASTICSEARCH_CHAT_APP_USERNAME" \
      "ELASTICSEARCH_CHAT_APP_PASSWORD"

if [ ! -f config/certs/ca.zip ]; then
  log_info "Certifications" "Creating CA.";
  bin/elasticsearch-certutil ca --silent --pem -out config/certs/ca.zip;
  unzip config/certs/ca.zip -d config/certs;
fi;

if [ ! -f config/certs/certs.zip ]; then
  log_info "Certifications" "Creating certs.";
  bin/elasticsearch-certutil cert --silent --pem -out config/certs/certs.zip --in config/certs/instances.yml --ca-cert config/certs/ca/ca.crt --ca-key config/certs/ca/ca.key;
  unzip config/certs/certs.zip -d config/certs;
fi;

log_info "Certifications" "Setting file permissions.";
chown -R root:root config/certs;
find . -type d -exec chmod 750 \{\} \;;
find . -type f -exec chmod 640 \{\} \;;

log_info "ClusterAvailability" "⏳ Waiting for Elasticsearch availability...";
sleep 15;
until curl -s --cacert config/certs/ca/ca.crt \
    -u "elastic:${ELASTICSEARCH_ELASTIC_PASSWORD}" https://${ES_HOST}/_cluster/health | grep -q -E '\"status\":\"(yellow|green)\"';
do
  log_warn "ClusterAvailability" "Elasticsearch is not available yet. Retrying in 30 seconds...";
  sleep 15;
done;

log_info "Users" "⏳ Setting built-in $ELASTICSEARCH_KIBANA_SYSTEM_USERNAME user password...";
until ( \
  curl -s -X POST --cacert config/certs/ca/ca.crt -u "elastic:${ELASTICSEARCH_ELASTIC_PASSWORD}" \
      -H "Content-Type: application/json" https://${ES_HOST}/_security/user/${ELASTICSEARCH_KIBANA_SYSTEM_USERNAME}/_password \
      -d "{\"password\":\"${ELASTICSEARCH_KIBANA_SYSTEM_PASSWORD}\"}" | grep -q "^{}");
do
  log_warn "Users" "Could not set $ELASTICSEARCH_KIBANA_SYSTEM_USERNAME password. Retrying in 10 seconds...";
  sleep 10;
done;

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
es_api_request "PUT" "_security/role/kibana_admin_role" "$PAYLOAD" "Create kibana_admin_role" 200

PAYLOAD='{
    "cluster": ["monitor"],
    "indices": [
      {
        "names": [ "chat-messages-" ],
        "privileges": ["read", "write"]
      }
    ]
}'
es_api_request "PUT" "_security/role/chat_app_role" "$PAYLOAD" "Create chat_app_role" 200

PAYLOAD="{
  \"password\": \"${ELASTICSEARCH_KIBANA_ADMIN_PASSWORD}\",
  \"roles\": [ \"kibana_admin_role\" ]
}"
es_api_request "PUT" "_security/user/${ELASTICSEARCH_KIBANA_ADMIN_USERNAME}" "$PAYLOAD" "Create $ELASTICSEARCH_KIBANA_ADMIN_USERNAME user" 200

PAYLOAD="{
  \"password\": \"${ELASTICSEARCH_CHAT_APP_PASSWORD}\",
  \"roles\": [ \"chat_app_role\" ]
}"
es_api_request "PUT" "_security/user/${ELASTICSEARCH_CHAT_APP_USERNAME}" "$PAYLOAD" "Create $ELASTICSEARCH_CHAT_APP_USERNAME user" 200

PAYLOAD='{
  "policy": {
    "phases": {
      "hot": {
        "min_age": "0ms",
        "actions": {
          "rollover": {
            "max_age": "7d",
            "max_primary_shard_size": "50gb"
          }
        }
      },
      "warm": {
        "min_age": "1s",
        "actions": {
          "forcemerge": {
            "max_num_segments": 1
          },
          "shrink": {
            "number_of_shards": 1
          },
          "allocate": {
            "number_of_replicas": 1,
            "require": {
              "data_tier": "warm"
            }
          }
        }
      },
      "delete": {
        "min_age": "90d",
        "actions": {
          "delete": {}
        }
      }
    }
  }
}'
es_api_request "PUT" "_ilm/policy/90_day_retention_policy" "$PAYLOAD" "create ILM 90_day_retention_policy" 200

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

PAYLOAD="@/tmp/schemas/chat-messages/settings.json"
es_api_request "PUT" "_component_template/chat_messages_settings_template" "$PAYLOAD" "create chat_messages_settings_template" 200

PAYLOAD="@/tmp/schemas/chat-messages/mappings.json"
es_api_request "PUT" "_component_template/chat_messages_mappings_template" "$PAYLOAD" "create chat_messages_mappings_template" 200

PAYLOAD="@/tmp/schemas/chat-messages/template.json"
es_api_request "PUT" "_index_template/chat_messages_idx_template" "$PAYLOAD" "create chat_messages_idx_template" 200

es_api_request "PUT" "_data_stream/chat-messages-" "" "create chat-messages data stream" 200
curl --cacert config/certs/ca/ca.crt -u "elastic:${ELASTICSEARCH_ELASTIC_PASSWORD}" https://${ES_HOST}/_data_stream/chat-messages-

touch "$LOCK_FILE"
log_info "Initialization" "✅ All done!"
#!/bin/bash
set -e

# Import dependencies
. /tmp/scripts/logger.sh

# The lock file will be stored in the persistent data directory
LOCK_FILE="/usr/share/filebeat/data/.setup-complete"

# Check if the setup has already been completed
if [ -f "$LOCK_FILE" ]; then
    log_info "Initialization" "✅ Filebeat setup has already been completed. Starting Filebeat..."
else
    log_info "Initialization" "Performing one-time Filebeat setup..."
    chmod go-w /usr/share/filebeat/filebeat.yml

    # Run the setup command. The -e flag logs output to stderr and exits.
    # We add a '|| true' to prevent the script from exiting if setup fails,
    # allowing us to see the logs. We rely on the healthcheck for actual status.
    filebeat \
      -E "setup.kibana.username=${ELASTICSEARCH_ELASTIC_USERNAME}" \
      -E "setup.kibana.password=${ELASTICSEARCH_ELASTIC_PASSWORD}" \
      -E "output.elasticsearch.username=${ELASTICSEARCH_ELASTIC_USERNAME}" \
      -E "output.elasticsearch.password=${ELASTICSEARCH_ELASTIC_PASSWORD}" \
      setup --dashboards --pipelines --enable-all-filesets --force-enable-module-filesets --index-management -e

    # If setup was successful (which it should be if ES/Kibana are up),
    # create the lock file to prevent running it again.
    log_info "Initialization" "✅ Setup complete. Creating lock file."
    touch "$LOCK_FILE"
fi

log_info "Initialization" "Executing original Filebeat command..."
# Use 'exec' to replace the script process with the Filebeat process
exec filebeat -e
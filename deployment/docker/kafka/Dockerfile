# Start from your base Kafka image
FROM apache/kafka:4.1.0

# Define the user and group Kafka runs as (e.g., typically 'kafka' or 'appuser')
ARG KAFKA_USER=appuser
ARG KAFKA_GROUP=appuser

# Set the path for the secret file
ARG JMX_ACCESS_FILE=/opt/kafka/config/secrets/jmxremote.access
ARG JMX_PWD_FILE=/opt/kafka/config/secrets/jmxremote.password

# Copy the file into a temporary location first (assuming your host file is named jmxremote.password)
COPY ./secrets/jmxremote.access /tmp/jmxremote.access
COPY ./secrets/jmxremote.password /tmp/jmxremote.password

# Set permissions and ownership
# Switch to the root user temporarily if needed to run chown/chmod
USER root

RUN mkdir -p $(dirname $JMX_PWD_FILE) && \
    mkdir -p $(dirname $JMX_ACCESS_FILE) && \
    mv /tmp/jmxremote.password $JMX_PWD_FILE && \
    mv /tmp/jmxremote.access $JMX_ACCESS_FILE && \
    chown ${KAFKA_USER}:${KAFKA_GROUP} $JMX_PWD_FILE $JMX_ACCESS_FILE && \
    chmod 400 $JMX_PWD_FILE $JMX_ACCESS_FILE

# Switch back to the non-root user for security (if the base image uses one)
USER ${KAFKA_USER}
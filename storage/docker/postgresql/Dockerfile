# Use a PostGIS image built on PostgreSQL 16, where extensions are available.
FROM postgis/postgis:16-3.5

# Install required PostgreSQL extensions and tools.
RUN apt-get update && apt-get install -y \
    postgresql-16-partman \
    postgresql-16-cron \
    postgresql-plpython3-16 \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Install Patroni and its dependencies via pip.
# The `[psycopg,etcd]` part installs the Python database driver and the etcd client.
RUN pip3 install 'patroni[psycopg,etcd]'

# Patroni runs as a non-root user for security.
# We must create a dedicated directory for its configuration.
RUN mkdir /etc/patroni && chown postgres:postgres /etc/patroni

# Set the container's entrypoint to run Patroni.
# The configuration file argument will be supplied by docker-compose.
ENTRYPOINT ["/usr/local/bin/patroni"]
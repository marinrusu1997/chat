# Use a PostGIS image built on PostgreSQL 16, where extensions are available.
FROM postgis/postgis:16-3.5

# Install required PostgreSQL extensions and tools.
RUN apt-get update && apt-get install -y \
    bash \
    wget \
    git \
    ca-certificates \
    postgresql-16-partman \
    postgresql-16-cron \
    postgresql-plpython3-16 \
    pgpool2 \
    procps \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Install Patroni and its dependencies via pip.
# The `[psycopg,etcd]` part installs the Python database driver and the etcd client.
RUN pip3 install 'patroni[psycopg,etcd]==4.0.6'

# Patroni runs as a non-root user for security.
# We must create a dedicated directory for its configuration.
RUN mkdir /etc/patroni && chown postgres:postgres /etc/patroni

# Build Golang
RUN wget https://go.dev/dl/go1.25.1.linux-amd64.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf go1.25.1.linux-amd64.tar.gz && \
    rm -f go1.25.1.linux-amd64.tar.gz && \
    ln -s /usr/local/go/bin/go /usr/local/bin/go && \
    chmod 555 /usr/local/go/bin/go

# Build goose with postgres driver only
RUN git clone --depth 1 --branch v3.25.0 https://github.com/pressly/goose && \
    cd goose && \
    go mod tidy && \
    go build \
        -ldflags="-s -w" \
        -tags='no_clickhouse no_mssql no_mysql no_sqlite3 no_vertica no_ydb' \
        -o /usr/local/bin/goose ./cmd/goose && \
    cd .. && rm -rf goose && \
    chmod 555 /usr/local/bin/goose

# Set the container's entrypoint to run Patroni.
# The configuration file argument will be supplied by docker-compose.
ENTRYPOINT ["/usr/local/bin/patroni"]
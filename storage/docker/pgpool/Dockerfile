# Use a modern, slim Debian base image
FROM debian:bookworm-slim

# Install prerequisites and the official PostgreSQL repository
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    gnupg \
    wget \
    python3 \
    python3-pip \
    && echo "deb http://apt.postgresql.org/pub/repos/apt/ bookworm-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
    && wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | apt-key add -

# Install the latest Pgpool-II and the PostgreSQL client tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    pgpool2 \
    postgresql-client-16 \
    postgresql-client-common \
    && pip3 install "patroni[etcd3]" --break-system-packages \
    && rm -rf /var/lib/apt/lists/*

RUN echo "your_secret_key" > /var/lib/postgresql/.pgpoolkey \
    && chmod 600 /var/lib/postgresql/.pgpoolkey \
    && chown postgres:postgres /var/lib/postgresql/.pgpoolkey

# The pgpool2 package creates a 'postgres' user, which is standard for running the service.
USER postgres

# The default command to start Pgpool-II in the foreground
CMD ["/usr/sbin/pgpool", "-n", "-f", "/etc/pgpool2/pgpool.conf"]
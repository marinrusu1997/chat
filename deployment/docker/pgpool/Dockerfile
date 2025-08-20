# Use a modern, slim Debian base image
FROM debian:bookworm-slim

# Install prerequisites and the official PostgreSQL repository
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    gnupg \
    gosu \
    wget \
    jq \
    curl \
    procps \
    python3 \
    python3-pip \
    && echo "deb http://apt.postgresql.org/pub/repos/apt/ bookworm-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
    && wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | apt-key add -

# Install Pgpool-II and the PostgreSQL client tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    pgpool2 \
    postgresql-client-16 \
    && rm -rf /var/lib/apt/lists/*
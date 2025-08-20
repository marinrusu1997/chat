# Dockerfile
# Stage 1: Get the official postgres-exporter binary
FROM prometheuscommunity/postgres-exporter:v0.17.1 as exporter

# Stage 2: Create our final image
# Use a minimal base image that includes a shell and basic utilities
FROM alpine:3.20

# Install curl (and ca-certificates for HTTPS, which is good practice)
RUN apk add --no-cache curl ca-certificates bash

# Copy the postgres-exporter binary from the first stage
COPY --from=exporter /bin/postgres_exporter /bin/postgres_exporter

# Expose the default port
EXPOSE 9187

# Set the entrypoint to run the exporter
ENTRYPOINT [ "/bin/postgres_exporter" ]
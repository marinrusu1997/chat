# Use lightweight image with OpenSSL and Java keytool
FROM openjdk:17.0.1-jdk-slim

# Install OpenSSL and bash (if not included)
RUN apt-get update && apt-get install -y openssl bash && rm -rf /var/lib/apt/lists/*

# Set workdir
WORKDIR /certs

# Copy the script
COPY scripts/generate.sh /certs/generate.sh

# Make it executable
RUN chmod +x /certs/generate.sh

# Default command: run the script
CMD ["/certs/generate.sh"]
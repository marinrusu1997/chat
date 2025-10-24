#!/bin/bash
set -e

# ---------------------------------------------
# Sample usage:
#       ## assuming you are in './deployment/docker/' directory ##
#       docker run --rm -v "./kafka/init/output:/app/output" -v "./kafka/init/init.sh:/app/init.sh" -w /app apache/kafka:4.1.0 bash init.sh
# ---------------------------------------------

# --- 1. Define Variables ---
TRUSTSTORE_PASS="mJ0_H84dZI3d_1SU_h_O"
KEYSTORE_PASS="mJ0_H84dZI3d_1SU_h_O"
CA_ALIAS="root_ca"
KAFKA_NODES=("kclient" "kafka1" "kafka2" "kafka3")

# --- 2. Setup Directories and Cluster ID (Standard) ---
CLUSTER_ID_FILE="/app/output/cluster_id.txt"
SECRETS_DIR="/app/output/secrets"
mkdir -p "$SECRETS_DIR"

echo "----- Starting PKI generation in $SECRETS_DIR"
echo "----- Using Truststore Password: $TRUSTSTORE_PASS"
echo "----- Using Keystore Password: $KEYSTORE_PASS"

if [ ! -f "$CLUSTER_ID_FILE" ]; then
	CLUSTER_ID=$(/opt/kafka/bin/kafka-storage.sh random-uuid)
	echo "$CLUSTER_ID" >"$CLUSTER_ID_FILE"
	echo "----- Generated Cluster ID: $CLUSTER_ID"
fi

# --- 3. CREATE CA: Generate CA Keypair and Export Public Certificate ---
# CA Keystore: Stores the CA's private key (only used for signing).
CA_KEYSTORE="$SECRETS_DIR/ca.keystore.jks"
# CA Certificate: Public part of the CA (used by brokers to build trust).
CA_CERT="$SECRETS_DIR/ca.crt"

# a) Generate CA Keypair (Self-signed)
keytool -genkeypair -alias "$CA_ALIAS" -keystore "$CA_KEYSTORE" \
	-dname 'CN=CA, OU=IT, O=CHAT, L=Anywhere, C=US' -storepass "$KEYSTORE_PASS" \
	-keypass "$KEYSTORE_PASS" -keyalg RSA -validity 365 \
	-ext BasicConstraints=ca:true
echo "----- Generated CA Keypair under $CA_KEYSTORE."

# b) Export CA's Public Certificate
keytool -exportcert -alias "$CA_ALIAS" -keystore "$CA_KEYSTORE" \
	-file "$CA_CERT" -storepass "$KEYSTORE_PASS" -rfc
echo "----- Exported CA Public Certificate to $CA_CERT."

# --- 4. CREATE TRUSTSTORE: Populate with CA Public Certificate (FIX 1) ---
# This store is used by ALL Kafka brokers to trust each other.
TRUSTSTORE_FILE="$SECRETS_DIR/kafka.truststore.jks"

# Import the CA's public certificate (NOT the keypair) into the shared Truststore.
keytool -importcert -alias "$CA_ALIAS" -file "$CA_CERT" -keystore "$TRUSTSTORE_FILE" \
	-storepass "$TRUSTSTORE_PASS" -noprompt -trustcacerts
echo "----- Created shared Truststore ($TRUSTSTORE_FILE) with CA public certificate."

# --- 5. CREATE BROKER KEYS AND SIGN THEM ---
for NODE in "${KAFKA_NODES[@]}"; do
	BROKER_KEYSTORE="$SECRETS_DIR/$NODE.keystore.jks"
	BROKER_CSR="$SECRETS_DIR/$NODE.csr"
	BROKER_CERT="$SECRETS_DIR/$NODE.crt"

	echo "----- Generating and signing key for $NODE"

	# a) Generate Broker Keypair and Certificate Signing Request (CSR)
	keytool -genkeypair -alias "$NODE" -keystore "$BROKER_KEYSTORE" \
		-dname "CN=$NODE, OU=Manage, O=CHAT, L=Anywhere, C=US" -storepass "$KEYSTORE_PASS" \
		-keypass "$KEYSTORE_PASS" -keyalg RSA -validity 365
	echo "----- Generated Keypair for $NODE in $BROKER_KEYSTORE."

	keytool -certreq -alias "$NODE" -keystore "$BROKER_KEYSTORE" \
		-file "$BROKER_CSR" -storepass "$KEYSTORE_PASS"
	echo "----- Created CSR for $NODE at $BROKER_CSR."

	# b) Use the CA's private key to SIGN the broker's CSR
	keytool -gencert -alias "$CA_ALIAS" -keystore "$CA_KEYSTORE" \
		-infile "$BROKER_CSR" -outfile "$BROKER_CERT" -storepass "$KEYSTORE_PASS" \
		-ext san=dns:"$NODE" -validity 365 -rfc
	echo "----- Signed certificate for $NODE and saved to $BROKER_CERT."

	# c) Import the Signed Certificate back into the Broker's Keystore
	# Note: Must import the CA cert first (for chain) before the signed broker cert
	keytool -importcert -alias "$CA_ALIAS" -file "$CA_CERT" \
		-keystore "$BROKER_KEYSTORE" -storepass "$KEYSTORE_PASS" -noprompt
	echo "----- Imported CA certificate into $NODE keystore."

	keytool -importcert -alias "$NODE" -file "$BROKER_CERT" \
		-keystore "$BROKER_KEYSTORE" -storepass "$KEYSTORE_PASS" -noprompt
	echo "----- Imported signed certificate into $NODE keystore."
done

#!/bin/bash

set -Eeuo pipefail

# ===========================
# Configurable variables
# ===========================
SSL_DIR="./ssl"
ROOT_CA_DIR="$SSL_DIR/ca"
SERVICES_DIR="$SSL_DIR/services"
DAYS_VALID=3650
JAVA_KEYSTORE_PASS="${CERT_GEN_JAVA_KEYSTORE_PASSWORD}"
JAVA_TRUSTSTORE_PASS="${CERT_GEN_JAVA_KEYSTORE_PASSWORD}"

# List of services to generate certificates for
SERVICES=(
	"etcd-node-1"
	"etcd-node-2"
	"etcd-node-3"
	"pg-node-1"
	"pg-node-2"
	"pg-node-3"
	"neo4j-node-1"
	"pgpool"
	"redis-node-1"
	"redis-node-2"
	"redis-node-3"
	"redis-node-4"
	"redis-node-5"
	"redis-node-6"
	"chat-app-1"
)

# ===========================
# 1. Create Root CA if missing
# ===========================
mkdir -p "$ROOT_CA_DIR"
ROOT_KEY="$ROOT_CA_DIR/root.key"
ROOT_CERT="$ROOT_CA_DIR/root.crt"
ROOT_SRL="$ROOT_CA_DIR/root.srl"
ROOT_TRUSTSTORE="$ROOT_CA_DIR/root-truststore.jks"

if [ ! -f "$ROOT_KEY" ] || [ ! -f "$ROOT_CERT" ]; then
	echo "----> Creating Root CA"
	openssl genrsa -out "$ROOT_KEY" 4096
	openssl req -x509 -new -nodes -key "$ROOT_KEY" -sha256 -days $DAYS_VALID -out "$ROOT_CERT" -subj "/CN=ChatRootCA"
else
	echo "----> Root CA already exists"
fi

if [ ! -f "$ROOT_TRUSTSTORE" ]; then
	echo "----> Creating shared Java truststore with Root CA"
	keytool -import -trustcacerts -file "$ROOT_CERT" -alias ChatRootCA \
		-keystore "$ROOT_TRUSTSTORE" -storepass "$JAVA_TRUSTSTORE_PASS" -noprompt
else
	echo "----> Shared Java truststore already exists"
fi

# ===========================
# 2. Generate certificates for each service
# ===========================
mkdir -p "$SERVICES_DIR"

for SERVICE in "${SERVICES[@]}"; do
	echo "----> Generating certificates for $SERVICE ..."
	SERVICE_DIR="$SERVICES_DIR/$SERVICE"
	rm -rf "$SERVICE_DIR"
	mkdir -p "$SERVICE_DIR"

	CNF="$SERVICE_DIR/service.cnf"
	KEY="$SERVICE_DIR/private.key"
	CSR="$SERVICE_DIR/service.csr"
	CRT="$SERVICE_DIR/public.crt"
	P12="$SERVICE_DIR/service.p12"
	KEYSTORE="$SERVICE_DIR/keystore.jks"

	# 2a. Generate private key
	openssl genrsa -out "$KEY" 4096

	# 2b. Generate CSR with SAN = hostname
	cat >"$CNF" <<EOF
[ req ]
default_bits       = 4096
prompt             = no
default_md         = sha256
req_extensions     = req_ext
distinguished_name = dn

[ dn ]
CN = $SERVICE

[ req_ext ]
subjectAltName = @alt_names

[ alt_names ]
DNS.1 = $SERVICE
EOF

	openssl req -new -key "$KEY" -out "$CSR" -config "$CNF"

	# 2c. Sign CSR with Root CA
	openssl x509 -req -in "$CSR" -CA "$ROOT_CERT" -CAkey "$ROOT_KEY" -CAcreateserial \
		-out "$CRT" -days $DAYS_VALID -sha256 -extfile "$CNF" -extensions req_ext

	# 2e. Generate PKCS12 (.p12) for Java
	openssl pkcs12 -export -in "$CRT" -inkey "$KEY" -certfile "$ROOT_CERT" -out "$P12" -name "$SERVICE" -passout pass:$JAVA_KEYSTORE_PASS

	# 2f. Generate Java keystore (.jks) from PKCS12
	keytool -importkeystore -deststorepass $JAVA_KEYSTORE_PASS -destkeypass $JAVA_KEYSTORE_PASS \
		-destkeystore "$KEYSTORE" -srckeystore "$P12" -srcstoretype PKCS12 -srcstorepass $JAVA_KEYSTORE_PASS -alias "$SERVICE"

	rm -rf "$CNF" "$CSR" "$P12"

	echo "----> Certificates for $SERVICE generated in $SERVICE_DIR"
done

find "$SSL_DIR" -type f -exec chmod 444 {} +
find "$SSL_DIR" -type d -exec chmod 555 {} +
chmod 644 "$ROOT_SRL"
echo "----> All certificates generated successfully!"

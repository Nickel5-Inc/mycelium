#!/bin/bash
set -e

# Create directory if it doesn't exist
mkdir -p tests/fixtures

# Generate CA key and certificate
openssl req -x509 -newkey rsa:4096 -days 365 -nodes \
  -keyout tests/fixtures/ca_key.pem \
  -out tests/fixtures/ca_cert.pem \
  -subj "/C=US/ST=CA/L=San Francisco/O=Mycelium Test/OU=Test/CN=Test CA"

# Generate server key and CSR
openssl req -newkey rsa:4096 -nodes \
  -keyout tests/fixtures/test_key.pem \
  -out tests/fixtures/test.csr \
  -subj "/C=US/ST=CA/L=San Francisco/O=Mycelium Test/OU=Test/CN=localhost"

# Sign the server certificate with our CA
openssl x509 -req -days 365 \
  -in tests/fixtures/test.csr \
  -CA tests/fixtures/ca_cert.pem \
  -CAkey tests/fixtures/ca_key.pem \
  -CAcreateserial \
  -out tests/fixtures/test_cert.pem

# Clean up CSR and serial
rm tests/fixtures/test.csr tests/fixtures/ca_cert.srl

# Set permissions
chmod 600 tests/fixtures/*.pem

echo "Test certificates generated successfully" 
#!/bin/bash
set -e

echo "=== Controller Setup ==="

# 1. Build controller
echo "Building controller..."
go build -o controller .
echo "✓ Built"

# 2. Generate CA if not exists
if [ ! -f ca/ca.crt ]; then
    echo "Generating CA certificates..."
    mkdir -p ca
    
    # Generate CA private key
    openssl ecparam -genkey -name prime256v1 -out ca/ca.key
    
    # Convert to PKCS8 format
    openssl pkcs8 -topk8 -nocrypt -in ca/ca.key -out ca/ca.pkcs8.key
    
    # Generate CA certificate
    openssl req -new -x509 -key ca/ca.key -out ca/ca.crt -days 3650 \
      -subj "/CN=Internal CA/O=MyCorp/C=US"
    
    echo "✓ CA certificates generated"
else
    echo "✓ CA certificates exist"
fi

# 3. Create .env if not exists
if [ ! -f .env ]; then
    echo "Creating .env file..."
    cat > .env << 'ENVEOF'
TRUST_DOMAIN=mycorp.internal
ADMIN_AUTH_TOKEN=7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4
INTERNAL_API_TOKEN=e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3
CONTROLLER_ADDR=localhost:8443
ADMIN_HTTP_ADDR=0.0.0.0:8081
ENVEOF
    echo "✓ .env created"
else
    echo "✓ .env exists"
fi

echo ""
echo "=== Setup Complete ==="
echo ""
echo "To run the controller:"
echo "  INTERNAL_CA_CERT=\"\$(cat ca/ca.crt)\" \\"
echo "  INTERNAL_CA_KEY=\"\$(cat ca/ca.pkcs8.key)\" \\"
echo "  ./controller"

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
    GENERATED_ADMIN_TOKEN=$(openssl rand -hex 16)
    GENERATED_API_TOKEN=$(openssl rand -hex 32)
    GENERATED_JWT_SECRET=$(openssl rand -hex 32)
    cat > .env <<ENVEOF
TRUST_DOMAIN=mycorp.internal
JWT_SECRET=${GENERATED_JWT_SECRET}
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

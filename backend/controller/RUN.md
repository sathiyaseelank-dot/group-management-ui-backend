# file path

cd ~/Desktop/tls-mtls/grpccontroller/backend/controller

# Run command

  sudo TRUST_DOMAIN="mycorp.internal" \
    INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
    INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
    ADMIN_AUTH_TOKEN="7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" \
    INTERNAL_API_TOKEN="e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3" \
    ./controller

    sudo \
TRUST_DOMAIN="mycorp.internal" \
INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
ADMIN_AUTH_TOKEN="7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" \
INTERNAL_API_TOKEN="e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3" \
CONTROLLER_ADDR="192.168.1.104:8443" \
ADMIN_HTTP_ADDR="0.0.0.0:8081" \
./controller

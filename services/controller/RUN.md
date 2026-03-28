# file path

cd ~/Desktop/tls-mtls/grpccontroller/backend/controller

# Run command v1

  sudo TRUST_DOMAIN="mycorp.internal" \
    INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
    INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
    ./controller

# Run command v2
    sudo \
      TRUST_DOMAIN="mycorp.internal" \
      INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
      INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
      CONTROLLER_ADDR="192.168.1.213:8443" \
      ADMIN_HTTP_ADDR="0.0.0.0:8081" \
      ./controller

# Run command v3
./run-air.sh

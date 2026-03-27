#!/bin/bash
cd "$(dirname "$0")"

export TRUST_DOMAIN="mycorp.internal"
export INTERNAL_CA_CERT="$(cat ca/ca.crt)"
export INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)"
export ADMIN_AUTH_TOKEN="${ADMIN_AUTH_TOKEN:?Set ADMIN_AUTH_TOKEN env var}"
export INTERNAL_API_TOKEN="${INTERNAL_API_TOKEN:?Set INTERNAL_API_TOKEN env var}"
export CONTROLLER_ADDR="192.168.1.104:8443"
export ADMIN_HTTP_ADDR="0.0.0.0:8081"

exec ./tmp/main "$@"

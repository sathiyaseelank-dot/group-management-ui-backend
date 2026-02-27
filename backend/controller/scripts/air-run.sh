#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

export TRUST_DOMAIN="mycorp.internal"
export INTERNAL_CA_CERT_PATH="ca/ca.crt"
export INTERNAL_CA_KEY_PATH="ca/ca.pkcs8.key"
export ADMIN_AUTH_TOKEN="7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4"
export INTERNAL_API_TOKEN="e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3"
export CONTROLLER_ADDR="192.168.1.104:8443"
export ADMIN_HTTP_ADDR="0.0.0.0:8081"

if [ -f "${ROOT_DIR}/${INTERNAL_CA_CERT_PATH}" ]; then
  INTERNAL_CA_CERT="$(cat "${ROOT_DIR}/${INTERNAL_CA_CERT_PATH}")"
  export INTERNAL_CA_CERT
fi

if [ -f "${ROOT_DIR}/${INTERNAL_CA_KEY_PATH}" ]; then
  INTERNAL_CA_KEY="$(cat "${ROOT_DIR}/${INTERNAL_CA_KEY_PATH}")"
  export INTERNAL_CA_KEY
fi

exec "${ROOT_DIR}/tmp/main"

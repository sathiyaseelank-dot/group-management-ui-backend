#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

export TRUST_DOMAIN="mycorp.internal"
export INTERNAL_CA_CERT_PATH="ca/ca.crt"
export INTERNAL_CA_KEY_PATH="ca/ca.pkcs8.key"
export ADMIN_AUTH_TOKEN="${ADMIN_AUTH_TOKEN:?Set ADMIN_AUTH_TOKEN env var}"
export INTERNAL_API_TOKEN="${INTERNAL_API_TOKEN:?Set INTERNAL_API_TOKEN env var}"
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

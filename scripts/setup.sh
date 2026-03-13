#!/usr/bin/env bash
set -euo pipefail

# Zero-Trust gRPC Connector one-time installer (non-interactive).
# - Installs connector binary
# - Enables and starts systemd service

if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: setup must be run as root." >&2
  exit 1
fi

required_envs=(CONTROLLER_ADDR CONTROLLER_HTTP_ADDR CONNECTOR_ID ENROLLMENT_TOKEN POLICY_SIGNING_KEY)
for var in "${required_envs[@]}"; do
  if [[ -z "${!var:-}" ]]; then
    echo "ERROR: ${var} is required." >&2
    exit 1
  fi
done

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

if [[ "${os}" != "linux" ]]; then
  echo "ERROR: unsupported OS '${os}'. Linux only." >&2
  exit 1
fi

case "${arch}" in
  x86_64|amd64)
    arch="amd64"
    ;;
  aarch64|arm64)
    arch="arm64"
    ;;
  *)
    echo "ERROR: unsupported architecture '${arch}'." >&2
    exit 1
    ;;
esac

binary="connector-${os}-${arch}"
release_url="https://github.com/vairabarath/zero-trust/releases/latest/download/${binary}"
unit_url="https://raw.githubusercontent.com/vairabarath/zero-trust/main/systemd/connector.service"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

echo "Downloading connector binary..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${release_url}" -o "${tmpdir}/connector"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmpdir}/connector" "${release_url}"
else
  echo "ERROR: curl or wget is required for download." >&2
  exit 1
fi

install -m 0755 "${tmpdir}/connector" /usr/bin/connector

config_dir="/etc/connector"
config_file="${config_dir}/connector.conf"
bundled_ca="${config_dir}/ca.crt"
# token_file="${config_dir}/enrollment-token"

mkdir -p "${config_dir}"
chmod 0700 "${config_dir}"

force_overwrite=false
if [[ "${1:-}" == "-f" ]]; then
  force_overwrite=true
fi

if [[ -f "${config_file}" && "${force_overwrite}" != "true" ]]; then
  echo "ERROR: ${config_file} already exists. Use -f to overwrite." >&2
  exit 1
fi

if [[ -f "${config_file}" ]]; then
  ts="$(date +%Y%m%d%H%M%S)"
  cp "${config_file}" "${config_file}.${ts}.bak"
fi

echo "Fetching controller CA from ${CONTROLLER_HTTP_ADDR}..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "http://${CONTROLLER_HTTP_ADDR}/ca.crt" -o "${bundled_ca}"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${bundled_ca}" "http://${CONTROLLER_HTTP_ADDR}/ca.crt"
else
  echo "ERROR: curl or wget is required." >&2
  exit 1
fi
chmod 0644 "${bundled_ca}"

# echo "Installing enrollment token..."
# printf "%s" "${ENROLLMENT_TOKEN}" > "${token_file}"
# chmod 0600 "${token_file}"

{
  echo "CONTROLLER_ADDR=${CONTROLLER_ADDR}"
  echo "CONNECTOR_ID=${CONNECTOR_ID}"
  echo "ENROLLMENT_TOKEN=${ENROLLMENT_TOKEN}"
  echo "POLICY_SIGNING_KEY=${POLICY_SIGNING_KEY}"
  if [[ -n "${CONNECTOR_PRIVATE_IP:-}" ]]; then
    echo "CONNECTOR_PRIVATE_IP=${CONNECTOR_PRIVATE_IP}"
  fi
  if [[ -n "${CONNECTOR_VERSION:-}" ]]; then
    echo "CONNECTOR_VERSION=${CONNECTOR_VERSION}"
  fi
} > "${config_file}"

chmod 0600 "${config_file}"

systemd_dst="/etc/systemd/system/connector.service"

echo "Downloading systemd unit..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${unit_url}" -o "${tmpdir}/connector.service"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmpdir}/connector.service" "${unit_url}"
else
  echo "ERROR: curl or wget is required for download." >&2
  exit 1
fi

install -m 0644 "${tmpdir}/connector.service" "${systemd_dst}"

systemctl daemon-reload
systemctl enable connector.service
systemctl stop connector.service 2>/dev/null || true
# Clear any saved enrollment from the previous install so a new CONNECTOR_ID
# always performs a fresh enrollment instead of reusing an old certificate.
rm -rf /var/lib/private/connector /var/lib/connector /run/connector
systemctl start connector.service

# Unset sensitive env vars.
unset ENROLLMENT_TOKEN

echo "Setup completed."

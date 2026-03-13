#!/usr/bin/env bash
set -euo pipefail

# Zero-Trust gRPC Agent one-time installer (non-interactive).
# - Installs agent binary
# - Enables and starts systemd service

if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: setup must be run as root." >&2
  exit 1
fi

required_envs=(CONTROLLER_ADDR CONTROLLER_HTTP_ADDR CONNECTOR_ADDR AGENT_ID ENROLLMENT_TOKEN)
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

binary="agent-${os}-${arch}"
release_url="https://github.com/vairabarath/zero-trust/releases/latest/download/${binary}"
unit_url="https://raw.githubusercontent.com/vairabarath/zero-trust/main/systemd/agent.service"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

echo "Downloading agent binary..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${release_url}" -o "${tmpdir}/agent"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmpdir}/agent" "${release_url}"
else
  echo "ERROR: curl or wget is required for download." >&2
  exit 1
fi

install -m 0755 "${tmpdir}/agent" /usr/bin/agent

config_dir="/etc/agent"
config_file="${config_dir}/agent.conf"
bundled_ca="${config_dir}/ca.crt"

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

{
  echo "CONTROLLER_ADDR=${CONTROLLER_ADDR}"
  echo "CONNECTOR_ADDR=${CONNECTOR_ADDR}"
  echo "AGENT_ID=${AGENT_ID}"
  echo "ENROLLMENT_TOKEN=${ENROLLMENT_TOKEN}"
  if [[ -n "${TRUST_DOMAIN:-}" ]]; then
    echo "TRUST_DOMAIN=${TRUST_DOMAIN}"
  fi
  if [[ -n "${TUN_NAME:-}" ]]; then
    echo "TUN_NAME=${TUN_NAME}"
  fi
} > "${config_file}"

chmod 0600 "${config_file}"

systemd_dst="/etc/systemd/system/agent.service"

echo "Downloading systemd unit..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${unit_url}" -o "${tmpdir}/agent.service"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmpdir}/agent.service" "${unit_url}"
else
  echo "ERROR: curl or wget is required for download." >&2
  exit 1
fi

install -m 0644 "${tmpdir}/agent.service" "${systemd_dst}"

systemctl daemon-reload
systemctl enable agent.service
systemctl start agent.service

unset ENROLLMENT_TOKEN

echo "Agent setup completed."

#!/usr/bin/env bash
set -euo pipefail

# Zero-Trust gRPC Connector one-time installer (non-interactive).
# - Installs connector binary
# - Enables and starts systemd service

if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: setup must be run as root." >&2
  exit 1
fi

required_envs=(CONTROLLER_ADDR CONNECTOR_ID ENROLLMENT_TOKEN CONTROLLER_CA_PATH)
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

binary="grpcconnector-${os}-${arch}"
release_url="https://github.com/sathiyaseelank-dot/group-management-ui-backend/releases/latest/download/${binary}"
unit_url="https://raw.githubusercontent.com/sathiyaseelank-dot/grpccontroller/main/systemd/grpcconnector.service"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

echo "Downloading connector binary..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${release_url}" -o "${tmpdir}/grpcconnector"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmpdir}/grpcconnector" "${release_url}"
else
  echo "ERROR: curl or wget is required for download." >&2
  exit 1
fi

install -m 0755 "${tmpdir}/grpcconnector" /usr/bin/grpcconnector

config_dir="/etc/grpcconnector"
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

echo "Installing controller CA..."
if [[ -z "${CONTROLLER_CA_PATH}" ]]; then
  echo "ERROR: CONTROLLER_CA_PATH is not set" >&2
  exit 1
fi
if [[ ! -f "${CONTROLLER_CA_PATH}" ]]; then
  echo "ERROR: Controller CA file not found at ${CONTROLLER_CA_PATH}" >&2
  exit 1
fi
install -m 0644 "${CONTROLLER_CA_PATH}" "${bundled_ca}"

# echo "Installing enrollment token..."
# printf "%s" "${ENROLLMENT_TOKEN}" > "${token_file}"
# chmod 0600 "${token_file}"

{
  echo "CONTROLLER_ADDR=${CONTROLLER_ADDR}"
  echo "CONNECTOR_ID=${CONNECTOR_ID}"
  echo "ENROLLMENT_TOKEN=${ENROLLMENT_TOKEN}"
  if [[ -n "${CONNECTOR_PRIVATE_IP:-}" ]]; then
    echo "CONNECTOR_PRIVATE_IP=${CONNECTOR_PRIVATE_IP}"
  fi
  if [[ -n "${CONNECTOR_VERSION:-}" ]]; then
    echo "CONNECTOR_VERSION=${CONNECTOR_VERSION}"
  fi
} > "${config_file}"

chmod 0600 "${config_file}"

systemd_dst="/etc/systemd/system/grpcconnector.service"

echo "Downloading systemd unit..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${unit_url}" -o "${tmpdir}/grpcconnector.service"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmpdir}/grpcconnector.service" "${unit_url}"
else
  echo "ERROR: curl or wget is required for download." >&2
  exit 1
fi

install -m 0644 "${tmpdir}/grpcconnector.service" "${systemd_dst}"

systemctl daemon-reload
systemctl enable grpcconnector.service
systemctl start grpcconnector.service

# Unset sensitive env vars.
unset ENROLLMENT_TOKEN

echo "Setup completed."

#!/usr/bin/env bash
set -euo pipefail

# Zero-Trust gRPC Tunneler one-time installer (non-interactive).
# - Installs tunneler binary
# - Enables and starts systemd service

if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: setup must be run as root." >&2
  exit 1
fi

required_envs=(CONTROLLER_ADDR CONNECTOR_ADDR TUNNELER_ID ENROLLMENT_TOKEN CONTROLLER_CA_PATH)
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

binary="grpctunneler-${os}-${arch}"
release_url="https://github.com/sathiyaseelank-dot/grpccontroller/releases/latest/download/${binary}"
unit_url="https://raw.githubusercontent.com/sathiyaseelank-dot/grpccontroller/main/systemd/grpctunneler.service"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

echo "Downloading tunneler binary..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${release_url}" -o "${tmpdir}/grpctunneler"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmpdir}/grpctunneler" "${release_url}"
else
  echo "ERROR: curl or wget is required for download." >&2
  exit 1
fi

install -m 0755 "${tmpdir}/grpctunneler" /usr/bin/grpctunneler

config_dir="/etc/grpctunneler"
config_file="${config_dir}/tunneler.conf"
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

echo "Installing controller CA..."
if [[ ! -f "${CONTROLLER_CA_PATH}" ]]; then
  echo "ERROR: Controller CA file not found at ${CONTROLLER_CA_PATH}" >&2
  exit 1
fi
install -m 0644 "${CONTROLLER_CA_PATH}" "${bundled_ca}"

{
  echo "CONTROLLER_ADDR=${CONTROLLER_ADDR}"
  echo "CONNECTOR_ADDR=${CONNECTOR_ADDR}"
  echo "TUNNELER_ID=${TUNNELER_ID}"
  echo "ENROLLMENT_TOKEN=${ENROLLMENT_TOKEN}"
  echo "CONTROLLER_CA_PATH=${bundled_ca}"
  if [[ -n "${TRUST_DOMAIN:-}" ]]; then
    echo "TRUST_DOMAIN=${TRUST_DOMAIN}"
  fi
} > "${config_file}"

chmod 0600 "${config_file}"

systemd_dst="/etc/systemd/system/grpctunneler.service"

echo "Downloading systemd unit..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${unit_url}" -o "${tmpdir}/grpctunneler.service"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmpdir}/grpctunneler.service" "${unit_url}"
else
  echo "ERROR: curl or wget is required for download." >&2
  exit 1
fi

install -m 0644 "${tmpdir}/grpctunneler.service" "${systemd_dst}"

systemctl daemon-reload
systemctl enable grpctunneler.service
systemctl start grpctunneler.service

unset ENROLLMENT_TOKEN

echo "Tunneler setup completed."

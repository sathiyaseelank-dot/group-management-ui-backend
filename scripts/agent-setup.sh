#!/usr/bin/env bash
set -euo pipefail

# Zero-Trust gRPC Agent one-time installer (non-interactive).
# - Installs agent binary
# - Enables and starts systemd service

if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: setup must be run as root." >&2
  exit 1
fi

service_group="zero-trust-agent"
service_user="zero-trust-agent"

detect_package_manager() {
  if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
  fi

  if command -v pacman >/dev/null 2>&1; then
    PKG_MANAGER="pacman"
  elif command -v apt-get >/dev/null 2>&1; then
    PKG_MANAGER="apt-get"
  elif command -v dnf >/dev/null 2>&1; then
    PKG_MANAGER="dnf"
  elif command -v yum >/dev/null 2>&1; then
    PKG_MANAGER="yum"
  elif command -v apk >/dev/null 2>&1; then
    PKG_MANAGER="apk"
  elif command -v zypper >/dev/null 2>&1; then
    PKG_MANAGER="zypper"
  else
    PKG_MANAGER=""
  fi
  OS_ID="${ID:-unknown}"
}

install_nftables() {
  case "${PKG_MANAGER}" in
    pacman)
      pacman -Sy --noconfirm nftables
      ;;
    apt-get)
      apt-get update
      apt-get install -y nftables
      ;;
    dnf)
      dnf install -y nftables
      ;;
    yum)
      yum install -y nftables
      ;;
    apk)
      apk add --no-cache nftables
      ;;
    zypper)
      zypper --non-interactive install nftables
      ;;
    *)
      return 1
      ;;
  esac
}

ensure_nft() {
  detect_package_manager
  if command -v nft >/dev/null 2>&1; then
    echo "nft already installed: $(command -v nft)"
    return 0
  fi

  if [[ "$(uname -s)" != "Linux" ]]; then
    echo "ERROR: nftables auto-install is supported only on Linux hosts." >&2
    exit 1
  fi

  if [[ -z "${PKG_MANAGER}" ]]; then
    echo "ERROR: no supported package manager found on Linux host (os=${OS_ID}). Install nftables manually and re-run." >&2
    exit 1
  fi

  echo "Installing nftables via ${PKG_MANAGER} (os=${OS_ID})..."
  install_nftables || {
    echo "ERROR: failed to install nftables via ${PKG_MANAGER}. Install it manually and re-run." >&2
    exit 1
  }

  if ! command -v nft >/dev/null 2>&1; then
    echo "ERROR: nftables installation finished but \`nft\` is still missing. Agent firewall protection will not work." >&2
    exit 1
  fi

  echo "nft installed: $(command -v nft)"
}

resolve_nologin_shell() {
  local candidate
  for candidate in /usr/sbin/nologin /usr/bin/nologin /sbin/nologin /bin/false; do
    if [[ -x "${candidate}" ]]; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  printf '%s\n' "/bin/false"
}

ensure_service_account() {
  local shell_path
  shell_path="$(resolve_nologin_shell)"

  if ! getent group "${service_group}" >/dev/null 2>&1; then
    groupadd --system "${service_group}"
  fi

  if ! id -u "${service_user}" >/dev/null 2>&1; then
    useradd \
      --system \
      --no-create-home \
      --gid "${service_group}" \
      --shell "${shell_path}" \
      "${service_user}"
  fi
}

required_envs=(CONTROLLER_ADDR CONTROLLER_HTTP_ADDR CONNECTOR_ADDR AGENT_ID ENROLLMENT_TOKEN)
for var in "${required_envs[@]}"; do
  if [[ -z "${!var:-}" ]]; then
    echo "ERROR: ${var} is required." >&2
    exit 1
  fi
done

WORKSPACE_SLUG="${WORKSPACE_SLUG:-}"
CONTROLLER_TRUST_DOMAIN="${CONTROLLER_TRUST_DOMAIN:-}"
CONNECTOR_TRUST_DOMAIN="${CONNECTOR_TRUST_DOMAIN:-}"

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

ensure_nft

binary="agent-${os}-${arch}"
release_url="https://github.com/vairabarath/zero-trust/releases/latest/download/${binary}"
unit_url="https://raw.githubusercontent.com/vairabarath/zero-trust/alpha/systemd/agent.service"

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
  if [[ -n "${WORKSPACE_SLUG}" ]]; then
    echo "WORKSPACE_SLUG=${WORKSPACE_SLUG}"
  fi
  if [[ -n "${CONTROLLER_TRUST_DOMAIN}" ]]; then
    echo "CONTROLLER_TRUST_DOMAIN=${CONTROLLER_TRUST_DOMAIN}"
  fi
  if [[ -n "${CONNECTOR_TRUST_DOMAIN}" ]]; then
    echo "CONNECTOR_TRUST_DOMAIN=${CONNECTOR_TRUST_DOMAIN}"
  fi
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

ensure_service_account

systemctl daemon-reload
systemctl enable agent.service
systemctl stop agent.service 2>/dev/null || true
rm -rf /var/lib/private/agent /var/lib/agent /run/agent
systemctl start agent.service

unset ENROLLMENT_TOKEN

echo "Agent setup completed."

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

required_envs=(CONTROLLER_ADDR CONTROLLER_HTTP_ADDR AGENT_ID ENROLLMENT_TOKEN)
for var in "${required_envs[@]}"; do
  if [[ -z "${!var:-}" ]]; then
    echo "ERROR: ${var} is required." >&2
    exit 1
  fi
done

CONNECTOR_ADDR="${CONNECTOR_ADDR:-}"
WORKSPACE_SLUG="${WORKSPACE_SLUG:-}"
CONTROLLER_TRUST_DOMAIN="${CONTROLLER_TRUST_DOMAIN:-}"
CONNECTOR_TRUST_DOMAIN="${CONNECTOR_TRUST_DOMAIN:-}"

# ── Auto-fetch config from controller ───────────────────────────────────────
# Calls the provisioning endpoint to auto-populate trust domains, workspace
# slug, and connector address. Manually-set env vars always take priority.
# Fails silently if the endpoint is unreachable (backward compatible).
parse_json_field() {
  local json="$1" field="$2"
  if command -v jq >/dev/null 2>&1; then
    echo "${json}" | jq -r ".${field} // empty" 2>/dev/null
  elif command -v python3 >/dev/null 2>&1; then
    echo "${json}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('${field}',''))" 2>/dev/null
  else
    echo "${json}" | grep -o "\"${field}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | sed 's/.*:[[:space:]]*"\([^"]*\)"/\1/' 2>/dev/null || true
  fi
}

auto_fetch_config() {
  local http_addr="${CONTROLLER_HTTP_ADDR:-}"
  if [[ -z "${http_addr}" ]]; then
    return 0
  fi

  echo "Fetching configuration from controller..."
  local provision_url="http://${http_addr}/api/provision?token=${ENROLLMENT_TOKEN}&entity_id=${AGENT_ID}"
  local provision_resp=""

  if command -v curl >/dev/null 2>&1; then
    provision_resp="$(curl -fsSL --max-time 10 "${provision_url}" 2>/dev/null)" || true
  elif command -v wget >/dev/null 2>&1; then
    provision_resp="$(wget -qO- --timeout=10 "${provision_url}" 2>/dev/null)" || true
  fi

  if [[ -z "${provision_resp}" ]]; then
    echo "  Could not fetch config from controller — using manually specified values."
    return 0
  fi

  local fetched_controller_td fetched_workspace_td fetched_workspace_slug fetched_connector_addr
  fetched_controller_td="$(parse_json_field "${provision_resp}" controller_trust_domain)"
  fetched_workspace_td="$(parse_json_field "${provision_resp}" workspace_trust_domain)"
  fetched_workspace_slug="$(parse_json_field "${provision_resp}" workspace_slug)"
  fetched_connector_addr="$(parse_json_field "${provision_resp}" connector_addr)"

  if [[ -z "${CONTROLLER_TRUST_DOMAIN}" && -n "${fetched_controller_td}" ]]; then
    CONTROLLER_TRUST_DOMAIN="${fetched_controller_td}"
    echo "  Auto-configured CONTROLLER_TRUST_DOMAIN=${CONTROLLER_TRUST_DOMAIN}"
  fi
  if [[ -z "${CONNECTOR_TRUST_DOMAIN}" && -n "${fetched_workspace_td}" ]]; then
    CONNECTOR_TRUST_DOMAIN="${fetched_workspace_td}"
    echo "  Auto-configured CONNECTOR_TRUST_DOMAIN=${CONNECTOR_TRUST_DOMAIN}"
  fi
  if [[ -z "${WORKSPACE_SLUG}" && -n "${fetched_workspace_slug}" ]]; then
    WORKSPACE_SLUG="${fetched_workspace_slug}"
    echo "  Auto-configured WORKSPACE_SLUG=${WORKSPACE_SLUG}"
  fi
  if [[ -z "${CONNECTOR_ADDR}" && -n "${fetched_connector_addr}" ]]; then
    CONNECTOR_ADDR="${fetched_connector_addr}"
    echo "  Auto-configured CONNECTOR_ADDR=${CONNECTOR_ADDR}"
  fi
}

auto_fetch_config

# Validate CONNECTOR_ADDR after auto-fetch (may have been populated from controller)
if [[ -z "${CONNECTOR_ADDR}" ]]; then
  echo "ERROR: CONNECTOR_ADDR is required (set it manually or ensure the agent is linked to a connector in the dashboard)." >&2
  exit 1
fi

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

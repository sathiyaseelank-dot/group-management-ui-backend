#!/usr/bin/env bash
# docker-test-setup.sh
# Interactive setup for N connector+agent pairs.
# Prompts for number of pairs and network names; generates docker-compose.test.yml
# dynamically and creates all controller resources automatically.
# Run from repo root: ./scripts/docker-test-setup.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[setup]${NC} $*"; }
ok()    { echo -e "${GREEN}[ok]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC}  $*"; }
die()   { echo -e "${RED}[error]${NC} $*" >&2; exit 1; }

CONTROLLER_URL="${CONTROLLER_URL:-http://localhost:8081}"
CONTROLLER_ADDR="${CONTROLLER_ADDR:-localhost:8443}"
ENROLL_WAIT_SEC="${ENROLL_WAIT_SEC:-60}"

# ── Prerequisites ─────────────────────────────────────────────────────────────

info "Checking prerequisites..."
command -v docker   >/dev/null 2>&1 || die "docker not found"
command -v jq       >/dev/null 2>&1 || die "jq not found (pacman -S jq)"
command -v python3  >/dev/null 2>&1 || die "python3 not found"
command -v curl     >/dev/null 2>&1 || die "curl not found"
command -v openssl  >/dev/null 2>&1 || die "openssl not found"
docker info >/dev/null 2>&1 || die "Docker daemon not running"

for bin in \
    services/connector/target/release/connector \
    services/agent/target/release/agent; do
    [ -x "$bin" ] || die "Binary not found: $bin\n  Build with: make build-connector / make build-agent"
done
ok "Prerequisites satisfied"

# ── Load controller .env ──────────────────────────────────────────────────────

info "Loading controller environment..."
ENV_FILE="services/controller/.env"
[ -f "$ENV_FILE" ] || die "$ENV_FILE not found — controller must be configured first"

set +u
while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ -z "${line// }" ]] && continue
    key="${line%%=*}"
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    export "$line" 2>/dev/null || true
done < "$ENV_FILE"
set -u

CONTROLLER_URL="${CONTROLLER_URL:-http://localhost:8081}"
# TRUST_DOMAIN is read from the controller's .env — do not override or hardcode
TRUST_DOMAIN="${TRUST_DOMAIN:-mycorp.internal}"
ok "Environment loaded (TRUST_DOMAIN=${TRUST_DOMAIN})"

# ── Interactive prompts ───────────────────────────────────────────────────────

echo ""
read -rp "How many connector+agent pairs? [2]: " _N
N_PAIRS="${_N:-2}"
[[ "$N_PAIRS" =~ ^[1-9][0-9]*$ ]] || die "Invalid number: $N_PAIRS"

declare -a NET_NAMES
for i in $(seq 1 "$N_PAIRS"); do
    while true; do
        read -rp "Network name for pair ${i}: " _name
        [[ -n "$_name" ]] && break
        echo "  Name cannot be empty."
    done
    NET_NAMES[$i]="$_name"
done
echo ""

# ── Extract / auto-generate CA certificate ────────────────────────────────────

info "Extracting CA certificate..."
mkdir -p docker/env

CA_CERT_PEM=""

if [[ -n "${INTERNAL_CA_CERT:-}" ]]; then
    CA_CERT_PEM=$(python3 -c "
import os, sys
cert = os.environ.get('INTERNAL_CA_CERT','')
cert = cert.replace('\\\\n', '\n').strip()
sys.stdout.write(cert + '\n')
")
    ok "CA cert loaded from INTERNAL_CA_CERT env var"
elif [[ -n "${INTERNAL_CA_CERT_PATH:-}" && -f "${INTERNAL_CA_CERT_PATH}" ]]; then
    CA_CERT_PEM=$(cat "$INTERNAL_CA_CERT_PATH")
    ok "CA cert loaded from $INTERNAL_CA_CERT_PATH"
else
    CA_DIR="$HOME/.ztna/ca"
    mkdir -p "$CA_DIR"

    if [[ ! -f "$CA_DIR/ca.crt" ]]; then
        info "No CA found — generating new internal CA at $CA_DIR ..."
        openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 \
            -out "$CA_DIR/ca.key" 2>/dev/null
        openssl pkcs8 -topk8 -nocrypt \
            -in "$CA_DIR/ca.key" -out "$CA_DIR/ca.pkcs8.key" 2>/dev/null
        openssl req -new -x509 \
            -key "$CA_DIR/ca.pkcs8.key" \
            -out "$CA_DIR/ca.crt" -days 3650 \
            -subj "/CN=ztna-internal-ca/O=ZTNA" \
            -addext "basicConstraints=critical,CA:TRUE" 2>/dev/null
        ok "CA generated: $CA_DIR/ca.crt"
    else
        ok "Reusing existing CA at $CA_DIR/ca.crt"
    fi

    CA_CERT_PEM=$(cat "$CA_DIR/ca.crt")

    # Write file paths (not inline PEM) — safe for Makefile's xargs-based export
    grep -v '^INTERNAL_CA_CERT' "$ENV_FILE" \
        | grep -v '^INTERNAL_CA_KEY' > "$ENV_FILE.tmp"
    echo "INTERNAL_CA_CERT_PATH=$CA_DIR/ca.crt"      >> "$ENV_FILE.tmp"
    echo "INTERNAL_CA_KEY_PATH=$CA_DIR/ca.pkcs8.key" >> "$ENV_FILE.tmp"
    mv "$ENV_FILE.tmp" "$ENV_FILE"
    ok "CA paths written to $ENV_FILE"

    echo ""
    warn "The controller must be restarted to pick up the new CA cert."
    warn "Stop the running controller (Ctrl+C in its terminal), then run:"
    warn "  make dev-controller"
    read -rp "  Press Enter once the controller is back up: " _
    echo ""

    export INTERNAL_CA_CERT_PATH="$CA_DIR/ca.crt"
    export INTERNAL_CA_KEY_PATH="$CA_DIR/ca.pkcs8.key"
fi

echo "$CA_CERT_PEM" > docker/ca.crt
ok "CA cert written to docker/ca.crt"

# ── Generate docker-compose.test.yml dynamically ─────────────────────────────

info "Generating docker-compose.test.yml for ${N_PAIRS} pair(s)..."

{
    # Networks
    echo "networks:"
    for i in $(seq 1 "$N_PAIRS"); do
        printf "  ztna-net-%d:\n" "$i"
        printf "    driver: bridge\n"
        printf "    ipam:\n"
        printf "      config:\n"
        printf "        - subnet: 172.20.%d.0/24\n" "$i"
        echo ""
    done

    # Volumes (one per connector for state persistence)
    echo "volumes:"
    for i in $(seq 1 "$N_PAIRS"); do
        printf "  connector-%d-state:\n" "$i"
    done

    echo ""
    echo "services:"

    for i in $(seq 1 "$N_PAIRS"); do
        GRPC_HOST_PORT=$((18000 + i * 1000 + 443))
        TUNNEL_HOST_PORT=$((18000 + i * 1000 + 444))
        RESOURCE_IP="172.20.${i}.10"

        echo ""
        printf "  # ── Pair %d (%s) ──────────────────────────────────────────────\n" "$i" "${NET_NAMES[$i]}"
        echo ""

        # Connector
        printf "  connector-%d:\n" "$i"
        printf "    build:\n"
        printf "      context: .\n"
        printf "      dockerfile: docker/Dockerfile.connector\n"
        printf "    env_file: docker/env/connector-%d.env\n" "$i"
        printf "    environment:\n"
        printf "      - STATE_DIRECTORY=/var/lib/connector\n"
        printf "      - CONNECTOR_LISTEN_ADDR=0.0.0.0:9443\n"
        printf "      - DEVICE_TUNNEL_ADDR=0.0.0.0:9444\n"
        printf "    volumes:\n"
        printf "      - connector-%d-state:/var/lib/connector\n" "$i"
        printf "      - ./services/connector/target/release/connector:/usr/local/bin/connector:ro\n"
        printf "      - ./docker/ca.crt:/etc/ztna/ca.crt:ro\n"
        printf "    ports:\n"
        printf "      - \"%d:9443\"\n" "$GRPC_HOST_PORT"
        printf "      - \"%d:9444\"\n" "$TUNNEL_HOST_PORT"
        printf "    networks:\n"
        printf "      - ztna-net-%d\n" "$i"
        printf "    extra_hosts:\n"
        printf "      - \"host.docker.internal:host-gateway\"\n"
        printf "    restart: unless-stopped\n"
        echo ""

        # Agent
        printf "  agent-%d:\n" "$i"
        printf "    build:\n"
        printf "      context: .\n"
        printf "      dockerfile: docker/Dockerfile.agent\n"
        printf "    env_file: docker/env/agent-%d.env\n" "$i"
        printf "    environment:\n"
        printf "      - CONNECTOR_ADDR=connector-%d:9443\n" "$i"
        printf "    volumes:\n"
        printf "      - ./services/agent/target/release/agent:/usr/local/bin/agent:ro\n"
        printf "      - ./docker/ca.crt:/etc/ztna/ca.crt:ro\n"
        printf "    networks:\n"
        printf "      - ztna-net-%d\n" "$i"
        printf "    extra_hosts:\n"
        printf "      - \"host.docker.internal:host-gateway\"\n"
        printf "    cap_add:\n"
        printf "      - NET_ADMIN\n"
        printf "    restart: unless-stopped\n"
        printf "    depends_on:\n"
        printf "      - connector-%d\n" "$i"
        echo ""

        # Resource (nginx)
        printf "  resource-%d:\n" "$i"
        printf "    image: nginx:alpine\n"
        printf "    volumes:\n"
        printf "      - ./docker/nginx/resource-%d.html:/usr/share/nginx/html/index.html:ro\n" "$i"
        printf "    networks:\n"
        printf "      ztna-net-%d:\n" "$i"
        printf "        ipv4_address: %s\n" "$RESOURCE_IP"
        printf "    restart: unless-stopped\n"
    done
} > docker-compose.test.yml

ok "docker-compose.test.yml generated"

# ── Generate nginx HTML pages ─────────────────────────────────────────────────

mkdir -p docker/nginx
for i in $(seq 1 "$N_PAIRS"); do
    RESOURCE_IP="172.20.${i}.10"
    cat > "docker/nginx/resource-${i}.html" <<EOF
<!DOCTYPE html>
<html>
<head><title>Resource ${i} — ${NET_NAMES[$i]}</title></head>
<body>
<h1>Resource ${i}</h1>
<p>Network: ${NET_NAMES[$i]}</p>
<p>IP: ${RESOURCE_IP}</p>
</body>
</html>
EOF
done
ok "Nginx HTML pages generated"

# ── Get workspace info from PostgreSQL ───────────────────────────────────────

info "Looking up workspace info from PostgreSQL..."

pg_query() {
    docker exec ztna-postgres psql -U ztnaadmin -d ztna -t -A -c "$1" 2>/dev/null \
        | head -1 | tr -d '[:space:]'
}

WORKSPACE_ID=$(pg_query "SELECT id FROM workspaces LIMIT 1")
WORKSPACE_SLUG=$(pg_query "SELECT slug FROM workspaces LIMIT 1")
WORKSPACE_TRUST_DOMAIN=$(pg_query "SELECT trust_domain FROM workspaces LIMIT 1")

[[ -n "$WORKSPACE_ID" ]]   || die "No workspace found in DB. Create a workspace through the dashboard first."
[[ -n "$WORKSPACE_SLUG" ]] || die "Workspace has no slug."

# Use the workspace's trust domain — overrides the global TRUST_DOMAIN env var.
# These can differ: the workspace CA issues certs with its own trust domain.
[[ -n "$WORKSPACE_TRUST_DOMAIN" ]] && TRUST_DOMAIN="$WORKSPACE_TRUST_DOMAIN"

ok "Workspace: $WORKSPACE_SLUG ($WORKSPACE_ID) trust_domain=$TRUST_DOMAIN"

# ── Generate admin JWT ────────────────────────────────────────────────────────

info "Generating admin JWT..."

export _WS_ID="$WORKSPACE_ID" _WS_SLUG="$WORKSPACE_SLUG"
ADMIN_JWT=$(python3 - <<PYEOF
import hmac, hashlib, base64, json, time, os, sys

jwt_secret_str = os.environ.get('JWT_SECRET', '').strip()
if jwt_secret_str:
    secret = jwt_secret_str.encode('utf-8')
else:
    ca_key = os.environ.get('INTERNAL_CA_KEY', '').strip()
    prefix = b'ztna:controller-jwt:'
    secret = hashlib.sha256(prefix + ca_key.encode('utf-8')).digest()

ws_id   = os.environ.get('_WS_ID', '')
ws_slug = os.environ.get('_WS_SLUG', '')
now = int(time.time())

def b64url(data):
    if isinstance(data, str): data = data.encode('utf-8')
    return base64.urlsafe_b64encode(data).rstrip(b'=').decode()

header  = json.dumps({'alg': 'HS256', 'typ': 'JWT'}, separators=(',', ':'))
payload = json.dumps({
    'sub':   'docker-test-setup@localhost',
    'uid':   'setup-script',
    'wid':   ws_id,
    'wslug': ws_slug,
    'wrole': 'owner',
    'aud':   'admin',
    'jti':   'setup-' + str(now),
    'iss':   'ztna-controller',
    'exp':   now + 3600,
    'iat':   now,
}, separators=(',', ':'))

msg = b64url(header) + '.' + b64url(payload)
sig = hmac.new(secret, msg.encode('utf-8'), hashlib.sha256).digest()
print(msg + '.' + b64url(sig))
PYEOF
)

HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer $ADMIN_JWT" \
    "$CONTROLLER_URL/api/connectors")
[[ "$HTTP_STATUS" == "200" ]] \
    || die "Admin JWT rejected by controller (HTTP $HTTP_STATUS). Check JWT_SECRET in $ENV_FILE"
ok "Admin JWT validated"

# ── Helper: admin API call ────────────────────────────────────────────────────

api() {
    local method="$1" path="$2"; shift 2
    curl -s -X "$method" \
        -H "Authorization: Bearer $ADMIN_JWT" \
        -H "Content-Type: application/json" \
        "$CONTROLLER_URL$path" "$@"
}

# ── Create remote networks ────────────────────────────────────────────────────

info "Creating remote networks..."

declare -a NET_IDS
for i in $(seq 1 "$N_PAIRS"); do
    _name="docker-test-${NET_NAMES[$i]}"
    resp=$(api POST /api/remote-networks -d "{\"name\":\"${_name}\",\"location\":\"OTHER\"}")
    _id=$(echo "$resp" | jq -r '.id // empty')
    [[ -n "$_id" ]] || die "Failed to create network '${_name}': $resp"
    NET_IDS[$i]="$_id"
    ok "Network '${_name}': $_id"
done

# ── Create connectors ─────────────────────────────────────────────────────────

info "Creating connectors..."

declare -a CONN_IDS
for i in $(seq 1 "$N_PAIRS"); do
    api POST /api/connectors \
        -d "{\"name\":\"docker-test-connector-${i}\",\"remoteNetworkId\":\"${NET_IDS[$i]}\"}" >/dev/null
done

for i in $(seq 1 "$N_PAIRS"); do
    _id=$(api GET /api/connectors \
        | jq -r ".[] | select(.name==\"docker-test-connector-${i}\") | .id" | head -1)
    [[ -n "$_id" ]] || die "connector-${i} not found after creation"
    CONN_IDS[$i]="$_id"
    ok "connector-${i}: $_id"
done

# ── Create agents ─────────────────────────────────────────────────────────────

info "Creating agents..."

declare -a AGENT_IDS
for i in $(seq 1 "$N_PAIRS"); do
    api POST /api/agents \
        -d "{\"name\":\"docker-test-agent-${i}\",\"remoteNetworkId\":\"${NET_IDS[$i]}\",\"connectorId\":\"${CONN_IDS[$i]}\"}" >/dev/null
done

for i in $(seq 1 "$N_PAIRS"); do
    _id=$(api GET /api/agents \
        | jq -r ".[] | select(.name==\"docker-test-agent-${i}\") | .id" | head -1)
    [[ -n "$_id" ]] || die "agent-${i} not found after creation"
    AGENT_IDS[$i]="$_id"
    ok "agent-${i}: $_id"
done

# ── Get enrollment tokens ─────────────────────────────────────────────────────

info "Generating enrollment tokens..."

get_token() {
    api POST /api/admin/tokens \
        -d "{\"workspace_id\":\"$WORKSPACE_ID\"}" | jq -r '.token // empty'
}

declare -a TOKEN_CONN TOKEN_AGENT
for i in $(seq 1 "$N_PAIRS"); do
    TOKEN_CONN[$i]=$(get_token)
    TOKEN_AGENT[$i]=$(get_token)
    [[ -n "${TOKEN_CONN[$i]}" ]]  || die "Failed to get enrollment token for connector-${i}"
    [[ -n "${TOKEN_AGENT[$i]}" ]] || die "Failed to get enrollment token for agent-${i}"
done
ok "$((N_PAIRS * 2)) enrollment tokens created"

# ── Write env files ───────────────────────────────────────────────────────────

info "Writing env files to docker/env/ ..."

# Containers cannot reach "localhost" — translate to host.docker.internal
CONTAINER_CTRL_ADDR="${CONTROLLER_ADDR/localhost/host.docker.internal}"
CONTAINER_CTRL_URL="${CONTROLLER_URL/localhost/host.docker.internal}"

for i in $(seq 1 "$N_PAIRS"); do
    TUNNEL_HOST_PORT=$((18000 + i * 1000 + 444))

    cat > "docker/env/connector-${i}.env" <<EOF
CONTROLLER_ADDR=${CONTAINER_CTRL_ADDR}
CONTROLLER_HTTP_URL=${CONTAINER_CTRL_URL}
CONNECTOR_ID=${CONN_IDS[$i]}
ENROLLMENT_TOKEN=${TOKEN_CONN[$i]}
TRUST_DOMAIN=${TRUST_DOMAIN}
CONTROLLER_CA_PATH=/etc/ztna/ca.crt
DEVICE_TUNNEL_ADVERTISE_ADDR=127.0.0.1:${TUNNEL_HOST_PORT}
EOF

    cat > "docker/env/agent-${i}.env" <<EOF
CONTROLLER_ADDR=${CONTAINER_CTRL_ADDR}
AGENT_ID=${AGENT_IDS[$i]}
ENROLLMENT_TOKEN=${TOKEN_AGENT[$i]}
TRUST_DOMAIN=${TRUST_DOMAIN}
CONTROLLER_CA_PATH=/etc/ztna/ca.crt
EOF
done
ok "Env files written"

# ── Build and start containers ────────────────────────────────────────────────

info "Building Docker images..."
docker compose -f docker-compose.test.yml build --quiet

info "Starting containers..."
docker compose -f docker-compose.test.yml up -d

# ── Wait for enrollment ───────────────────────────────────────────────────────

wait_for_status() {
    local kind="$1" id="$2" name="$3"
    local deadline=$((SECONDS + ENROLL_WAIT_SEC))
    printf "[setup] Waiting for %s %s to come online " "$kind" "$name"
    while [ $SECONDS -lt $deadline ]; do
        status=$(api GET "/api/${kind}s/$id" | jq -r ".${kind}.status // empty" 2>/dev/null)
        if [[ "$status" == "online" ]]; then
            echo " online"
            return 0
        fi
        printf "."
        sleep 3
    done
    echo " TIMED OUT"
    warn "$kind $name did not come online within ${ENROLL_WAIT_SEC}s"
    warn "Check logs: docker compose -f docker-compose.test.yml logs $name"
    return 1
}

for i in $(seq 1 "$N_PAIRS"); do
    wait_for_status "connector" "${CONN_IDS[$i]}"  "connector-${i}"
    wait_for_status "agent"     "${AGENT_IDS[$i]}" "agent-${i}"
done
ok "All components online"

# ── Create resources ──────────────────────────────────────────────────────────

info "Creating resources..."

declare -a RES_IDS
for i in $(seq 1 "$N_PAIRS"); do
    RESOURCE_IP="172.20.${i}.10"
    api POST /api/resources -d "{
      \"name\":         \"docker-test-resource-${i}\",
      \"type\":         \"IP\",
      \"address\":      \"${RESOURCE_IP}\",
      \"protocol\":     \"TCP\",
      \"port_from\":    80,
      \"port_to\":      80,
      \"network_id\":   \"${NET_IDS[$i]}\",
      \"connector_id\": \"${CONN_IDS[$i]}\"
    }" >/dev/null

    _id=$(api GET /api/resources \
        | jq -r ".[] | select(.address==\"${RESOURCE_IP}\") | .id" | head -1)
    [[ -n "$_id" ]] || die "resource-${i} not found after creation"
    RES_IDS[$i]="$_id"
    ok "resource-${i}: $_id (${RESOURCE_IP}:80)"

    api PATCH "/api/resources/$_id" -d '{"firewall_status":"protected"}' >/dev/null
done

# ── Create user group ─────────────────────────────────────────────────────────

info "Creating user group..."

api POST /api/groups \
    -d '{"name":"docker-test-group","description":"Auto-created by docker-test-setup.sh"}' >/dev/null

GROUP_ID=$(api GET /api/groups | jq -r '.[] | select(.name=="docker-test-group") | .id' | head -1)
[[ -n "$GROUP_ID" ]] || die "group not found after creation"
ok "Group: $GROUP_ID"

# ── Create access rules ───────────────────────────────────────────────────────

info "Creating access rules..."

for i in $(seq 1 "$N_PAIRS"); do
    api POST /api/access-rules -d "{
      \"resourceId\": \"${RES_IDS[$i]}\",
      \"name\":       \"docker-test-rule-${i}\",
      \"groupIds\":   [\"$GROUP_ID\"],
      \"enabled\":    true
    }" >/dev/null
done
ok "Access rules created"

# ── Add admin user to group ───────────────────────────────────────────────────

info "Looking up admin user..."
USER_ID=$(docker exec ztna-postgres psql -U ztnaadmin -d ztna -t -A \
    -c "SELECT id FROM users WHERE workspace_id = '$WORKSPACE_ID' LIMIT 1" 2>/dev/null \
    | head -1 | tr -d '[:space:]')

if [[ -n "$USER_ID" ]]; then
    api POST "/api/groups/$GROUP_ID/members" \
        -d "{\"memberIds\":[\"$USER_ID\"]}" >/dev/null
    ok "Added user $USER_ID to group"
else
    warn "No users found — add yourself to group '$GROUP_ID' via the dashboard"
fi

# ── Done ──────────────────────────────────────────────────────────────────────

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  Docker test environment ready! (${N_PAIRS} pair(s))${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════${NC}"
echo ""
echo "  Workspace:    $WORKSPACE_SLUG"
echo "  Trust domain: $TRUST_DOMAIN"
echo ""
for i in $(seq 1 "$N_PAIRS"); do
    RESOURCE_IP="172.20.${i}.10"
    TUNNEL_PORT=$((18000 + i * 1000 + 444))
    printf "  Pair %-2d  %-20s  %s:80  tunnel → 127.0.0.1:%d\n" \
        "$i" "(${NET_NAMES[$i]})" "$RESOURCE_IP" "$TUNNEL_PORT"
done
echo ""
echo -e "${CYAN}Test with SOCKS5 (no root required):${NC}"
echo "  ZTNA_MODE=socks5 \\"
echo "  CONTROLLER_URL=$CONTROLLER_URL \\"
echo "  ZTNA_TENANT=$WORKSPACE_SLUG \\"
echo "  SOCKS5_ADDR=127.0.0.1:1080 \\"
echo "  INTERNAL_CA_CERT=\"\$(cat docker/ca.crt)\" \\"
echo "  ./services/ztna-client/target/release/ztna-client"
echo ""
echo "  # Then test each resource:"
for i in $(seq 1 "$N_PAIRS"); do
    RESOURCE_IP="172.20.${i}.10"
    echo "  curl -x socks5h://127.0.0.1:1080 http://${RESOURCE_IP}/   # pair ${i}: ${NET_NAMES[$i]}"
done
echo ""
echo -e "${CYAN}Container logs:${NC}"
echo "  docker compose -f docker-compose.test.yml logs -f connector-1"
echo ""
echo -e "${CYAN}Teardown:${NC}"
echo "  ./scripts/docker-test-destroy.sh"
echo ""

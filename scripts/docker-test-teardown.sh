#!/usr/bin/env bash
# docker-test-teardown.sh
# Stops containers, removes volumes and generated env files.
# Run from repo root: ./scripts/docker-test-teardown.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

CYAN='\033[0;36m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info() { echo -e "${CYAN}[teardown]${NC} $*"; }
ok()   { echo -e "${GREEN}[ok]${NC}      $*"; }
warn() { echo -e "${YELLOW}[warn]${NC}    $*"; }

# ── Stop and remove containers + volumes ──────────────────────────────────────

info "Stopping containers and removing volumes..."
docker compose -f docker-compose.test.yml down -v 2>/dev/null && ok "Containers stopped" \
    || warn "Compose down returned non-zero (containers may not have been running)"

# ── Remove generated env files and CA cert ────────────────────────────────────

info "Removing generated env files..."
rm -f docker/env/connector-1.env \
      docker/env/connector-2.env \
      docker/env/agent-1.env \
      docker/env/agent-2.env \
      docker/ca.crt
ok "Env files removed"

# ── Optionally clean up controller resources ──────────────────────────────────

if [[ "${CLEAN_CONTROLLER:-0}" == "1" ]]; then
    info "Removing docker-test-* resources from controller DB..."
    docker exec ztna-postgres psql -U ztnaadmin -d ztna -c "
        DELETE FROM access_rules    WHERE name LIKE 'docker-test-%';
        DELETE FROM access_rule_groups WHERE rule_id NOT IN (SELECT id FROM access_rules);
        DELETE FROM resources       WHERE name LIKE 'docker-test-%';
        DELETE FROM user_groups     WHERE name = 'docker-test-group';
        DELETE FROM agents          WHERE name LIKE 'docker-test-%';
        DELETE FROM connectors      WHERE name LIKE 'docker-test-%';
        DELETE FROM remote_networks WHERE name LIKE 'docker-test-%';
    " 2>/dev/null && ok "Controller resources cleaned" \
        || warn "DB cleanup skipped (controller may not be running)"
fi

echo ""
echo -e "${GREEN}Teardown complete.${NC}"
[[ "${CLEAN_CONTROLLER:-0}" != "1" ]] && \
    echo "  Run with CLEAN_CONTROLLER=1 to also delete docker-test-* objects from the controller DB."
echo ""

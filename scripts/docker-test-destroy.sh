#!/usr/bin/env bash
# docker-test-destroy.sh
# Stops and removes all running containers except PostgreSQL.
# Run from repo root: ./scripts/docker-test-destroy.sh
set -euo pipefail

CYAN='\033[0;36m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info() { echo -e "${CYAN}[destroy]${NC} $*"; }
ok()   { echo -e "${GREEN}[ok]${NC}     $*"; }
warn() { echo -e "${YELLOW}[warn]${NC}   $*"; }

if [[ $EUID -ne 0 ]]; then
    exec sudo "$0" "$@"
fi

# Get all running containers except postgres
TARGETS=$(docker ps --format '{{.ID}} {{.Image}}' \
    | grep -v 'postgres' \
    | awk '{print $1}')

if [[ -z "$TARGETS" ]]; then
    warn "No containers to destroy (postgres excluded)"
    exit 0
fi

info "Containers to destroy:"
docker ps --format '  {{.Names}} ({{.Image}})' | grep -v postgres

echo ""
docker rm -f $TARGETS
ok "Done"

# Prune only unused test networks (never touch postgres data volume)
docker network prune -f >/dev/null
ok "Networks pruned"

echo ""
echo -e "${GREEN}Destroy complete.${NC}"
echo ""

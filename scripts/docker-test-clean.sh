#!/usr/bin/env bash
# docker-test-clean.sh
# Removes all generated artifacts from a docker test run:
#   - docker/env/*.env files
#   - docker/ca.crt
#   - ~/.ztna/ca/ (generated CA outside the project)
#   - INTERNAL_CA_CERT* / INTERNAL_CA_KEY* lines from services/controller/.env
# Does NOT stop containers — run docker-test-destroy.sh for that.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

CYAN='\033[0;36m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info() { echo -e "${CYAN}[clean]${NC} $*"; }
ok()   { echo -e "${GREEN}[ok]${NC}   $*"; }
warn() { echo -e "${YELLOW}[warn]${NC} $*"; }

# ── Docker env files and CA cert ─────────────────────────────────────────────

info "Removing generated docker env files..."
rm -f docker/env/*.env
ok "docker/env/*.env removed"

info "Removing docker/ca.crt..."
rm -f docker/ca.crt
ok "docker/ca.crt removed"

# ── External CA directory ─────────────────────────────────────────────────────

CA_DIR="$HOME/.ztna/ca"
if [[ -d "$CA_DIR" ]]; then
    info "Removing CA directory $CA_DIR ..."
    rm -rf "$CA_DIR"
    ok "$CA_DIR removed"
else
    warn "$CA_DIR not found — skipping"
fi

# ── Strip CA entries from controller .env ─────────────────────────────────────

ENV_FILE="services/controller/.env"
if [[ -f "$ENV_FILE" ]]; then
    info "Stripping INTERNAL_CA_CERT* / INTERNAL_CA_KEY* from $ENV_FILE ..."
    grep -v '^INTERNAL_CA_CERT' "$ENV_FILE" \
        | grep -v '^INTERNAL_CA_KEY' > "$ENV_FILE.tmp"
    mv "$ENV_FILE.tmp" "$ENV_FILE"
    ok "CA entries removed from $ENV_FILE"
else
    warn "$ENV_FILE not found — skipping"
fi

echo ""
echo -e "${GREEN}Clean complete.${NC}"
echo ""

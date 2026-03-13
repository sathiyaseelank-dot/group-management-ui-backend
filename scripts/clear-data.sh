#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Zero-Trust data reset script
# Clears all database data: controller SQLite, frontend SQLite,
# and connector/agent enrollment state.
#
# Usage:
#   ./clear-data.sh              # interactive (prompts for each section)
#   ./clear-data.sh --all        # clear everything without prompts
#   ./clear-data.sh --controller # controller DB only
#   ./clear-data.sh --frontend   # frontend DB only
#   ./clear-data.sh --enrollment # connector + agent enrollment state only
# ─────────────────────────────────────────────────────────────────────────────

REPO_DIR="${REPO_DIR:-/home/bairava/zero-trust}"

CONTROLLER_DB="${REPO_DIR}/services/controller/controller.db"
FRONTEND_DB="${REPO_DIR}/apps/frontend/ztna.db"
CONNECTOR_STATE_DIR="/var/lib/connector"
AGENT_STATE_DIR="/var/lib/agent"

# ── Helpers ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
ok()   { echo -e "  ${GREEN}[OK]${NC}  $*"; }
warn() { echo -e "  ${YELLOW}[WARN]${NC} $*"; }
info() { echo -e "  ${CYAN}[INFO]${NC} $*"; }
err()  { echo -e "  ${RED}[ERR]${NC}  $*" >&2; }

confirm() {
    local msg="$1"
    read -rp "  ${msg} [y/N]: " answer
    [[ "${answer,,}" == "y" ]]
}

# ── Argument parsing ──────────────────────────────────────────────────────────
MODE="${1:-interactive}"

DO_CONTROLLER=false
DO_FRONTEND=false
DO_ENROLLMENT=false

case "$MODE" in
    --all)
        DO_CONTROLLER=true
        DO_FRONTEND=true
        DO_ENROLLMENT=true
        ;;
    --controller)
        DO_CONTROLLER=true
        ;;
    --frontend)
        DO_FRONTEND=true
        ;;
    --enrollment)
        DO_ENROLLMENT=true
        ;;
    interactive)
        : # handled below
        ;;
    *)
        err "Unknown option: $MODE"
        echo "  Usage: $0 [--all | --controller | --frontend | --enrollment]"
        exit 1
        ;;
esac

echo ""
echo "════════════════════════════════════════════════════"
echo "  Zero-Trust Data Reset"
echo "════════════════════════════════════════════════════"
echo ""
info "Repo: ${REPO_DIR}"
echo ""

# ── Interactive prompts ───────────────────────────────────────────────────────
if [[ "$MODE" == "interactive" ]]; then
    echo "  Select what to clear:"
    echo ""
    confirm "Clear controller database (connectors, agents, tokens, audit logs, users)?" && DO_CONTROLLER=true
    confirm "Clear frontend database (demo users, groups, resources, access rules)?" && DO_FRONTEND=true
    confirm "Clear connector + agent enrollment state (forces re-enrollment on next start)?" && DO_ENROLLMENT=true
    echo ""
fi

# ── Stop services if clearing enrollment state ────────────────────────────────
if [[ "$DO_ENROLLMENT" == "true" ]]; then
    echo "── Stopping services ────────────────────────────────"
    if systemctl is-active --quiet connector.service 2>/dev/null; then
        systemctl stop connector.service
        ok "connector.service stopped"
    else
        warn "connector.service not running"
    fi
    if systemctl is-active --quiet agent.service 2>/dev/null; then
        systemctl stop agent.service
        ok "agent.service stopped"
    else
        warn "agent.service not running"
    fi
    echo ""
fi

# ── Clear controller DB ───────────────────────────────────────────────────────
if [[ "$DO_CONTROLLER" == "true" ]]; then
    echo "── Controller Database ──────────────────────────────"
    if [[ -f "$CONTROLLER_DB" ]]; then
        rm -f "$CONTROLLER_DB"
        ok "Deleted: ${CONTROLLER_DB}"
    else
        warn "Not found: ${CONTROLLER_DB}"
    fi
    # Also remove WAL/SHM journal files if present
    rm -f "${CONTROLLER_DB}-wal" "${CONTROLLER_DB}-shm" 2>/dev/null || true
    echo ""
fi

# ── Clear frontend DB ─────────────────────────────────────────────────────────
if [[ "$DO_FRONTEND" == "true" ]]; then
    echo "── Frontend Database ────────────────────────────────"
    if [[ -f "$FRONTEND_DB" ]]; then
        rm -f "$FRONTEND_DB"
        ok "Deleted: ${FRONTEND_DB}"
    else
        warn "Not found: ${FRONTEND_DB}"
    fi
    rm -f "${FRONTEND_DB}-wal" "${FRONTEND_DB}-shm" 2>/dev/null || true
    echo ""
fi

# ── Clear enrollment state ────────────────────────────────────────────────────
if [[ "$DO_ENROLLMENT" == "true" ]]; then
    echo "── Connector Enrollment State ───────────────────────"
    if [[ -d "$CONNECTOR_STATE_DIR" ]]; then
        rm -f "${CONNECTOR_STATE_DIR}"/*.json "${CONNECTOR_STATE_DIR}"/*.pem "${CONNECTOR_STATE_DIR}"/*.der 2>/dev/null || true
        ok "Cleared: ${CONNECTOR_STATE_DIR}"
    else
        warn "Not found: ${CONNECTOR_STATE_DIR}"
    fi
    echo ""

    echo "── Agent Enrollment State ───────────────────────────"
    if [[ -d "$AGENT_STATE_DIR" ]]; then
        rm -f "${AGENT_STATE_DIR}"/*.json "${AGENT_STATE_DIR}"/*.pem "${AGENT_STATE_DIR}"/*.der 2>/dev/null || true
        ok "Cleared: ${AGENT_STATE_DIR}"
    else
        warn "Not found: ${AGENT_STATE_DIR}"
    fi
    echo ""
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo "════════════════════════════════════════════════════"
echo "  Done!"
echo "════════════════════════════════════════════════════"
echo ""

if [[ "$DO_ENROLLMENT" == "true" ]]; then
    echo "  Re-enrollment required. Run install.sh to re-enroll:"
    echo "    sudo ./install.sh"
    echo ""
fi

if [[ "$DO_FRONTEND" == "true" ]]; then
    echo "  Frontend DB will be reseeded with demo data on next start."
    echo "  Also clear localStorage in the browser:"
    echo "    DevTools → Console → localStorage.clear()"
    echo ""
fi

if [[ "$DO_CONTROLLER" == "true" ]]; then
    echo "  Restart the controller to recreate the database schema:"
    echo "    make dev-controller"
    echo ""
fi

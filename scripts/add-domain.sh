#!/usr/bin/env bash
set -euo pipefail

# ── Dubly Add Domain Script ─────────────────────────────────────────
# Adds a new domain to an existing Dubly installation by updating the
# .env file, Caddyfile, and restarting services.
# Usage: sudo bash scripts/add-domain.sh <domain>
# ─────────────────────────────────────────────────────────────────────

INSTALL_DIR="/opt/dubly"

# ── Colors ────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; }

# ── Usage ─────────────────────────────────────────────────────────────

if [ $# -lt 1 ] || [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
  echo "Usage: sudo bash scripts/add-domain.sh <domain>"
  echo ""
  echo "  Adds a domain to the Dubly .env and Caddyfile, then restarts services."
  echo ""
  echo "  Example: sudo bash scripts/add-domain.sh short.example.com"
  exit 0
fi

DOMAIN="$1"

# ── Root check ────────────────────────────────────────────────────────

if [ "$(id -u)" -ne 0 ]; then
  error "This script must be run as root (use sudo)."
  exit 1
fi

# ── Validate domain ──────────────────────────────────────────────────

if [[ "$DOMAIN" == *" "* ]]; then
  error "Domain cannot contain spaces."
  exit 1
fi

if [[ "$DOMAIN" != *.* ]]; then
  error "Domain must contain at least one dot (e.g. short.example.com)."
  exit 1
fi

# ── Check .env exists ────────────────────────────────────────────────

ENV_FILE="$INSTALL_DIR/.env"

if [ ! -f "$ENV_FILE" ]; then
  error "No .env file found at $ENV_FILE"
  error "Is Dubly installed? Run scripts/install.sh first."
  exit 1
fi

# ── Check for duplicate ─────────────────────────────────────────────

CURRENT_DOMAINS="$(grep '^DUBLY_DOMAINS=' "$ENV_FILE" | cut -d= -f2-)"

IFS=',' read -ra DOMAIN_LIST <<< "$CURRENT_DOMAINS"
for d in "${DOMAIN_LIST[@]}"; do
  d="$(echo "$d" | xargs)"
  if [ "$d" = "$DOMAIN" ]; then
    error "Domain '$DOMAIN' is already in DUBLY_DOMAINS."
    exit 1
  fi
done

# ── Update .env ──────────────────────────────────────────────────────

info "Adding $DOMAIN to $ENV_FILE..."
sed -i "s|^DUBLY_DOMAINS=.*|&,$DOMAIN|" "$ENV_FILE"
ok "Updated DUBLY_DOMAINS in $ENV_FILE"

# ── Update Caddyfile ─────────────────────────────────────────────────

CADDYFILE="/etc/caddy/Caddyfile"

if [ ! -f "$CADDYFILE" ]; then
  warn "No Caddyfile found at $CADDYFILE — skipping Caddy update."
  warn "You will need to add the domain to your Caddyfile manually."
else
  info "Adding $DOMAIN to $CADDYFILE..."
  # The Caddyfile has format: "domain1 domain2 {" on the first non-comment line.
  # Prepend the new domain to that line.
  sed -i "0,/^[^#].*{/s|{|$DOMAIN {|" "$CADDYFILE"
  ok "Updated $CADDYFILE"
fi

# ── Reload services ──────────────────────────────────────────────────

info "Restarting services..."
systemctl restart dubly caddy
ok "Services restarted"

# ── Done ─────────────────────────────────────────────────────────────

echo ""
ok "Domain ${BOLD}$DOMAIN${NC} added successfully!"
echo ""
echo -e "  ${YELLOW}Reminder:${NC} Point an A record for ${BOLD}$DOMAIN${NC} to this server's IP."
echo "  You can verify DNS on the Domains page in the admin panel."
echo ""

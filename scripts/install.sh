#!/usr/bin/env bash
set -euo pipefail

# ── Dubly Install Script ─────────────────────────────────────────────
# Sets up a fresh Ubuntu/Debian server with Dubly, Caddy, and optional
# Litestream S3 backups. Safe to re-run (idempotent).
# Usage: sudo bash scripts/install.sh [--update] [--help]
# ─────────────────────────────────────────────────────────────────────

INSTALL_DIR="/opt/dubly"
REPO_URL="https://github.com/scmmishra/dubly.git"
GO_VERSION="1.24.0"
LITESTREAM_VERSION="0.3.13"

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
header(){ echo -e "\n${BOLD}=== $* ===${NC}\n"; }

# ── Cleanup trap ──────────────────────────────────────────────────────

INSTALL_SUCCESS=false
cleanup() {
  if [ "$INSTALL_SUCCESS" = false ]; then
    echo ""
    error "Installation failed. You can safely re-run this script."
  fi
}
trap cleanup EXIT

# ── Flags ─────────────────────────────────────────────────────────────

UPDATE_MODE=false

for arg in "$@"; do
  case "$arg" in
    --update) UPDATE_MODE=true ;;
    --help|-h)
      echo "Usage: sudo bash scripts/install.sh [--update] [--help]"
      echo ""
      echo "  --update   Quick update: git pull, rebuild, restart service"
      echo "  --help     Show this help message"
      INSTALL_SUCCESS=true
      exit 0
      ;;
    *)
      error "Unknown flag: $arg"
      echo "Usage: sudo bash scripts/install.sh [--update] [--help]"
      exit 1
      ;;
  esac
done

# ── Root & OS checks ─────────────────────────────────────────────────

check_root() {
  if [ "$(id -u)" -ne 0 ]; then
    error "This script must be run as root (use sudo)."
    exit 1
  fi
}

check_os() {
  if [ ! -f /etc/os-release ]; then
    error "Cannot detect OS. This script requires Ubuntu or Debian."
    exit 1
  fi
  . /etc/os-release
  case "$ID" in
    ubuntu|debian) ok "Detected $PRETTY_NAME" ;;
    *)
      error "Unsupported OS: $ID. This script requires Ubuntu or Debian."
      exit 1
      ;;
  esac
}

check_root
check_os

# ── Update mode ───────────────────────────────────────────────────────

if [ "$UPDATE_MODE" = true ]; then
  header "Updating Dubly"

  if [ ! -d "$INSTALL_DIR/.git" ]; then
    error "No Dubly installation found at $INSTALL_DIR."
    error "Run without --update for a fresh install."
    exit 1
  fi

  info "Pulling latest changes..."
  git -C "$INSTALL_DIR" pull origin main

  info "Rebuilding binary..."
  cd "$INSTALL_DIR"
  /usr/local/go/bin/go build -o dubly ./cmd/server

  info "Restarting service..."
  systemctl restart dubly

  echo ""
  ok "Dubly updated successfully!"
  systemctl --no-pager status dubly
  INSTALL_SUCCESS=true
  exit 0
fi

# ── Prompt helpers ────────────────────────────────────────────────────

prompt() {
  local var_name="$1" prompt_text="$2" default="${3:-}"
  local value

  if [ -n "$default" ]; then
    read -rp "  $prompt_text [$default]: " value </dev/tty
    value="${value:-$default}"
  else
    while true; do
      read -rp "  $prompt_text: " value </dev/tty
      [ -n "$value" ] && break
      warn "This field is required."
    done
  fi

  eval "$var_name=\"\$value\""
}

prompt_secret() {
  local var_name="$1" prompt_text="$2"
  local value

  while true; do
    read -rsp "  $prompt_text: " value </dev/tty
    echo ""
    [ -n "$value" ] && break
    warn "This field is required."
  done

  eval "$var_name=\"\$value\""
}

prompt_yn() {
  local prompt_text="$1" default="${2:-n}"
  local value

  read -rp "  $prompt_text " value </dev/tty
  value="${value:-$default}"
  [[ "$value" =~ ^[Yy] ]]
}

# ── Collect inputs ────────────────────────────────────────────────────

collect_inputs() {
  header "Configuration"

  # Check if .env already exists
  if [ -f "$INSTALL_DIR/.env" ]; then
    warn "Existing .env found at $INSTALL_DIR/.env"
    if ! prompt_yn "Overwrite configuration? (y/N)" "n"; then
      info "Keeping existing .env. Skipping prompts."
      SKIP_PROMPTS=true
      # Read existing values for Caddyfile/systemd generation
      set -a
      source "$INSTALL_DIR/.env"
      set +a
      INPUT_DOMAINS="$DUBLY_DOMAINS"
      S3_ENABLED=false
      [ -n "${LITESTREAM_S3_BUCKET:-}" ] && S3_ENABLED=true
      return
    fi
  fi

  SKIP_PROMPTS=false

  echo -e "  ${BOLD}General${NC}"
  prompt INPUT_APP_NAME "App name" "Dubly"
  prompt INPUT_ADMIN_DOMAIN "Admin domain (e.g. dubly.chatwoot.dev)"
  prompt INPUT_REDIRECT_DOMAINS "Redirect domains (comma-separated)" "$INPUT_ADMIN_DOMAIN"

  # Deduplicate domains: admin + redirect
  INPUT_DOMAINS="$INPUT_ADMIN_DOMAIN"
  IFS=',' read -ra _redirect_parts <<< "$INPUT_REDIRECT_DOMAINS"
  for d in "${_redirect_parts[@]}"; do
    d="$(echo "$d" | xargs)" # trim whitespace
    [ -z "$d" ] && continue
    if [ "$d" != "$INPUT_ADMIN_DOMAIN" ]; then
      INPUT_DOMAINS="$INPUT_DOMAINS,$d"
    fi
  done

  echo ""
  echo -e "  ${BOLD}Authentication${NC}"
  read -rsp "  API password (blank = auto-generate): " INPUT_PASSWORD </dev/tty
  echo ""
  if [ -z "$INPUT_PASSWORD" ]; then
    INPUT_PASSWORD="$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)"
    PASSWORD_AUTO=true
    echo ""
    warn "Auto-generated password: ${BOLD}$INPUT_PASSWORD${NC}"
    warn "Save this now!"
    echo ""
  else
    PASSWORD_AUTO=false
  fi

  echo ""
  echo -e "  ${BOLD}S3 Backups${NC}"
  warn "S3 backups are strongly recommended for production."
  if prompt_yn "Configure S3 backups? (y/N)" "n"; then
    S3_ENABLED=true
    prompt INPUT_S3_BUCKET "S3 bucket name"
    prompt INPUT_S3_ENDPOINT "S3 endpoint (e.g. https://s3.amazonaws.com)"
    prompt INPUT_S3_REGION "S3 region" "us-east-1"
    prompt_secret INPUT_S3_ACCESS_KEY "S3 access key"
    prompt_secret INPUT_S3_SECRET_KEY "S3 secret key"
  else
    S3_ENABLED=false
  fi

  echo ""
  echo -e "  ${BOLD}GeoIP (optional)${NC}"
  info "Free MaxMind license: https://www.maxmind.com/en/geolite2/signup"
  read -rp "  MaxMind license key (blank to skip): " INPUT_GEOIP_KEY </dev/tty
  INPUT_GEOIP_KEY="${INPUT_GEOIP_KEY:-}"
}

# ── Summary ───────────────────────────────────────────────────────────

show_summary() {
  if [ "$SKIP_PROMPTS" = true ]; then
    info "Using existing configuration from $INSTALL_DIR/.env"
    return
  fi

  header "Summary"
  echo "  App name:          $INPUT_APP_NAME"
  echo "  Domains:           $INPUT_DOMAINS"
  echo "  Password:          ${INPUT_PASSWORD:0:4}****"
  echo "  S3 backups:        $([ "$S3_ENABLED" = true ] && echo "Yes ($INPUT_S3_BUCKET)" || echo "No")"
  echo "  GeoIP:             $([ -n "$INPUT_GEOIP_KEY" ] && echo "Yes" || echo "No")"
  echo ""

  if ! prompt_yn "Proceed? [Y/n]" "y"; then
    info "Aborted."
    INSTALL_SUCCESS=true
    exit 0
  fi
}

collect_inputs
show_summary

# ── System dependencies ───────────────────────────────────────────────

header "Installing System Dependencies"

info "Updating apt package list..."
apt-get update -qq

for pkg in git curl ufw; do
  if dpkg -s "$pkg" &>/dev/null; then
    ok "$pkg already installed"
  else
    info "Installing $pkg..."
    apt-get install -y -qq "$pkg"
    ok "$pkg installed"
  fi
done

# ── Go ────────────────────────────────────────────────────────────────

header "Installing Go"

if command -v /usr/local/go/bin/go &>/dev/null; then
  CURRENT_GO="$(/usr/local/go/bin/go version | awk '{print $3}' | sed 's/go//')"
  ok "Go $CURRENT_GO already installed, skipping"
else
  ARCH="$(dpkg --print-architecture)"
  case "$ARCH" in
    amd64) GO_ARCH="amd64" ;;
    arm64) GO_ARCH="arm64" ;;
    *)
      error "Unsupported architecture: $ARCH"
      exit 1
      ;;
  esac

  GO_TAR="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  info "Downloading Go $GO_VERSION ($GO_ARCH)..."
  curl -fsSL "https://go.dev/dl/$GO_TAR" -o "/tmp/$GO_TAR"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "/tmp/$GO_TAR"
  rm -f "/tmp/$GO_TAR"
  ok "Go $GO_VERSION installed to /usr/local/go"
fi

export PATH="/usr/local/go/bin:$PATH"

# ── Litestream ────────────────────────────────────────────────────────

header "Installing Litestream"

if command -v litestream &>/dev/null; then
  ok "Litestream already installed, skipping"
else
  ARCH="$(dpkg --print-architecture)"
  LITESTREAM_DEB="litestream-v${LITESTREAM_VERSION}-linux-${ARCH}.deb"
  info "Downloading Litestream $LITESTREAM_VERSION..."
  curl -fsSL "https://github.com/benbjohnson/litestream/releases/download/v${LITESTREAM_VERSION}/${LITESTREAM_DEB}" -o "/tmp/${LITESTREAM_DEB}"
  dpkg -i "/tmp/${LITESTREAM_DEB}"
  rm -f "/tmp/${LITESTREAM_DEB}"
  ok "Litestream $LITESTREAM_VERSION installed"
fi

# ── Caddy ─────────────────────────────────────────────────────────────

header "Installing Caddy"

if command -v caddy &>/dev/null; then
  ok "Caddy already installed, skipping"
else
  info "Adding Caddy apt repository..."
  apt-get install -y -qq debian-keyring debian-archive-keyring apt-transport-https
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list
  apt-get update -qq
  apt-get install -y -qq caddy
  ok "Caddy installed"
fi

# ── Clone / pull repo ────────────────────────────────────────────────

header "Setting Up Dubly Repository"

if [ -d "$INSTALL_DIR/.git" ]; then
  info "Repository exists, pulling latest..."
  git -C "$INSTALL_DIR" pull origin main
  ok "Repository updated"
else
  info "Cloning repository to $INSTALL_DIR..."
  git clone "$REPO_URL" "$INSTALL_DIR"
  ok "Repository cloned"
fi

# ── Build binary ──────────────────────────────────────────────────────

header "Building Dubly"

cd "$INSTALL_DIR"
info "Building binary..."
/usr/local/go/bin/go build -o dubly ./cmd/server
ok "Binary built: $INSTALL_DIR/dubly"

# ── GeoIP database ───────────────────────────────────────────────────

if [ "${SKIP_PROMPTS:-false}" = false ] && [ -n "${INPUT_GEOIP_KEY:-}" ]; then
  header "Downloading GeoIP Database"

  GEOIP_PATH="$INSTALL_DIR/GeoLite2-City.mmdb"
  if [ -f "$GEOIP_PATH" ]; then
    ok "GeoIP database already exists, skipping"
  else
    info "Downloading MaxMind GeoLite2-City..."
    curl -fsSL -o /tmp/geoip.tar.gz -G \
      --data-urlencode "edition_id=GeoLite2-City" \
      --data-urlencode "license_key=${INPUT_GEOIP_KEY}" \
      --data-urlencode "suffix=tar.gz" \
      "https://download.maxmind.com/app/geoip_download"
    tar -xzf /tmp/geoip.tar.gz -C /tmp --wildcards '*.mmdb'
    mv /tmp/GeoLite2-City_*/GeoLite2-City.mmdb "$GEOIP_PATH"
    rm -rf /tmp/GeoLite2-City_* /tmp/geoip.tar.gz
    ok "GeoIP database downloaded to $GEOIP_PATH"
  fi
fi

# ── Write .env file ──────────────────────────────────────────────────

if [ "${SKIP_PROMPTS:-false}" = false ]; then
  header "Writing Environment File"

  ENV_FILE="$INSTALL_DIR/.env"

  {
    echo "DUBLY_PASSWORD=$INPUT_PASSWORD"
    echo "DUBLY_DOMAINS=$INPUT_DOMAINS"
    echo "DUBLY_PORT=8080"
    echo "DUBLY_DB_PATH=./dubly.db"
    echo "DUBLY_APP_NAME=$INPUT_APP_NAME"

    if [ -n "${INPUT_GEOIP_KEY:-}" ]; then
      echo "DUBLY_GEOIP_PATH=./GeoLite2-City.mmdb"
    fi

    if [ "$S3_ENABLED" = true ]; then
      echo ""
      echo "# Litestream S3 backups"
      echo "LITESTREAM_S3_BUCKET=$INPUT_S3_BUCKET"
      echo "LITESTREAM_S3_ENDPOINT=$INPUT_S3_ENDPOINT"
      echo "LITESTREAM_S3_REGION=$INPUT_S3_REGION"
      echo "LITESTREAM_ACCESS_KEY_ID=$INPUT_S3_ACCESS_KEY"
      echo "LITESTREAM_SECRET_ACCESS_KEY=$INPUT_S3_SECRET_KEY"
    fi
  } > "$ENV_FILE"

  chmod 600 "$ENV_FILE"
  ok "Environment file written to $ENV_FILE (mode 600)"
fi

# ── Systemd unit ──────────────────────────────────────────────────────

header "Writing Systemd Service"

# Determine ExecStart based on S3 config
if [ "${S3_ENABLED:-false}" = true ]; then
  EXEC_START="/usr/bin/bash /opt/dubly/scripts/start.sh"
else
  EXEC_START="/opt/dubly/dubly"
fi

cat > /etc/systemd/system/dubly.service <<EOF
[Unit]
Description=Dubly URL Shortener
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/dubly
EnvironmentFile=/opt/dubly/.env
Environment="PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
ExecStart=$EXEC_START
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/dubly

[Install]
WantedBy=multi-user.target
EOF

ok "Systemd unit written to /etc/systemd/system/dubly.service"

# ── Caddyfile ─────────────────────────────────────────────────────────

header "Writing Caddyfile"

CADDYFILE="/etc/caddy/Caddyfile"

# Warn if existing non-Dubly Caddyfile
if [ -f "$CADDYFILE" ] && ! grep -q "Dubly" "$CADDYFILE" 2>/dev/null; then
  warn "Existing Caddyfile found that was not created by this script."
  if ! prompt_yn "Overwrite $CADDYFILE? (y/N)" "n"; then
    warn "Skipping Caddyfile. You'll need to configure Caddy manually."
    SKIP_CADDY=true
  fi
fi

if [ "${SKIP_CADDY:-false}" = false ]; then
  # Build domain list from INPUT_DOMAINS (comma-separated → space-separated)
  CADDY_DOMAINS="${INPUT_DOMAINS//,/ }"

  cat > "$CADDYFILE" <<EOF
# Managed by Dubly install script
$CADDY_DOMAINS {
    reverse_proxy localhost:8080

    header {
        X-Frame-Options "DENY"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "strict-origin-when-cross-origin"
    }
}
EOF

  ok "Caddyfile written to $CADDYFILE"
fi

# ── UFW firewall ──────────────────────────────────────────────────────

header "Configuring Firewall"

ufw allow 80/tcp >/dev/null 2>&1
ufw allow 443/tcp >/dev/null 2>&1
ufw deny 8080/tcp >/dev/null 2>&1

if ! ufw status | grep -q "Status: active"; then
  info "Enabling UFW (will allow SSH)..."
  ufw allow OpenSSH >/dev/null 2>&1
  ufw --force enable
fi

ok "Firewall configured (80/443 open, 8080 blocked)"

# ── Start services ────────────────────────────────────────────────────

header "Starting Services"

systemctl daemon-reload

systemctl enable dubly
systemctl restart dubly
ok "Dubly service started"

systemctl enable caddy
systemctl restart caddy
ok "Caddy service started"

# ── Done ──────────────────────────────────────────────────────────────

INSTALL_SUCCESS=true

# Extract admin domain for display
ADMIN_DOMAIN="${INPUT_DOMAINS%%,*}"

header "Installation Complete"
echo -e "  ${GREEN}Dubly is now running!${NC}"
echo ""
echo "  Admin URL:    https://$ADMIN_DOMAIN"
if [ "${PASSWORD_AUTO:-false}" = true ]; then
  echo ""
  echo -e "  ${BOLD}API password:   ${YELLOW}$INPUT_PASSWORD${NC}"
  echo -e "  ${YELLOW}Save this — it won't be shown again.${NC}"
fi
echo ""
echo -e "  ${BOLD}Useful commands:${NC}"
echo "    systemctl status dubly       # check service status"
echo "    systemctl status caddy       # check reverse proxy"
echo "    journalctl -u dubly -f       # follow logs"
echo "    $INSTALL_DIR/scripts/install.sh --update   # pull & rebuild"
echo ""
echo -e "  ${BOLD}Test the API:${NC}"
echo "    curl -s https://$ADMIN_DOMAIN/api/links -H 'X-API-Key: <password>'"
echo ""

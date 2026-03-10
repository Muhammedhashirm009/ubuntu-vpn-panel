#!/usr/bin/env bash
# VPN Panel setup for Ubuntu 22.04+ with Xray + Dropbear
set -euo pipefail

APP_DIR="/opt/vpn-panel"
PANEL_PORT=9990
REQUIRED_PORTS=(80 443 2022 9990)
DOMAIN=""
EMAIL=""
OPEN_9990="n"

usage() {
  echo "Usage: sudo ./setup.sh --domain your.domain --email admin@example.com [--open-9990]"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="$2"; shift 2 ;;
    --email) EMAIL="$2"; shift 2 ;;
    --open-9990) OPEN_9990="y"; shift 1 ;;
    *) usage ;;
  esac
done

# Interactive prompts if missing
if [[ -z "$DOMAIN" ]]; then
  read -rp "Enter domain for TLS (A record must point here): " DOMAIN
fi
if [[ -z "$EMAIL" ]]; then
  read -rp "Enter email for Let's Encrypt notices: " EMAIL
fi
read -rp "Open firewall port 9990 for direct admin access? [y/N]: " resp
if [[ "${resp,,}" == "y" ]]; then OPEN_9990="y"; fi

[[ -z "$DOMAIN" || -z "$EMAIL" ]] && usage

log() { echo "[$(date -Is)] $*"; }
require_root() { [[ $EUID -eq 0 ]] || { echo "Run as root"; exit 1; }; }

ensure_ports_free() {
  for p in "${REQUIRED_PORTS[@]}"; do
    local offenders
    offenders=$(lsof -i :"$p" -sTCP:LISTEN -n -P 2>/dev/null || true)
    if [[ -n "$offenders" ]]; then
      log "Port $p in use, killing blockers"
      echo "$offenders" | awk 'NR>1{print $2}' | xargs -r -I{} kill -15 {} || true
      sleep 1
      echo "$offenders" | awk 'NR>1{print $2}' | xargs -r -I{} kill -9 {} || true
      echo "$offenders" | awk 'NR>1{print $1" pid "$2" on port '$p'"}' >> /var/log/vpn-panel/install.log
    fi
  done
}

install_packages() {
  log "Updating apt and installing deps"
  apt-get update
  apt-get install -y curl wget unzip tar ufw nginx certbot python3-certbot-nginx sqlite3 lsof dropbear openssh-server
}

install_go() {
  if ! command -v go >/dev/null 2>&1; then
    log "Installing Go"
    VERSION="1.22.2"
    wget -q https://go.dev/dl/go${VERSION}.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    export PATH=/usr/local/go/bin:$PATH
  fi
}

install_xray() {
  log "Installing Xray"
  bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
  mkdir -p /usr/local/etc/xray/users.d
  cat >/usr/local/etc/xray/config.json <<'JSON'
{
  "log": {"loglevel": "warning"},
  "inbounds": [
    {"port": 10000, "listen": "127.0.0.1", "protocol": "vless", "settings": {"clients": []}, "streamSettings":{"network":"ws","security":"none","wsSettings":{"path":"/ws"}}},
    {"port": 10001, "listen": "127.0.0.1", "protocol": "vmess", "settings": {"clients": []}, "streamSettings":{"network":"ws","security":"none","wsSettings":{"path":"/vm"}}},
    {"port": 10002, "listen": "127.0.0.1", "protocol": "trojan", "settings": {"clients": []}, "streamSettings":{"network":"grpc","grpcSettings":{"serviceName":"trojan"}}}
  ],
  "outbounds": [{ "protocol": "freedom" }]
}
JSON
  systemctl enable xray
  systemctl restart xray
}

configure_dropbear() {
  log "Configuring Dropbear on 2022"
  sed -i 's/NO_START=1/NO_START=0/g' /etc/default/dropbear
  sed -i 's/^DROPBEAR_PORT=.*/DROPBEAR_PORT=2022/' /etc/default/dropbear
  systemctl enable dropbear
  systemctl restart dropbear
}

ensure_sshd_password_auth() {
  log "Ensuring sshd allows password auth and root login"
  local cfg="/etc/ssh/sshd_config"
  cp "$cfg" "${cfg}.bak.$(date +%s)" || true
  grep -q '^PasswordAuthentication' "$cfg" && sed -i 's/^PasswordAuthentication.*/PasswordAuthentication yes/' "$cfg" || echo "PasswordAuthentication yes" >> "$cfg"
  grep -q '^PermitRootLogin' "$cfg" && sed -i 's/^PermitRootLogin.*/PermitRootLogin yes/' "$cfg" || echo "PermitRootLogin yes" >> "$cfg"
  grep -q '^ChallengeResponseAuthentication' "$cfg" && sed -i 's/^ChallengeResponseAuthentication.*/ChallengeResponseAuthentication yes/' "$cfg" || echo "ChallengeResponseAuthentication yes" >> "$cfg"
  # Restart whichever service name exists (Debian/Ubuntu usually 'ssh')
  systemctl restart ssh || systemctl restart sshd || service ssh restart || true
}

build_panel() {
  log "Building panel binary"
  mkdir -p "$APP_DIR"/{bin,data}
  cp -r ./panel/* "$APP_DIR"/
  pushd "$APP_DIR" >/dev/null
  /usr/local/go/bin/go mod tidy
  /usr/local/go/bin/go build -o bin/vpn-panel ./cmd/api
  popd >/dev/null
}

setup_nginx() {
  log "Configuring nginx for panel (443->9990) and Xray WS paths"
  cat >/etc/nginx/sites-available/vpn-panel.conf <<EOF
server {
    listen 80;
    server_name ${DOMAIN};
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://${DOMAIN}\$request_uri; }
}

server {
    listen 443 ssl http2;
    server_name ${DOMAIN};
    ssl_certificate /etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${DOMAIN}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    location /ws { proxy_pass http://127.0.0.1:10000; proxy_http_version 1.1; proxy_set_header Upgrade \$http_upgrade; proxy_set_header Connection "upgrade"; proxy_set_header Host \$host; }
    location /vm { proxy_pass http://127.0.0.1:10001; proxy_http_version 1.1; proxy_set_header Upgrade \$http_upgrade; proxy_set_header Connection "upgrade"; proxy_set_header Host \$host; }
    location /trojan { proxy_pass http://127.0.0.1:10002; proxy_set_header Host \$host; }

    location / { proxy_pass http://127.0.0.1:${PANEL_PORT}; proxy_set_header Host \$host; }
}
EOF
  ln -sf /etc/nginx/sites-available/vpn-panel.conf /etc/nginx/sites-enabled/vpn-panel.conf
  nginx -t && systemctl reload nginx
}

issue_cert() {
  log "Requesting Let's Encrypt certificate"
  # use standalone so nginx config absence is fine; ensure port 80 free
  systemctl stop nginx || true
  certbot certonly --standalone -d "$DOMAIN" --non-interactive --agree-tos -m "$EMAIL" --preferred-challenges http
  systemctl start nginx
}

setup_systemd() {
  log "Creating systemd service"
  cat >/etc/systemd/system/vpn-panel.service <<EOF
[Unit]
Description=VPN Panel API
After=network.target

[Service]
Type=simple
Environment=PANEL_DB_PATH=${APP_DIR}/data/panel.db
Environment=PANEL_ADMIN_USER=admin
Environment=PANEL_ADMIN_PASS=changeme
Environment=PANEL_JWT_SECRET=$(head -c 32 /dev/urandom | base64)
Environment=PORT=${PANEL_PORT}
WorkingDirectory=${APP_DIR}
ExecStart=${APP_DIR}/bin/vpn-panel
Restart=on-failure
User=root

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable vpn-panel
  systemctl restart vpn-panel
}

configure_firewall() {
  ufw allow 22/tcp || true
  ufw allow 80/tcp
  ufw allow 443/tcp
  ufw allow 2022/tcp
  if [[ "$OPEN_9990" == "y" ]]; then
    ufw allow 9990/tcp || true
  fi
  ufw --force enable
}

main() {
  require_root
  mkdir -p /var/log/vpn-panel
  touch /var/log/vpn-panel/install.log
  install_packages
  ensure_ports_free
  ensure_sshd_password_auth
  install_go
  install_xray
  configure_dropbear
  build_panel
  issue_cert
  setup_nginx
  setup_systemd
  configure_firewall
  log "Setup complete. Panel: https://${DOMAIN} (proxy) or http://<ip>:9990 if opened. Default admin password: changeme"
}

main "$@"

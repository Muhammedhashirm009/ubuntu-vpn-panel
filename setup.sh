#!/usr/bin/env bash
# VPN Panel setup for Ubuntu 22.04+ with Xray + Dropbear
set -euo pipefail

APP_DIR="/opt/vpn-panel"
PANEL_PORT=9990
REQUIRED_PORTS=(80 443 2022 9990)
DOMAIN=""
EMAIL=""

usage() {
  echo "Usage: sudo ./setup.sh --domain your.domain --email admin@example.com"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="$2"; shift 2 ;;
    --email) EMAIL="$2"; shift 2 ;;
    *) usage ;;
  esac
done

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
      echo "$offenders" | awk 'NR>1{print $1\" pid \"$2\" on port '$p'\"}' >> /var/log/vpn-panel/install.log
    fi
  done
}

install_packages() {
  log "Updating apt and installing deps"
  apt-get update
  apt-get install -y curl wget unzip tar ufw nginx certbot python3-certbot-nginx sqlite3 lsof dropbear
}

install_go() {
  if ! command -v go >/dev/null 2>&1; then
    log "Installing Go"
    VERSION="1.23.1"
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
    {"port": 443, "listen": "127.0.0.1", "protocol": "vless", "settings": {"clients": []}, "streamSettings":{"network":"ws","security":"none","wsSettings":{"path":"/ws"}}},
    {"port": 8443, "listen": "127.0.0.1", "protocol": "vmess", "settings": {"clients": []}, "streamSettings":{"network":"ws","security":"none","wsSettings":{"path":"/vm"}}},
    {"port": 2053, "listen": "127.0.0.1", "protocol": "trojan", "settings": {"clients": []}, "streamSettings":{"network":"grpc","grpcSettings":{"serviceName":"trojan"}}}
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
    location / { return 301 https://\$host\$request_uri; }
}

server {
    listen 443 ssl http2;
    server_name ${DOMAIN};
    ssl_certificate /etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${DOMAIN}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    location /ws { proxy_pass http://127.0.0.1:443; proxy_http_version 1.1; proxy_set_header Upgrade \$http_upgrade; proxy_set_header Connection \"upgrade\"; }
    location /vm { proxy_pass http://127.0.0.1:8443; proxy_http_version 1.1; proxy_set_header Upgrade \$http_upgrade; proxy_set_header Connection \"upgrade\"; }
    location /trojan { proxy_pass http://127.0.0.1:2053; }

    location / { proxy_pass http://127.0.0.1:${PANEL_PORT}; proxy_set_header Host \$host; }
}
EOF
  ln -sf /etc/nginx/sites-available/vpn-panel.conf /etc/nginx/sites-enabled/vpn-panel.conf
  nginx -t && systemctl reload nginx
}

issue_cert() {
  log "Requesting Let’s Encrypt certificate"
  certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos -m "$EMAIL"
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
  # 9990 optional direct admin access
  ufw --force enable
}

main() {
  require_root
  mkdir -p /var/log/vpn-panel
  touch /var/log/vpn-panel/install.log
  ensure_ports_free
  install_packages
  install_go
  install_xray
  configure_dropbear
  build_panel
  issue_cert
  setup_nginx
  setup_systemd
  configure_firewall
  log \"Setup complete. Panel on https://${DOMAIN} (proxy) or http://<ip>:9990 if opened. Default admin password: changeme\"\n}\n\nmain \"$@\"\n*** End Patch"})?;

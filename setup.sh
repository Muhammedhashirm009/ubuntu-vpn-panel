#!/usr/bin/env bash
# VPN Panel setup for Ubuntu 22.04+ with Xray + Dropbear
set -euo pipefail

DOMAIN_PANEL=""
EMAIL=""
OPEN_9990="n"
APP_DIR="/opt/vpn-panel"
PANEL_PORT="9990"
REQUIRED_PORTS=(80 443 "$PANEL_PORT" 2022)

usage() {
  echo "Usage: sudo ./setup.sh --panel-domain panel.domain.com --email admin@example.com [--open-9990]"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --panel-domain) DOMAIN_PANEL="$2"; shift 2 ;;
    --email) EMAIL="$2"; shift 2 ;;
    --open-9990) OPEN_9990="y"; shift 1 ;;
    *) usage ;;
  esac
done

# Interactive prompts if missing
if [[ -z "$DOMAIN_PANEL" ]]; then
  read -rp "Enter PANEL domain (A record must point here): " DOMAIN_PANEL
fi
if [[ -z "$EMAIL" ]]; then
  read -rp "Enter email for Let's Encrypt notices: " EMAIL
fi
read -rp "Open firewall port 9990 for direct admin access? [y/N]: " resp
if [[ "${resp,,}" == "y" ]]; then OPEN_9990="y"; fi

[[ -z "$DOMAIN_PANEL" || -z "$EMAIL" ]] && usage

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
  apt-get install -y curl wget unzip tar ufw nginx certbot python3-certbot-nginx sqlite3 lsof dropbear openssh-server unbound dnsdist libnginx-mod-stream build-essential
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
    {"port": 10000, "listen": "127.0.0.1", "protocol": "vless", "settings": {"decryption": "none", "clients": []}, "streamSettings":{"network":"ws","security":"none","wsSettings":{"path":"/ws"}}},
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
  cat > /etc/issue.net << 'EOF'
<font color="green"><b>======================================</b></font><br>
<font color="red"><b>        VPN SYSTEM BY ERPAGENT        </b></font><br>
<font color="green"><b>======================================</b></font><br>
<font color="white"><b>Welcome to the Premium VPN Service.</b></font><br>
<br>
<font color="yellow"><b>Rules:</b></font><br>
<font color="white"><b>1. No Torrenting / Illegal activities.</b></font><br>
<font color="white"><b>2. Do not share your account.</b></font><br>
<font color="white"><b>3. No Spamming or Hacking.</b></font><br>
<br>
<font color="red"><b>Violation of rules will result in account termination.</b></font><br>
<font color="green"><b>======================================</b></font><br>
EOF
  sed -i 's/NO_START=1/NO_START=0/g' /etc/default/dropbear
  sed -i 's/^#*DROPBEAR_PORT=.*/DROPBEAR_PORT=2022/' /etc/default/dropbear
  sed -i 's|^#*DROPBEAR_EXTRA_ARGS=.*|DROPBEAR_EXTRA_ARGS="-b /etc/issue.net"|' /etc/default/dropbear
  grep -q "/usr/sbin/nologin" /etc/shells || echo "/usr/sbin/nologin" >> /etc/shells
  grep -q "/bin/false" /etc/shells || echo "/bin/false" >> /etc/shells
  systemctl enable dropbear
  systemctl reset-failed dropbear || true
  systemctl restart dropbear
}

ensure_sshd_password_auth() {
  log "Ensuring sshd allows password auth and root login"
  # Clean up cloud provider overrides that typically block password auth (Google Cloud, AWS, etc)
  rm -f /etc/ssh/sshd_config.d/50-cloud-init.conf || true
  rm -f /etc/ssh/sshd_config.d/60-cloudimg-settings.conf || true

  local cfg="/etc/ssh/sshd_config"
  cp "$cfg" "${cfg}.bak.$(date +%s)" || true

  sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication yes/' "$cfg"
  sed -i 's/^#*PermitRootLogin.*/PermitRootLogin yes/' "$cfg"
  sed -i 's/^#*ChallengeResponseAuthentication.*/ChallengeResponseAuthentication yes/' "$cfg"
  sed -i 's/^#*KbdInteractiveAuthentication.*/KbdInteractiveAuthentication yes/' "$cfg"

  grep -q "^PasswordAuthentication yes" "$cfg" || echo "PasswordAuthentication yes" >> "$cfg"
  grep -q "^PermitRootLogin yes" "$cfg" || echo "PermitRootLogin yes" >> "$cfg"
  grep -q "^ChallengeResponseAuthentication yes" "$cfg" || echo "ChallengeResponseAuthentication yes" >> "$cfg"
  grep -q "^KbdInteractiveAuthentication yes" "$cfg" || echo "KbdInteractiveAuthentication yes" >> "$cfg"

  # Restart whichever service name exists (Debian/Ubuntu usually 'ssh')
  systemctl restart ssh || systemctl restart sshd || service ssh restart || true
}

build_panel() {
  log "Building panel binary"
  mkdir -p "$APP_DIR"/{bin,data}
  cp -r ./panel/* "$APP_DIR"/
  pushd "$APP_DIR" >/dev/null
  /usr/local/go/bin/go mod tidy
  CGO_ENABLED=1 /usr/local/go/bin/go build -o bin/vpn-panel ./cmd/api
  popd >/dev/null
}

setup_nginx() {
  log "Configuring nginx for panel domain"
  HOST_VAR='$host'
  REQ_VAR='$request_uri'
  cat >/etc/nginx/sites-available/vpn-panel.conf <<EOF
server {
    listen 80;
    server_name ${DOMAIN_PANEL};
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://${DOMAIN_PANEL}${REQ_VAR}; }
}

server {
    listen 443 ssl http2;
    server_name ${DOMAIN_PANEL};
    ssl_certificate /etc/letsencrypt/live/${DOMAIN_PANEL}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${DOMAIN_PANEL}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    location / { proxy_pass http://127.0.0.1:${PANEL_PORT}; proxy_set_header Host ${HOST_VAR}; }
}
EOF
  ln -sf /etc/nginx/sites-available/vpn-panel.conf /etc/nginx/sites-enabled/vpn-panel.conf
  nginx -t && systemctl reload nginx
}

issue_cert() {
  log "Requesting Let's Encrypt certificate"
  # use standalone so nginx config absence is fine; ensure port 80/443 free
  systemctl stop nginx || true
  certbot certonly --standalone -d "$DOMAIN_PANEL" --non-interactive --agree-tos -m "$EMAIL" --preferred-challenges http
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
Environment=LETSENCRYPT_EMAIL=${EMAIL}
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
  ufw allow 853/tcp
  ufw allow 2022/tcp
  if [[ "$OPEN_9990" == "y" ]]; then
    ufw allow 9990/tcp || true
  fi
  ufw --force enable
}

setup_unbound_dnsdist() {
  log "Configuring Unbound and dnsdist for Private DNS"
  
  cat >/etc/unbound/unbound.conf.d/private-dns.conf <<EOF
server:
    interface: 127.0.0.1
    port: 5353
    do-ip4: yes
    do-udp: yes
    do-tcp: yes
    access-control: 127.0.0.0/8 allow
    hide-identity: yes
    hide-version: yes
    cache-min-ttl: 300
    cache-max-ttl: 86400
    prefetch: yes
  
forward-zone:
    name: "."
    forward-addr: 8.8.8.8
    forward-addr: 1.1.1.1
EOF
  systemctl restart unbound || true

  cat >/etc/dnsdist/dnsdist.conf <<EOF
setLocal('127.0.0.1:8053')
newServer({address='127.0.0.1:5353', pool='unbound'})
addAction(AllRule(), PoolAction('unbound'))
EOF
  systemctl restart dnsdist || true
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
  setup_unbound_dnsdist
  configure_firewall
  
  # Allow vpn-panel user (root here) to run certbot and systemctl commands securely via Go if not running as root
  echo "root ALL=(ALL) NOPASSWD: /usr/bin/certbot, /usr/bin/systemctl" > /etc/sudoers.d/vpn-panel

  log "Setup complete. Panel: https://${DOMAIN_PANEL} | Default admin: admin / changeme"
}

main "$@"

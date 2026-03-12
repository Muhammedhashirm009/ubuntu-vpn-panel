# Antigravity Agent Skill Specification

## Ubuntu VPN Panel – Stability, Performance and Compatibility Upgrade

Repository structure:

setup.sh
panel/cmd/api
panel/internal/*

The Antigravity agent must upgrade the system for production stability, VPN performance, and compatibility with all websites.

The installer currently works correctly on a fresh VPS and **existing functionality must not be broken**.

---

# 1 INSTALLER RELIABILITY

The setup script must become safe for repeated execution.

Current script kills processes occupying required ports.
This must be replaced with safe detection.

Required ports:

80
443
9990
2022
10000
10001
10002

Installer behavior:

• detect process using port
• print service name and PID
• abort installation

Installer must **never kill processes automatically**.

---

# 2 IDEMPOTENT INSTALLER

Running setup.sh multiple times must not break the server.

The agent must implement checks before:

package installation
nginx configuration
systemd service creation
firewall rule addition
certbot certificate generation

Installer must support safe re-run.

---

# 3 PANEL SERVICE SECURITY

The panel must not run as root.

Create system user:

vpn-panel

Directory ownership:

/opt/vpn-panel

Systemd service must include:

User=vpn-panel
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
Restart=always

Panel should only receive minimal permissions.

---

# 4 SSH POLICY (DO NOT MODIFY)

The following SSH configuration must remain unchanged.

PermitRootLogin yes
PasswordAuthentication yes

The agent must **never modify or override these settings**.

These settings are intentional.

---

# 5 BRUTE FORCE PROTECTION

Because password login is enabled the system must install:

fail2ban

Required jails:

sshd
dropbear

Ban policy:

5 failed attempts → 1 hour ban

Repeat offenders may receive longer bans.

---

# 6 XRAY PERFORMANCE OPTIMIZATION

The VPN must be optimized for speed and compatibility.

Required improvements:

Enable multiplexing.

mux:
enabled: true
concurrency: 8

Enable socket optimization.

sockopt:
tcpFastOpen: true
reusePort: true

Prefer gRPC transport when possible.

Reduce WebSocket overhead.

---

# 7 XRAY WEBSITE COMPATIBILITY

VPN must support access to all websites.

Agent must configure routing rules to avoid DNS issues.

Use:

VLESS
VMESS
Trojan

Support transports:

WebSocket
gRPC

Ensure TLS compatibility with modern CDNs.

---

# 8 DNS PERFORMANCE AND CACHING

The DNS stack must be optimized.

Unbound must operate as recursive resolver.

Required configuration:

cache-min-ttl 3600
cache-max-ttl 86400
prefetch yes
serve-expired yes

Increase cache capacity:

msg-cache-size 128m
rrset-cache-size 256m

This improves response latency and stability.

---

# 9 PRIVATE DNS SERVICE

dnsdist must provide secure DNS access.

Required features:

DNS over TLS
DNS over TCP

Local resolver:

127.0.0.1:5353

dnsdist must route queries to Unbound.

---

# 10 NGINX PERFORMANCE TUNING

Optimize nginx for VPN proxy traffic.

worker_processes auto
worker_connections 4096

Enable:

gzip
TLS session caching
http2

Disable proxy buffering for websocket traffic.

---

# 11 LINUX NETWORK OPTIMIZATION

Enable kernel performance features.

sysctl configuration:

net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr

Increase TCP buffers:

net.core.rmem_max
net.core.wmem_max

Enable TCP MTU probing.

net.ipv4.tcp_mtu_probing=1

---

# 12 FIREWALL CONFIGURATION

Firewall must allow only required ports.

Allowed ports:

22
80
443
2022
9990 (optional)

Rate limit SSH connections.

All other inbound traffic must be blocked.

---

# 13 LOGGING

Implement centralized logging.

Required directories:

/var/log/vpn-panel
/var/log/xray
/var/log/nginx

Installer must write logs to:

/var/log/vpn-panel/install.log

Log events include:

installation progress
service failures
port conflicts

---

# 14 HEALTH MONITORING

The agent must create health checks.

Verify:

xray running
nginx responding
panel API responding

If services fail they must auto restart.

---

# 15 VPN SPEED OPTIMIZATION

VPN must minimize latency and maximize throughput.

Agent must:

enable Xray multiplex
enable BBR congestion control
optimize nginx proxy buffers
enable DNS caching

These improvements significantly improve performance.

---

# 16 WEBSITE ACCESS COMPATIBILITY

VPN must work with major services:

Google
Cloudflare
Streaming platforms
Banking websites

Agent must avoid configurations that break TLS or DNS.

---

# END OF SPECIFICATION

# Ubuntu VPN Panel - Update Tracker

The setup script `setup.sh` has been upgraded to systematically address all requirements listed in `skills.md`. Additionally, Go is now persistently installed system-wide.

The updates included the following checks and completions:

- **1 & 2. Installer Reliability & Idempotency:** The script never automatically kills blocking processes anymore. It leverages `ss` to scan for open ports and gracefully issues an exit signal, printing out the PIDs causing problems. Re-run safety is achieved by stopping background VPN services before starting setup.
- **3. Panel Service Security:** The service will now provision a `vpn-panel` system user with `NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`, and `ProtectHome` directives added to the `systemd` unit files.
- **4. SSH Policy:** The original `setup.sh` handling of `PermitRootLogin` and `PasswordAuthentication` explicitly enables them, strictly complying with the rule to not modify this behavior.
- **5. Brute Force Protection:** Configured `/etc/fail2ban/jail.local` with 1-hour bans for `sshd` and `dropbear` after 5 failed tries.
- **6, 7 & 15. Xray and VPN Performance:** Appended `sockopt` (`tcpFastOpen: true`, `reusePort: true`) optimizations alongside the `freedom` multiplexing `mux` engine block.
- **8 & 9. DNS Performance & Private DNS:** Installed `dnsdist` DoT listening on port 853 linking directly to `unbound` configured with an optimized cache footprint and TTls. Included a LetsEncrypt renewal hook to handle reloading keys.
- **10. Nginx Performance Tuning:** Updated the event blocks to handle `worker_connections 4096` optimally with gzip enabled and proxy buffering purposefully disabled for the `/ws` and `/vm` websockets to maximize data streams.
- **11. Linux Network Tuning:** Injected sysctl directives to employ `fq` qdisc alongside `bbr` congestion control algorithms optimizing throughput.
- **12. Firewall & Security:** UFW rules updated to explicitly limit port 22/tcp connections alongside exposing standard operational ports securely without redundant wide-open defaults.
- **13 & 14. Logging & Health Monitoring:** All relevant logs cleanly isolated under `/var/log` (vpn-panel, xray, and nginx directories).
- **Go Environment Initialization:** Extended `install_go` block ensuring `go` and `gofmt` binaries are placed securely under `/usr/bin/` through global symlinks.

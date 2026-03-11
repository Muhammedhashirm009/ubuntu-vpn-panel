package handlers

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"vpn-panel/internal/db"
)

type SetupHandler struct {
	Store *db.Store
}

type setupReq struct {
	NewUser     string `json:"new_user"`
	NewPassword string `json:"new_password"`
	VPNDomain   string `json:"vpn_domain"`
}

func (h *SetupHandler) InitSetup(c *gin.Context) {
	if h.Store.GetSetting("setup_complete") == "true" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Setup already complete"})
		return
	}

	var req setupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if req.NewUser == "" || req.NewPassword == "" || req.VPNDomain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "All fields are required"})
		return
	}

	// 1. Issue TLS Certificate for the new VPN Domain
	email := os.Getenv("LETSENCRYPT_EMAIL")
	if email == "" {
		email = "admin@localhost"
	}
	
	certCmd := fmt.Sprintf("certbot certonly --nginx -d %s --non-interactive --agree-tos -m %s", req.VPNDomain, email)
	if err := exec.Command("sudo", "bash", "-c", certCmd).Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to provision SSL certificate for VPN domain", "details": err.Error()})
		return
	}

	// 2. Write Nginx configuration for the VPN Domain
	nginxConf := fmt.Sprintf(`server {
    listen 80;
    server_name %s;
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://$host$request_uri; }
}

server {
    listen 443 ssl http2;
    server_name %s;
    ssl_certificate /etc/letsencrypt/live/%s/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/%s/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    location /ws { proxy_pass http://127.0.0.1:10000; proxy_http_version 1.1; proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection "upgrade"; proxy_set_header Host $host; }
    location /vm { proxy_pass http://127.0.0.1:10001; proxy_http_version 1.1; proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection "upgrade"; proxy_set_header Host $host; }
    location /trojan { 
        grpc_pass grpc://127.0.0.1:10002;
        grpc_set_header Host $host;
    }

    location / { return 404; }
}`, req.VPNDomain, req.VPNDomain, req.VPNDomain, req.VPNDomain)

	confPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", req.VPNDomain)
	if err := os.WriteFile(confPath, []byte(nginxConf), 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write Nginx configuration"})
		return
	}

	linkCmd := fmt.Sprintf("ln -sf %s /etc/nginx/sites-enabled/%s.conf && systemctl reload nginx", confPath, req.VPNDomain)
	if err := exec.Command("sudo", "bash", "-c", linkCmd).Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reload Nginx"})
		return
	}

	// 3. Update Admin Credentials (wipe existing defaults and add new)
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}
	// Wipe default admin to ensure a clean multi-admin slate, then add the new
	h.Store.DB.Exec(`DELETE FROM admin`)
	
	if err := h.Store.AddAdmin(req.NewUser, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to configure admin credentials"})
		return
	}

	// 4. Mark setup complete and save VPN domain
	_ = h.Store.SetSetting("vpn_domain", req.VPNDomain)
	_ = h.Store.SetSetting("setup_complete", "true")

	// Restart Xray mapping
	exec.Command("sudo", "systemctl", "restart", "xray").Run()

	c.JSON(http.StatusOK, gin.H{"status": "setup complete"})
}

func (h *SetupHandler) GetStatus(c *gin.Context) {
	isComplete := h.Store.GetSetting("setup_complete") == "true"
	c.JSON(http.StatusOK, gin.H{
		"setup_complete": isComplete,
		"vpn_domain":     h.Store.GetSetting("vpn_domain"),
	})
}

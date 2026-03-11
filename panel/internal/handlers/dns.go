package handlers

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"github.com/gin-gonic/gin"

	"vpn-panel/internal/db"
)

type DNSHandler struct {
	Store *db.Store
}

func (h *DNSHandler) List(c *gin.Context) {
	domains, err := h.Store.ListDNSDomains()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list dns domains"})
		return
	}
	c.JSON(http.StatusOK, domains)
}

type addDNSReq struct {
	Domain string `json:"domain"`
}

func (h *DNSHandler) Add(c *gin.Context) {
	var req addDNSReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.Domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain is required"})
		return
	}

	// 1. Issue TLS Certificate for the Private DNS Domain
	email := os.Getenv("LETSENCRYPT_EMAIL")
	if email == "" {
		email = "admin@localhost"
	}
	
	certCmd := fmt.Sprintf("certbot certonly --nginx -d %s --non-interactive --agree-tos -m %s", req.Domain, email)
	if err := exec.Command("sudo", "bash", "-c", certCmd).Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to provision SSL certificate for DNS domain", "details": err.Error()})
		return
	}

	// 2. Write Nginx configuration for DoH (HTTPS) and DoT (Port 853)
	// We need a stream block for DoT (853) and an HTTP block for DoH (443)
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

    location /dns-query { 
        proxy_pass http://127.0.0.1:8053; 
        proxy_http_version 1.1; 
        proxy_set_header Host $host; 
        proxy_set_header X-Real-IP $remote_addr;
    }

    location / { return 404; }
}`, req.Domain, req.Domain, req.Domain, req.Domain)

	confPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", req.Domain)
	if err := os.WriteFile(confPath, []byte(nginxConf), 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write Nginx HTTP configuration"})
		return
	}
	
	linkCmd := fmt.Sprintf("ln -sf %s /etc/nginx/sites-enabled/%s.conf", confPath, req.Domain)
	exec.Command("sudo", "bash", "-c", linkCmd).Run()

	// 2.5 Write Nginx Stream block for Android Private DNS (DoT on Port 853)
	streamConf := fmt.Sprintf(`server {
    listen 853 ssl;
    ssl_certificate /etc/letsencrypt/live/%s/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/%s/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    proxy_pass 127.0.0.1:8053;
}`, req.Domain, req.Domain)

	exec.Command("sudo", "mkdir", "-p", "/etc/nginx/streams").Run()
	streamPath := fmt.Sprintf("/etc/nginx/streams/%s.conf", req.Domain)
	if err := os.WriteFile(streamPath, []byte(streamConf), 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write Nginx Stream configuration"})
		return
	}
	
	// Ensure main nginx.conf includes the streams directory
	exec.Command("sudo", "bash", "-c", `grep -q "include /etc/nginx/streams/\*.conf;" /etc/nginx/nginx.conf || echo "stream { include /etc/nginx/streams/*.conf; }" >> /etc/nginx/nginx.conf`).Run()

	if err := exec.Command("sudo", "systemctl", "reload", "nginx").Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reload Nginx"})
		return
	}

	// 3. Save to DB
	if _, err := h.Store.AddDNSDomain(req.Domain); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save domain to DB"})
		return
	}

	_ = h.Store.AddAudit("dns_added", fmt.Sprintf("domain=%s", req.Domain))
	c.JSON(http.StatusOK, gin.H{"status": "dns domain added"})
}

type deleteDNSReq struct {
	ID     int64  `json:"id"`
	Domain string `json:"domain"`
}

func (h *DNSHandler) Delete(c *gin.Context) {
	var req deleteDNSReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := h.Store.DeleteDNSDomain(req.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete dns domain"})
		return
	}
	
	// Try cleaning up Nginx
	os.Remove(fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", req.Domain))
	os.Remove(fmt.Sprintf("/etc/nginx/sites-available/%s.conf", req.Domain))
	os.Remove(fmt.Sprintf("/etc/nginx/streams/%s.conf", req.Domain))
	exec.Command("sudo", "systemctl", "reload", "nginx").Run()

	_ = h.Store.AddAudit("dns_deleted", fmt.Sprintf("domain=%s", req.Domain))
	c.JSON(http.StatusOK, gin.H{"status": "dns domain deleted"})
}

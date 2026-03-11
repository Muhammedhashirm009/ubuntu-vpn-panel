package handlers

import (
    "encoding/base64"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "vpn-panel/internal/db"
    "vpn-panel/internal/services"
)

type UsersHandler struct {
    Store      *db.Store
    XWriter    *services.XrayWriter
}

type createXrayReq struct {
    Protocol string `json:"protocol"` // vless|vmess|trojan
    Remark   string `json:"remark"`
    Days     int    `json:"days"`
}

type createSSHReq struct {
    Username string `json:"username"`
    Password string `json:"password"`
    Days     int    `json:"days"`
}

func (h *UsersHandler) CreateXray(c *gin.Context) {
    var req createXrayReq
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if req.Protocol == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "protocol required"})
        return
    }
    expiry := time.Now().AddDate(0, 0, max(req.Days, 30))
    uid := uuid.NewString()
    u := db.User{Protocol: req.Protocol, Username: fmt.Sprintf("%s-%s", req.Protocol, uid[:6]), UUID: uid, Remark: req.Remark, ExpiresAt: expiry, Active: true}
    id, err := h.Store.AddUser(u)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    if h.XWriter != nil {
        _, _ = h.XWriter.WriteUser(services.XrayUser{ID: id, Protocol: req.Protocol, Username: u.Username, UUID: uid, Remark: req.Remark})
        _ = services.ReloadXray()
    }
    vpnDomain := h.Store.GetSetting("vpn_domain")
    link := buildLink(req.Protocol, uid, req.Remark, vpnDomain)
    _ = h.Store.AddAudit("xray_user_created", fmt.Sprintf("protocol=%s user_id=%d", req.Protocol, id))
    c.JSON(http.StatusOK, gin.H{"id": id, "username": u.Username, "uuid": uid, "expires_at": expiry, "link": link})
}

func (h *UsersHandler) CreateSSH(c *gin.Context) {
    var req createSSHReq
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    expiry := time.Now().AddDate(0, 0, max(req.Days, 30))
    u := db.User{Protocol: "ssh", Username: req.Username, Password: req.Password, ExpiresAt: expiry, Active: true}
    id, err := h.Store.AddUser(u)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    _ = services.CreateDropbearUser(req.Username, req.Password)
    _ = h.Store.AddAudit("ssh_user_created", fmt.Sprintf("username=%s id=%d", req.Username, id))
    c.JSON(http.StatusOK, gin.H{"id": id, "username": req.Username, "expires_at": expiry})
}

func (h *UsersHandler) List(c *gin.Context) {
    users, err := h.Store.ListUsers()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    vpnDomain := h.Store.GetSetting("vpn_domain")
    for i := range users {
        if users[i].Protocol == "vless" || users[i].Protocol == "vmess" || users[i].Protocol == "trojan" {
            users[i].Link = buildLink(users[i].Protocol, users[i].UUID, users[i].Remark, vpnDomain)
        }
    }
    c.JSON(http.StatusOK, users)
}

func (h *UsersHandler) Delete(c *gin.Context) {
    var req struct{ ID int64 `json:"id"` }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Retrieve user first to know what physical files/users to delete
    var protocol, username string
    _ = h.Store.DB.QueryRow(`SELECT protocol, username FROM users WHERE id=?`, req.ID).Scan(&protocol, &username)

    if err := h.Store.HardDeleteUser(req.ID); err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
        return
    }

    // Physical Deletion
    if protocol == "ssh" {
        _ = services.DeleteDropbearUser(username)
    } else if protocol == "vless" || protocol == "vmess" || protocol == "trojan" {
        _ = h.XWriter.DeleteUser(req.ID)
        _ = services.ReloadXray()
    }

    _ = h.Store.AddAudit("user_deleted", fmt.Sprintf("id=%d", req.ID))
    c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func buildLink(proto, id, remark, domain string) string {
    if domain == "" {
        domain = "your.domain"
    }
    safeRemark := url.QueryEscape(remark)
    if safeRemark == "" {
        safeRemark = "VPN"
    }

    switch proto {
    case "vless":
        return fmt.Sprintf("vless://%s@%s:443?encryption=none&security=tls&sni=%s&type=ws&host=%s&path=%%2Fws#%s", id, domain, domain, domain, safeRemark)
    case "vmess":
        vobj := map[string]string{
            "v":    "2",
            "ps":   remark,
            "add":  domain,
            "port": "443",
            "id":   id,
            "aid":  "0",
            "scy":  "auto",
            "net":  "ws",
            "type": "none",
            "host": domain,
            "path": "/vm",
            "tls":  "tls",
            "sni":  domain,
        }
        b, _ := json.Marshal(vobj)
        return fmt.Sprintf("vmess://%s", base64.StdEncoding.EncodeToString(b))
    case "trojan":
        return fmt.Sprintf("trojan://%s@%s:443?security=tls&sni=%s&type=grpc&serviceName=trojan#%s", id, domain, domain, safeRemark)
    default:
        return ""
    }
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}

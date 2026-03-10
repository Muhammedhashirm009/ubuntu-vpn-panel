package handlers

import (
    "fmt"
    "net/http"
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
    link := buildLink(req.Protocol, uid)
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
    c.JSON(http.StatusOK, users)
}

func (h *UsersHandler) Delete(c *gin.Context) {
    var req struct{ ID int64 `json:"id"` }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if err := h.Store.DeactivateUser(req.ID); err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
        return
    }
    _ = h.Store.AddAudit("user_revoked", fmt.Sprintf("id=%d", req.ID))
    c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}

func buildLink(proto, id string) string {
    switch proto {
    case "vless":
        return fmt.Sprintf("vless://%s@your.domain:443?security=tls&type=ws#%s", id, id[:6])
    case "vmess":
        return fmt.Sprintf("vmess://%s", id)
    case "trojan":
        return fmt.Sprintf("trojan://%s@your.domain:443?security=tls&type=grpc", id)
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

package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "vpn-panel/internal/auth"
    "vpn-panel/internal/config"
    "vpn-panel/internal/db"
)

type AuthHandler struct {
    Store  *db.Store
    Cfg    config.Config
}

type loginRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

func (h *AuthHandler) Login(c *gin.Context) {
    var req loginRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    user, hash, err := h.Store.GetAdmin()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "admin not initialized"})
        return
    }
    if req.Username != user || auth.CheckPassword(hash, req.Password) != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
        return
    }
    token, err := auth.IssueToken(user, h.Cfg.JWTSecret)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
        return
    }
    c.SetCookie("panel_token", token, 86400, "/", "", true, true)
    c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func AuthMiddleware(secret string) gin.HandlerFunc {
    return func(c *gin.Context) {
        token, err := c.Cookie("panel_token")
        if err != nil || token == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "auth required"})
            return
        }
        if _, err := auth.ParseToken(token, secret); err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
            return
        }
        c.Next()
    }
}

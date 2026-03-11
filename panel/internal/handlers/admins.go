package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"vpn-panel/internal/db"
)

type AdminsHandler struct {
	Store *db.Store
}

func (h *AdminsHandler) List(c *gin.Context) {
	admins, err := h.Store.ListAdmins()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list admins"})
		return
	}
	c.JSON(http.StatusOK, admins)
}

type addAdminReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AdminsHandler) Add(c *gin.Context) {
	var req addAdminReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	if err := h.Store.AddAdmin(req.Username, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "username might already exist"})
		return
	}

	_ = h.Store.AddAudit("admin_added", fmt.Sprintf("username=%s", req.Username))
	c.JSON(http.StatusOK, gin.H{"status": "admin added"})
}

type updateAdminReq struct {
	Username string `json:"username"` // The admin taking action or the new username? We'll update password for now.
	Password string `json:"password"`
}

func (h *AdminsHandler) UpdatePassword(c *gin.Context) {
	var req updateAdminReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	if err := h.Store.UpdateAdminPassword(req.Username, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
		return
	}

	_ = h.Store.AddAudit("admin_updated", fmt.Sprintf("username=%s", req.Username))
	c.JSON(http.StatusOK, gin.H{"status": "password updated"})
}

type deleteAdminReq struct {
	ID int64 `json:"id"`
}

func (h *AdminsHandler) Delete(c *gin.Context) {
	var req deleteAdminReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := h.Store.DeleteAdmin(req.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete admin (or cannot delete main admin)"})
		return
	}

	_ = h.Store.AddAudit("admin_deleted", fmt.Sprintf("id=%d", req.ID))
	c.JSON(http.StatusOK, gin.H{"status": "admin deleted"})
}

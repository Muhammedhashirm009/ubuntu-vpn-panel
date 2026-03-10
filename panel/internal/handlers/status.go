package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "vpn-panel/internal/db"
    "vpn-panel/internal/ports"
)

type StatusHandler struct {
    Store *db.Store
}

func (h *StatusHandler) Ports(c *gin.Context) {
    portsToCheck := []int{80, 443, 2022, 9990}
    resp := []any{}
    for _, p := range portsToCheck {
        listeners, _ := ports.List(p)
        resp = append(resp, gin.H{"port": p, "listeners": listeners})
    }
    c.JSON(http.StatusOK, resp)
}

func (h *StatusHandler) Audits(c *gin.Context) {
    audits, err := h.Store.ListAudits(50)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, audits)
}

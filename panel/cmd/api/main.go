package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"vpn-panel/internal/auth"
	"vpn-panel/internal/config"
	"vpn-panel/internal/db"
	"vpn-panel/internal/handlers"
	"vpn-panel/internal/logging"
	"vpn-panel/internal/services"
)

func main() {
    cfg := config.Load()

    // ensure data dir exists
    _ = os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755)

    store, err := db.New(cfg.DBPath)
    if err != nil {
        log.Fatalf("db init: %v", err)
    }

    if _, err := logging.Setup(cfg.AuditLogPath); err != nil {
        log.Printf("log setup: %v", err)
    }

    // seed admin
    hash, err := authHash(cfg.AdminPass)
    if err != nil {
        log.Fatalf("hash admin: %v", err)
    }
    if err := store.UpsertAdmin(cfg.AdminUser, hash); err != nil {
        log.Fatalf("seed admin: %v", err)
    }

	r := gin.Default()
	r.Use(cors.Default())
	r.Static("/public", "./public")
	r.GET("/", func(c *gin.Context) { c.File("./public/index.html") })
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.File("./public/index.html")
	})

	// Public routes
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	authHandler := &handlers.AuthHandler{Store: store, Cfg: cfg}
	r.POST("/api/auth/login", authHandler.Login)

	protected := r.Group("/api")
	protected.Use(handlers.AuthMiddleware(cfg.JWTSecret))

	usersHandler := &handlers.UsersHandler{Store: store, XWriter: &services.XrayWriter{UserDir: cfg.XrayUserDir, ConfigPath: cfg.XrayConfig}}
	statusHandler := &handlers.StatusHandler{Store: store}

	protected.GET("/users", usersHandler.List)
	protected.POST("/users/xray", usersHandler.CreateXray)
	protected.POST("/users/ssh", usersHandler.CreateSSH)
	protected.POST("/users/revoke", usersHandler.Delete)

	protected.GET("/ports", statusHandler.Ports)
	protected.GET("/audits", statusHandler.Audits)

	log.Printf("server on :%s", cfg.PanelPort)
	if err := r.Run(":" + cfg.PanelPort); err != nil {
		log.Fatal(err)
	}
}

// small helper to avoid import cycle
func authHash(pw string) (string, error) {
	return auth.HashPassword(pw)
}

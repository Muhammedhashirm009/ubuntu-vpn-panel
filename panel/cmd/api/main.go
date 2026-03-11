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

	_ = os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755)

	store, err := db.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("db init: %v", err)
	}

	if _, err := logging.Setup(cfg.AuditLogPath); err != nil {
		log.Printf("log setup: %v", err)
	}

	if _, _, err := store.GetFirstAdmin(); err != nil {
		hash, err := authHash(cfg.AdminPass)
		if err != nil {
			log.Fatalf("hash admin: %v", err)
		}
		if err := store.AddAdmin(cfg.AdminUser, hash); err != nil {
			log.Fatalf("seed admin: %v", err)
		}
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

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	authHandler := &handlers.AuthHandler{Store: store, Cfg: cfg}
	r.POST("/api/auth/login", authHandler.Login)

	setupHandler := &handlers.SetupHandler{Store: store}
	r.GET("/api/setup/status", setupHandler.GetStatus)
	r.POST("/api/setup/init", setupHandler.InitSetup)

	// Setup Guard Middleware: block normal API usage if setup is incomplete
	setupGuard := func(c *gin.Context) {
		if store.GetSetting("setup_complete") != "true" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Setup incomplete"})
			c.Abort()
			return
		}
		c.Next()
	}

	protected := r.Group("/api")
	protected.Use(handlers.AuthMiddleware(cfg.JWTSecret))
	protected.Use(setupGuard)

	usersHandler := &handlers.UsersHandler{Store: store, XWriter: &services.XrayWriter{UserDir: cfg.XrayUserDir, ConfigPath: cfg.XrayConfig}}
	statusHandler := &handlers.StatusHandler{Store: store}

	protected.GET("/users", usersHandler.List)
	protected.POST("/users/xray", usersHandler.CreateXray)
	protected.POST("/users/ssh", usersHandler.CreateSSH)
	protected.POST("/users/revoke", usersHandler.Delete)

	adminsHandler := &handlers.AdminsHandler{Store: store}
	protected.GET("/admins", adminsHandler.List)
	protected.POST("/admins/add", adminsHandler.Add)
	protected.POST("/admins/update", adminsHandler.UpdatePassword)
	protected.POST("/admins/delete", adminsHandler.Delete)

	dnsHandler := &handlers.DNSHandler{Store: store}
	protected.GET("/dns", dnsHandler.List)
	protected.POST("/dns/add", dnsHandler.Add)
	protected.POST("/dns/delete", dnsHandler.Delete)

	protected.GET("/ports", statusHandler.Ports)
	protected.GET("/audits", statusHandler.Audits)
	protected.GET("/status/resources", statusHandler.Resources)

	log.Printf("server on :%s", cfg.PanelPort)
	if err := r.Run(":" + cfg.PanelPort); err != nil {
		log.Fatal(err)
	}
}

func authHash(pw string) (string, error) {
	return auth.HashPassword(pw)
}

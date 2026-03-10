package config

import (
    "log"
    "os"
)

// Config holds runtime settings.
type Config struct {
	DBPath       string
	AdminUser    string
	AdminPass    string
	JWTSecret    string
	PanelPort    string
	XrayUserDir  string
	XrayConfig   string
	AuditLogPath string
}

// Load reads environment variables with defaults.
func Load() Config {
    cfg := Config{
        DBPath:       getenv("PANEL_DB_PATH", "data/panel.db"),
        AdminUser:    getenv("PANEL_ADMIN_USER", "admin"),
        AdminPass:    getenv("PANEL_ADMIN_PASS", "changeme"),
        JWTSecret:    getenv("PANEL_JWT_SECRET", "dev-secret-change"),
        PanelPort:    getenv("PORT", "9990"),
		XrayUserDir:  getenv("XRAY_USER_DIR", "/usr/local/etc/xray/users.d"),
		XrayConfig:   getenv("XRAY_CONFIG", "/usr/local/etc/xray/config.json"),
        AuditLogPath: getenv("PANEL_AUDIT_LOG", "/var/log/vpn-panel/install.log"),
    }
    if cfg.JWTSecret == "dev-secret-change" {
        log.Println("[warn] using default JWT secret; set PANEL_JWT_SECRET for production")
    }
    return cfg
}

func getenv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

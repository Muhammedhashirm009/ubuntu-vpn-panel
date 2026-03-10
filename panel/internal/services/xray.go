package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type XrayUser struct {
    ID       int64  `json:"id"`
    Protocol string `json:"protocol"`
    Username string `json:"username"`
    UUID     string `json:"uuid"`
    Remark   string `json:"remark"`
}

type XrayWriter struct {
	UserDir     string
	ConfigPath  string
}

// WriteUser writes a per-user snippet AND appends to main Xray config clients list.
func (x *XrayWriter) WriteUser(u XrayUser) (string, error) {
	if err := os.MkdirAll(x.UserDir, 0o755); err != nil {
		return "", err
	}
	data := map[string]any{
		"protocol": u.Protocol,
		"id":       u.UUID,
		"email":    u.Username,
		"remark":   u.Remark,
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(x.UserDir, fmt.Sprintf("user-%d.json", u.ID))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	_ = appendToConfigClients(x.ConfigPath, u)
	return path, nil
}

type inbound struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Settings struct {
		Clients []map[string]any `json:"clients"`
	} `json:"settings"`
}

type xrayConfig struct {
	Inbounds []inbound `json:"inbounds"`
	Outbounds any     `json:"outbounds"`
}

func appendToConfigClients(path string, u XrayUser) error {
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg xrayConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	for i := range cfg.Inbounds {
		if cfg.Inbounds[i].Protocol == u.Protocol {
			client := map[string]any{"id": u.UUID, "email": u.Username}
			cfg.Inbounds[i].Settings.Clients = append(cfg.Inbounds[i].Settings.Clients, client)
			updated, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return err
			}
			return os.WriteFile(path, updated, 0o644)
		}
	}
	return fmt.Errorf("no inbound found for protocol %s", u.Protocol)
}

func ReloadXray() error {
    cmd := exec.Command("bash", "-c", "systemctl reload xray || systemctl restart xray")
    return cmd.Run()
}

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
    UserDir string
}

// WriteUser writes a per-user snippet for Xray to include.
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
    return path, nil
}

func ReloadXray() error {
    cmd := exec.Command("bash", "-c", "systemctl reload xray || systemctl restart xray")
    return cmd.Run()
}

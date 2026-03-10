package services

import (
    "fmt"
    "os/exec"
)

func CreateDropbearUser(username, password string) error {
    cmdStr := fmt.Sprintf("id -u %s >/dev/null 2>&1 || useradd -m -s /usr/sbin/nologin %s; echo '%s:%s' | chpasswd", username, username, username, password)
    cmd := exec.Command("bash", "-c", cmdStr)
    return cmd.Run()
}

func DeleteDropbearUser(username string) error {
    cmd := exec.Command("bash", "-c", fmt.Sprintf("userdel -r %s || true", username))
    return cmd.Run()
}

package ports

import (
    "bytes"
    "fmt"
    "log"
    "os/exec"
    "strings"
)

type Listener struct {
    Port int    `json:"port"`
    PID  string `json:"pid"`
    Cmd  string `json:"cmd"`
}

// List finds processes listening on a port using lsof.
func List(port int) ([]Listener, error) {
    cmd := exec.Command("bash", "-c", fmt.Sprintf("lsof -i :%d -sTCP:LISTEN -n -P", port))
    out, err := cmd.CombinedOutput()
    if err != nil {
        // lsof exits 1 when nothing found; treat empty output as none
        if len(out) == 0 {
            return []Listener{}, nil
        }
    }
    lines := strings.Split(string(out), "\n")
    var res []Listener
    for i, line := range lines {
        if i == 0 || strings.TrimSpace(line) == "" {
            continue
        }
        fields := strings.Fields(line)
        if len(fields) < 2 {
            continue
        }
        res = append(res, Listener{Cmd: fields[0], PID: fields[1], Port: port})
    }
    return res, nil
}

// Kill attempts SIGTERM then SIGKILL on PIDs.
func Kill(listeners []Listener) error {
    for _, l := range listeners {
        if l.PID == "" {
            continue
        }
        term := exec.Command("bash", "-c", "kill -15 "+l.PID)
        _ = term.Run()
    }
    for _, l := range listeners {
        if l.PID == "" {
            continue
        }
        kill := exec.Command("bash", "-c", "kill -0 "+l.PID)
        if err := kill.Run(); err == nil {
            log.Printf("PID %s still alive; sending SIGKILL", l.PID)
            _ = exec.Command("bash", "-c", "kill -9 "+l.PID).Run()
        }
    }
    return nil
}

// EnsureFree detects listeners and kills them, returning a log message slice.
func EnsureFree(port int) ([]Listener, error) {
    listeners, err := List(port)
    if err != nil {
        return listeners, err
    }
    if len(listeners) == 0 {
        return listeners, nil
    }
    var buf bytes.Buffer
    for _, l := range listeners {
        buf.WriteString(fmt.Sprintf("port %d held by pid %s (%s); ", port, l.PID, l.Cmd))
    }
    log.Printf("freeing port %d: %s", port, buf.String())
    if err := Kill(listeners); err != nil {
        return listeners, err
    }
    return listeners, nil
}

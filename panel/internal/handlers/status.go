package handlers

import (
    "net/http"
    "os"
    "strconv"
    "strings"
    "time"

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

func (h *StatusHandler) Resources(c *gin.Context) {
    memTotal, _, memAvailable := getMemInfo()
    rx, tx := h.GetNetDevRaw()
    cpuUsage := getCPUUsage()

    today := time.Now().Format("2006-01-02")
    thisMonth := time.Now().Format("2006-01")

    dayRx, dayTx := h.Store.GetNetworkUsageToday(today)
    monthRx, monthTx := h.Store.GetNetworkUsageMonth(thisMonth)
    totalRx, totalTx := h.Store.GetNetworkUsageTotal()

    c.JSON(http.StatusOK, gin.H{
        "cpu_percent": cpuUsage,
        "mem_total":   memTotal,
        "mem_used":    memTotal - memAvailable,
        "mem_percent": float64(memTotal-memAvailable) / float64(memTotal) * 100,
        "net_rx":      rx,
        "net_tx":      tx,
        "day_rx":      dayRx,
        "day_tx":      dayTx,
        "month_rx":    monthRx,
        "month_tx":    monthTx,
        "total_rx":    totalRx,
        "total_tx":    totalTx,
    })
}

// Helpers for reading procfs
func getMemInfo() (total, free, available uint64) {
    data, err := os.ReadFile("/proc/meminfo")
    if err != nil { return }
    lines := strings.Split(string(data), "\n")
    for _, line := range lines {
        fields := strings.Fields(line)
        if len(fields) < 2 { continue }
        val, _ := strconv.ParseUint(fields[1], 10, 64)
        switch fields[0] {
        case "MemTotal:": total = val * 1024
        case "MemFree:": free = val * 1024
        case "MemAvailable:": available = val * 1024
        }
    }
    // Fallback if MemAvailable is missing (older kernels)
    if available == 0 { available = free }
    return
}

func (h *StatusHandler) GetNetDevRaw() (rxBytes, txBytes uint64) {
    data, err := os.ReadFile("/proc/net/dev")
    if err != nil { return }
    lines := strings.Split(string(data), "\n")
    for _, line := range lines {
        if strings.Contains(line, "lo:") || strings.Contains(line, "Inter-") || strings.Contains(line, "face") { continue }
        fields := strings.Fields(line)
        if len(fields) < 10 { continue }
        rx, _ := strconv.ParseUint(fields[1], 10, 64)
        tx, _ := strconv.ParseUint(fields[9], 10, 64)
        rxBytes += rx
        txBytes += tx
    }
    return
}

func getCPUUsage() float64 {
    // Quick snapshot: wait 100ms between two reads
    idle1, total1 := readCPUStat()
    time.Sleep(100 * time.Millisecond)
    idle2, total2 := readCPUStat()
    
    idleDiff := float64(idle2 - idle1)
    totalDiff := float64(total2 - total1)
    if totalDiff == 0 { return 0 }
    
    return (1.0 - (idleDiff / totalDiff)) * 100.0
}

func readCPUStat() (idle, total uint64) {
    data, err := os.ReadFile("/proc/stat")
    if err != nil { return }
    lines := strings.Split(string(data), "\n")
    for _, line := range lines {
        if strings.HasPrefix(line, "cpu ") {
            fields := strings.Fields(line)
            for i, f := range fields {
                if i == 0 { continue }
                val, _ := strconv.ParseUint(f, 10, 64)
                total += val
                if i == 4 { idle = val } // idle is the 4th field after "cpu"
            }
            break
        }
    }
    return
}

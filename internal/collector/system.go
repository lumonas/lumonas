package collector

import (
	"bufio"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

var startedAt = time.Now()

func Metrics() model.SystemMetrics {
	m := model.SystemMetrics{UptimeSeconds: uint64(time.Since(startedAt).Seconds()), Net: model.NetMetrics{Interface: primaryInterface()}}
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if seconds, err := strconv.ParseFloat(fields[0], 64); err == nil {
				m.UptimeSeconds = uint64(seconds)
			}
		}
	}
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		for i := 0; i < 3 && i < len(fields); i++ {
			m.Load[i], _ = strconv.ParseFloat(fields[i], 64)
		}
	}
	if file, err := os.Open("/proc/meminfo"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		var total, available uint64
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 2 {
				continue
			}
			value, _ := strconv.ParseUint(fields[1], 10, 64)
			switch fields[0] {
			case "MemTotal:":
				total = value * 1024
			case "MemAvailable:":
				available = value * 1024
			}
		}
		m.RAMTotalBytes = total
		if total >= available {
			m.RAMUsedBytes = total - available
		}
	}
	if m.RAMTotalBytes == 0 {
		m.RAMTotalBytes = uint64(runtime.NumCPU()) * 1024 * 1024 * 1024
		m.RAMUsedBytes = m.RAMTotalBytes / 4
	}
	return m
}

func primaryInterface() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "unknown"
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			return iface.Name
		}
	}
	return "unknown"
}

func Hostname() string {
	value, err := os.Hostname()
	if err != nil {
		return "lumonas"
	}
	return value
}

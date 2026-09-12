package collector

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

var startedAt = time.Now()

var metricState struct {
	sync.Mutex
	cpuTotal uint64
	cpuIdle  uint64
	netRx    uint64
	netTx    uint64
	netAt    time.Time
	netIface string
	ifaces   map[string]netIfaceCounters
}

type netIfaceCounters struct {
	rx      uint64
	tx      uint64
	errsIn  uint64
	errsOut uint64
	dropIn  uint64
	dropOut uint64
	at      time.Time
}

func Metrics() model.SystemMetrics {
	m := model.SystemMetrics{UptimeSeconds: uint64(time.Since(startedAt).Seconds()), Net: model.NetMetrics{Interface: primaryInterface()}}
	now := time.Now()
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		total, idle := parseCPUStat(string(data))
		metricState.Lock()
		if metricState.cpuTotal > 0 && total >= metricState.cpuTotal && idle >= metricState.cpuIdle {
			totalDelta := total - metricState.cpuTotal
			idleDelta := idle - metricState.cpuIdle
			if totalDelta > 0 && idleDelta <= totalDelta {
				m.CPUPercent = 100 * float64(totalDelta-idleDelta) / float64(totalDelta)
			}
		}
		metricState.cpuTotal, metricState.cpuIdle = total, idle
		metricState.Unlock()
	}
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
	m.CPUTempC = cpuTemperature()
	m.Net.UpMbps, m.Net.DownMbps = networkThroughput(m.Net.Interface, now)
	m.NetInterfaces = netInterfaceMetrics(now)
	return m
}

func parseCPUStat(data string) (total, idle uint64) {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err == nil {
				total += value
			}
		}
		idle, _ = strconv.ParseUint(fields[4], 10, 64)
		if len(fields) > 5 {
			iowait, _ := strconv.ParseUint(fields[5], 10, 64)
			idle += iowait
		}
		return total, idle
	}
	return 0, 0
}

func networkThroughput(iface string, now time.Time) (upMbps, downMbps float64) {
	if iface == "" || iface == "unknown" {
		return 0, 0
	}
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	rx, tx, ok := parseNetworkBytes(string(data), iface)
	if !ok {
		return 0, 0
	}
	metricState.Lock()
	defer metricState.Unlock()
	if metricState.netIface == iface && !metricState.netAt.IsZero() && now.After(metricState.netAt) && rx >= metricState.netRx && tx >= metricState.netTx {
		seconds := now.Sub(metricState.netAt).Seconds()
		if seconds > 0 {
			down := float64(rx-metricState.netRx) * 8 / seconds / 1_000_000
			up := float64(tx-metricState.netTx) * 8 / seconds / 1_000_000
			upMbps, downMbps = up, down
		}
	}
	metricState.netIface, metricState.netRx, metricState.netTx, metricState.netAt = iface, rx, tx, now
	return upMbps, downMbps
}

func parseNetworkBytes(data, iface string) (rx, tx uint64, ok bool) {
	for _, line := range strings.Split(data, "\n") {
		name, values, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found || strings.TrimSpace(name) != iface {
			continue
		}
		fields := strings.Fields(values)
		if len(fields) < 9 {
			return 0, 0, false
		}
		rx, errRx := strconv.ParseUint(fields[0], 10, 64)
		tx, errTx := strconv.ParseUint(fields[8], 10, 64)
		return rx, tx, errRx == nil && errTx == nil
	}
	return 0, 0, false
}

func cpuTemperature() float64 {
	paths, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	var highest float64
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil {
			continue
		}
		if value > 1000 {
			value /= 1000
		}
		if value > highest && value < 150 {
			highest = value
		}
	}
	return highest
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

func netInterfaceMetrics(now time.Time) []model.NetInterfaceMetrics {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	upMap := make(map[string]bool, len(interfaces))
	for _, iface := range interfaces {
		upMap[iface.Name] = iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0
	}
	entries := parseAllNetDev(string(data))
	metricState.Lock()
	defer metricState.Unlock()
	if metricState.ifaces == nil {
		metricState.ifaces = make(map[string]netIfaceCounters)
	}
	result := make([]model.NetInterfaceMetrics, 0, len(entries))
	for name, e := range entries {
		if name == "lo" {
			continue
		}
		m := model.NetInterfaceMetrics{
			Interface: name,
			Up:        upMap[name],
			ErrorsIn:  e.errsIn,
			ErrorsOut: e.errsOut,
			DroppedIn: e.dropIn,
			DroppedOut: e.dropOut,
		}
		prev, ok := metricState.ifaces[name]
		if ok && !prev.at.IsZero() && now.After(prev.at) && e.rx >= prev.rx && e.tx >= prev.tx {
			seconds := now.Sub(prev.at).Seconds()
			if seconds > 0 {
				m.DownMbps = float64(e.rx-prev.rx) * 8 / seconds / 1_000_000
				m.UpMbps = float64(e.tx-prev.tx) * 8 / seconds / 1_000_000
			}
		}
		metricState.ifaces[name] = netIfaceCounters{
			rx: e.rx, tx: e.tx, errsIn: e.errsIn, errsOut: e.errsOut,
			dropIn: e.dropIn, dropOut: e.dropOut, at: now,
		}
		result = append(result, m)
	}
	return result
}

type netDevEntry struct {
	rx, tx             uint64
	errsIn, errsOut    uint64
	dropIn, dropOut    uint64
}

func parseAllNetDev(data string) map[string]netDevEntry {
	entries := make(map[string]netDevEntry)
	for _, line := range strings.Split(data, "\n") {
		name, values, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		fields := strings.Fields(values)
		if len(fields) < 12 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		errsIn, _ := strconv.ParseUint(fields[2], 10, 64)
		dropIn, _ := strconv.ParseUint(fields[3], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		errsOut, _ := strconv.ParseUint(fields[10], 10, 64)
		dropOut, _ := strconv.ParseUint(fields[11], 10, 64)
		entries[name] = netDevEntry{
			rx: rx, tx: tx, errsIn: errsIn, errsOut: errsOut,
			dropIn: dropIn, dropOut: dropOut,
		}
	}
	return entries
}

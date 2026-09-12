package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const runtimeTranscodePath = "/var/tmp/lumonas-transcode"

var zramDevicePattern = regexp.MustCompile(`^/dev/zram[0-9]+$`)

type zramState struct {
	Device          string
	SizeBytes       int64
	CompressedBytes int64
	Ratio           float64
	Pressure        string
}

// runRuntimeFromEnv is the only boot-time entry point for runtime
// provisioning. It deliberately reuses the same typed operations as the API
// broker, so startup cannot drift from the allow-listed settings path.
func runRuntimeFromEnv() error {
	request := request{Confirmed: true, OperationID: "runtime-boot", PlanHash: "runtime-boot"}
	if envBool("LUMONAS_ZRAM_ENABLED") {
		request.RequestedState = map[string]any{"sizeBytes": envOr("LUMONAS_ZRAM_SIZE_BYTES", "536870912")}
		if result := applyZram(request, commandRunner); !result.OK {
			return fmt.Errorf("zram provisioning failed: %s", result.Error)
		}
	} else if result := disableZram(request, commandRunner); !result.OK {
		return fmt.Errorf("zram disable failed: %s", result.Error)
	}
	if envBool("LUMONAS_TMPFS_ENABLED") {
		request.RequestedState = map[string]any{"sizeBytes": envOr("LUMONAS_TMPFS_SIZE_BYTES", "1073741824")}
		if result := applyTmpfs(request, commandRunner); !result.OK {
			return fmt.Errorf("tmpfs provisioning failed: %s", result.Error)
		}
	} else if result := disableTmpfs(request, commandRunner); !result.OK {
		return fmt.Errorf("tmpfs disable failed: %s", result.Error)
	}
	return nil
}

func envBool(key string) bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(key)), "true")
}

// runtimeStatus reports the live zram swap and tmpfs transcode state. All
// inputs are kernel interfaces and allow-listed binaries — no unvalidated
// data reaches the filesystem.
func runtimeStatus(run command) response {
	zram := map[string]any{"enabled": false, "sizeBytes": int64(0), "compressedBytes": int64(0), "ratio": 0.0, "pressure": "low"}
	state := readZramState(run)
	if state.Device != "" {
		zram["enabled"] = true
		zram["sizeBytes"] = state.SizeBytes
		zram["compressedBytes"] = state.CompressedBytes
		zram["ratio"] = state.Ratio
		zram["pressure"] = state.Pressure
	}
	tmpfs := map[string]any{"enabled": false, "sizeBytes": int64(0), "mountPath": runtimeTranscodePath}
	if size, mounted := readTmpfsSize(run, runtimeTranscodePath); mounted {
		tmpfs["enabled"] = true
		tmpfs["sizeBytes"] = size
	}
	return response{OK: true, Data: map[string]any{"zram": zram, "tmpfs": tmpfs}}
}

func readZramState(run command) zramState {
	var state zramState
	state.Pressure = "low"
	swaps, err := run("cat", "/proc/swaps")
	if err != nil {
		return zramState{}
	}
	for _, line := range strings.Split(string(swaps), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || !zramDevicePattern.MatchString(fields[0]) {
			continue
		}
		state.Device = fields[0]
		if parsed, parseErr := strconv.ParseInt(fields[2], 10, 64); parseErr == nil {
			// /proc/swaps reports total/used sizes in KiB.
			state.SizeBytes = parsed * 1024
			if used, usedErr := strconv.ParseInt(fields[3], 10, 64); usedErr == nil {
				state.CompressedBytes = used * 1024
			}
		}
		break
	}
	if state.Device == "" {
		return zramState{}
	}
	if state.SizeBytes > 0 && state.CompressedBytes > 0 {
		state.Ratio = float64(state.SizeBytes) / float64(state.CompressedBytes)
	}
	if memInfo, err := run("cat", "/proc/meminfo"); err == nil {
		total, available := parseMemInfo(string(memInfo))
		switch {
		case total > 0 && available < total/10:
			state.Pressure = "high"
		case total > 0 && available < total/4:
			state.Pressure = "medium"
		}
	}
	return state
}

func parseMemInfo(content string) (total, available int64) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = value * 1024
		case "MemAvailable:":
			available = value * 1024
		}
	}
	return total, available
}

func readTmpfsSize(run command, mountPath string) (int64, bool) {
	out, err := run("findmnt", "-b", "-rn", "-o", "FSTYPE,SIZE", "-T", mountPath)
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 || fields[0] != "tmpfs" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, true
	}
	return parsed, true
}

func requestedSizeBytes(values map[string]any) (int64, bool, error) {
	raw, ok := values["sizeBytes"]
	if !ok {
		return 0, false, nil
	}
	switch value := raw.(type) {
	case float64:
		return int64(value), true, nil
	case int64:
		return value, true, nil
	case int:
		return int64(value), true, nil
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, true, fmt.Errorf("sizeBytes must be an integer")
		}
		return parsed, true, nil
	default:
		return 0, true, fmt.Errorf("sizeBytes must be a number")
	}
}

// applyZram provisions compressed swap idempotently. Re-invocation while a
// zram device is already active is a no-op.
func applyZram(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	sizeBytes, present, err := requestedSizeBytes(req.RequestedState)
	if err != nil {
		return response{Error: err.Error()}
	}
	if !present {
		return response{Error: "sizeBytes is required"}
	}
	if sizeBytes < 64*1024*1024 || sizeBytes > 32*1024*1024*1024 {
		return response{Error: "zram sizeBytes must be between 64MiB and 32GiB"}
	}
	if existing := readZramState(run); existing.Device != "" {
		return response{OK: true, Data: map[string]any{"enabled": true, "device": existing.Device, "sizeBytes": existing.SizeBytes, "state": "unchanged"}}
	}
	if _, err := run("modprobe", "zram"); err != nil {
		return response{Error: "zram module could not be loaded"}
	}
	out, err := run("zramctl", "--find", "--size", strconv.FormatInt(sizeBytes, 10))
	if err != nil {
		return response{Error: "zram device allocation failed"}
	}
	device := ""
	for _, line := range strings.Split(string(out), "\n") {
		candidate := strings.TrimSpace(line)
		if zramDevicePattern.MatchString(candidate) {
			device = candidate
			break
		}
		if fields := strings.Fields(candidate); len(fields) > 0 && zramDevicePattern.MatchString(fields[0]) {
			device = fields[0]
			break
		}
	}
	if device == "" {
		return response{Error: "zram device allocation returned no device"}
	}
	if _, err := run("mkswap", device); err != nil {
		return response{Error: "zram mkswap failed"}
	}
	if _, err := run("swapon", "-p", "100", device); err != nil {
		return response{Error: "zram swapon failed"}
	}
	return response{OK: true, Data: map[string]any{"enabled": true, "device": device, "sizeBytes": sizeBytes, "state": "active"}}
}

// disableZram turns off any active zram swap.
func disableZram(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	state := readZramState(run)
	if state.Device == "" {
		return response{OK: true, Data: map[string]any{"enabled": false, "state": "unchanged"}}
	}
	if _, err := run("swapoff", state.Device); err != nil {
		return response{Error: "zram swapoff failed"}
	}
	if _, err := run("zramctl", "--reset", state.Device); err != nil {
		return response{Error: "zram reset failed"}
	}
	return response{OK: true, Data: map[string]any{"enabled": false, "state": "disabled"}}
}

// applyTmpfs provisions the bounded RAM transcode mount idempotently.
func applyTmpfs(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	sizeBytes, present, err := requestedSizeBytes(req.RequestedState)
	if err != nil {
		return response{Error: err.Error()}
	}
	if !present {
		return response{Error: "sizeBytes is required"}
	}
	if sizeBytes < 64*1024*1024 || sizeBytes > 64*1024*1024*1024 {
		return response{Error: "tmpfs sizeBytes must be between 64MiB and 64GiB"}
	}
	if _, mounted := readTmpfsSize(run, runtimeTranscodePath); mounted {
		return response{OK: true, Data: map[string]any{"enabled": true, "sizeBytes": sizeBytes, "state": "unchanged"}}
	}
	if _, err := run("mkdir", "-p", runtimeTranscodePath); err != nil {
		return response{Error: "tmpfs mount path could not be created"}
	}
	opts := fmt.Sprintf("size=%dM,mode=1777", sizeBytes/(1024*1024))
	if _, err := run("mount", "-t", "tmpfs", "-o", opts, "tmpfs", runtimeTranscodePath); err != nil {
		return response{Error: "tmpfs mount failed"}
	}
	return response{OK: true, Data: map[string]any{"enabled": true, "mountPath": runtimeTranscodePath, "sizeBytes": sizeBytes, "state": "active"}}
}

// disableTmpfs unmounts the RAM transcode mount if present.
func disableTmpfs(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	if _, mounted := readTmpfsSize(run, runtimeTranscodePath); !mounted {
		return response{OK: true, Data: map[string]any{"enabled": false, "state": "unchanged"}}
	}
	if _, err := run("umount", runtimeTranscodePath); err != nil {
		return response{Error: "tmpfs unmount failed"}
	}
	return response{OK: true, Data: map[string]any{"enabled": false, "state": "disabled"}}
}

// applyRuntimeConfig persists the desired runtime provisioning so the boot
// oneshot unit can re-apply it after every reboot.
func applyRuntimeConfig(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	configPath := "/etc/lumonas/runtime.env"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		return response{Error: "runtime config directory could not be created"}
	}
	var builder strings.Builder
	builder.WriteString("# Managed by LumoNAS - regenerated on runtime settings changes\n")
	if enabled, ok := req.RequestedState["zramEnabled"].(bool); ok && enabled {
		if size, err := validateRuntimeSize(req.RequestedState, "zramSizeBytes", 32*1024*1024*1024); err != nil {
			return response{Error: err.Error()}
		} else {
			builder.WriteString("LUMONAS_ZRAM_ENABLED=true\n")
			builder.WriteString("LUMONAS_ZRAM_SIZE_BYTES=" + strconv.FormatInt(size, 10) + "\n")
		}
	} else {
		builder.WriteString("LUMONAS_ZRAM_ENABLED=false\n")
	}
	if enabled, ok := req.RequestedState["tmpfsEnabled"].(bool); ok && enabled {
		if size, err := validateRuntimeSize(req.RequestedState, "tmpfsSizeBytes", 64*1024*1024*1024); err != nil {
			return response{Error: err.Error()}
		} else {
			builder.WriteString("LUMONAS_TMPFS_ENABLED=true\n")
			builder.WriteString("LUMONAS_TMPFS_SIZE_BYTES=" + strconv.FormatInt(size, 10) + "\n")
		}
	} else {
		builder.WriteString("LUMONAS_TMPFS_ENABLED=false\n")
	}
	content := builder.String()
	temporary, err := os.CreateTemp(filepath.Dir(configPath), ".runtime-env-*")
	if err != nil {
		return response{Error: "runtime config could not be written"}
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return response{Error: "runtime config could not be written"}
	}
	if err := temporary.Close(); err != nil {
		return response{Error: "runtime config could not be written"}
	}
	if err := os.Chmod(temporaryPath, 0o640); err != nil {
		return response{Error: "runtime config could not be written"}
	}
	if err := os.Rename(temporaryPath, configPath); err != nil {
		return response{Error: "runtime config could not be activated"}
	}
	_, _ = run("systemctl", "try-restart", "lumonas-runtime.service")
	return response{OK: true, Data: map[string]string{"configPath": configPath, "state": "active"}}
}

func validateRuntimeSize(values map[string]any, key string, maximum int64) (int64, error) {
	size, present, err := requestedSizeBytes(map[string]any{"sizeBytes": values[key]})
	if !present {
		return 0, fmt.Errorf("%s is required", key)
	}
	if err != nil {
		return 0, err
	}
	if size < 64*1024*1024 || size > maximum {
		return 0, fmt.Errorf("%s must be between 64MiB and %dGiB", key, maximum/(1024*1024*1024))
	}
	return size, nil
}

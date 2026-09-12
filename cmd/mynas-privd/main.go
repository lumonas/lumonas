package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
)

type request struct {
	Operation        string            `json:"operation"`
	OperationID      string            `json:"operationId,omitempty"`
	PlanHash         string            `json:"planHash"`
	TargetDiskID     string            `json:"targetDiskId,omitempty"`
	ExpectedIdentity map[string]string `json:"expectedIdentity,omitempty"`
	ExpectedState    map[string]string `json:"expectedState,omitempty"`
	RequestedState   map[string]any    `json:"requestedState,omitempty"`
	ExpiresAt        time.Time         `json:"expiresAt,omitempty"`
	Confirmed        bool              `json:"confirmed"`
}
type response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

type activeCheckpoint struct {
	stdin io.WriteCloser
	done  chan error
}

var checkpointState = struct {
	sync.Mutex
	items map[string]activeCheckpoint
}{items: make(map[string]activeCheckpoint)}

func main() {
	socket := flag.String("socket", "/run/mynas/privd.sock", "Unix socket path")
	flag.Parse()
	_ = os.Remove(*socket)
	if err := os.MkdirAll(filepath.Dir(*socket), 0o750); err != nil {
		panic(err)
	}
	listener, err := net.Listen("unix", *socket)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	_ = os.Chmod(*socket, 0o660)
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go serve(conn)
	}
}

func serve(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = encoder.Encode(response{Error: "invalid request"})
			continue
		}
		_ = encoder.Encode(execute(req, collector.Disks, commandRunner))
	}
}

type command func(string, ...string) ([]byte, error)

func commandRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func execute(req request, discover func(collector.CommandRunner) ([]model.Disk, error), run command) response {
	if req.PlanHash == "" {
		return response{Error: "planHash is required"}
	}
	if !req.ExpiresAt.IsZero() && !time.Now().UTC().Before(req.ExpiresAt) {
		return response{Error: "operation plan has expired"}
	}
	switch req.Operation {
	case "ping":
		return response{OK: true, Data: map[string]string{"service": "mynas-privd"}}
	case "disk.read-identities":
		disks, err := discover(nil)
		if err != nil {
			return response{Error: "disk identity discovery failed"}
		}
		return response{OK: true, Data: disks}
	case "filesystem.mount", "filesystem.unmount", "filesystem.create", "filesystem.format", "disk.erase":
		if req.TargetDiskID == "" {
			return response{Error: "targetDiskId is required for filesystem operations"}
		}
		if !req.Confirmed {
			return response{Error: "operation plan is not confirmed"}
		}
		disks, err := discover(nil)
		if err != nil {
			return response{Error: "disk identity discovery failed"}
		}
		var target *model.Disk
		for index := range disks {
			if disks[index].ID == req.TargetDiskID {
				target = &disks[index]
				break
			}
		}
		if target == nil {
			return response{Error: "stable disk identity is no longer present"}
		}
		if err := validateIdentity(*target, req.ExpectedIdentity); err != nil {
			return response{Error: err.Error()}
		}
		if (req.Operation == "filesystem.format" || req.Operation == "disk.erase") && (target.Mounted || target.PoolID != "") {
			return response{Error: "target disk is mounted or assigned to a pool"}
		}
		if (req.Operation == "filesystem.format" || req.Operation == "disk.erase") && deviceHasMounts(target.CurrentPath, run) {
			return response{Error: "target device or one of its partitions is mounted"}
		}
		return executeStorage(req, *target, run)
	case "service.reload":
		if !req.Confirmed {
			return response{Error: "operation plan is not confirmed"}
		}
		name := requestedString(req.RequestedState, "service")
		if !allowedService(name) {
			return response{Error: "service is not allow-listed"}
		}
		if _, err := run("systemctl", "reload", name); err != nil {
			return response{Error: "service reload failed"}
		}
		return response{OK: true, Data: map[string]string{"service": name}}
	case "snapraid.sync", "snapraid.scrub":
		if !req.Confirmed {
			return response{Error: "operation plan is not confirmed"}
		}
		configPath := requestedString(req.RequestedState, "configPath")
		if !safeSnapraidConfig(configPath) {
			return response{Error: "SnapRAID config path is not allow-listed"}
		}
		args := []string{"-c", configPath}
		if req.Operation == "snapraid.sync" {
			args = append(args, "sync")
		} else {
			percent := requestedString(req.RequestedState, "scrubPercent")
			if percent == "" {
				percent = "5"
			}
			value, err := strconv.Atoi(percent)
			if err != nil || value < 1 || value > 100 {
				return response{Error: "scrubPercent must be between 1 and 100"}
			}
			args = append(args, "scrub", "-p", strconv.Itoa(value))
		}
		if _, err := run("snapraid", args...); err != nil {
			return response{Error: "SnapRAID operation failed"}
		}
		return response{OK: true, Data: map[string]string{"operation": req.Operation, "configPath": configPath}}
	case "network.checkpoint.begin":
		return beginNetworkCheckpoint(req)
	case "network.checkpoint.commit", "network.checkpoint.rollback":
		return finishNetworkCheckpoint(req)
	case "power.action":
		if !req.Confirmed {
			return response{Error: "operation plan is not confirmed"}
		}
		action := requestedString(req.RequestedState, "action")
		if action != "poweroff" && action != "reboot" {
			return response{Error: "power action is not allow-listed"}
		}
		if _, err := run("systemctl", action); err != nil {
			return response{Error: "power action failed"}
		}
		return response{OK: true, Data: map[string]string{"action": action}}
	case "network.apply", "acl.apply":
		return response{Error: fmt.Sprintf("typed operation %q requires its dedicated checkpointed worker", req.Operation)}
	default:
		return response{Error: fmt.Sprintf("operation %q is not allow-listed", strings.TrimSpace(req.Operation))}
	}
}

func beginNetworkCheckpoint(req request) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	if !validOperationID(req.OperationID) {
		return response{Error: "operationId is required"}
	}
	checkpointState.Lock()
	_, exists := checkpointState.items[req.OperationID]
	checkpointState.Unlock()
	if exists {
		return response{Error: "network checkpoint already exists"}
	}
	uuid := requestedString(req.RequestedState, "connectionUuid")
	if !validUUID(uuid) {
		return response{Error: "connectionUuid is invalid"}
	}
	timeout := requestedInt(req.RequestedState, "timeoutSeconds", 60)
	if timeout < 30 || timeout > 900 {
		return response{Error: "timeoutSeconds must be between 30 and 900"}
	}
	changes, err := requestedChanges(req.RequestedState)
	if err != nil {
		return response{Error: err.Error()}
	}
	args := []string{"device", "checkpoint", "--timeout", strconv.Itoa(timeout)}
	devices := requestedStrings(req.RequestedState, "devices")
	for _, device := range devices {
		if !validDeviceName(device) {
			return response{Error: "network device name is invalid"}
		}
		args = append(args, device)
	}
	args = append(args, "--", "nmcli", "connection", "modify", "uuid", uuid)
	keys := make([]string, 0, len(changes))
	for key := range changes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, key, changes[key])
	}
	command := exec.Command("nmcli", args...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return response{Error: "network checkpoint input failed"}
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return response{Error: "NetworkManager checkpoint could not start"}
	}
	done := make(chan error, 1)
	checkpointState.Lock()
	checkpointState.items[req.OperationID] = activeCheckpoint{stdin: stdin, done: done}
	checkpointState.Unlock()
	go func() {
		err := command.Wait()
		done <- err
		checkpointState.Lock()
		delete(checkpointState.items, req.OperationID)
		checkpointState.Unlock()
	}()
	return response{OK: true, Data: map[string]any{"operationId": req.OperationID, "timeoutSeconds": timeout, "state": "pending-confirmation"}}
}

func finishNetworkCheckpoint(req request) response {
	if !req.Confirmed || !validOperationID(req.OperationID) {
		return response{Error: "confirmed operationId is required"}
	}
	checkpointState.Lock()
	checkpoint, ok := checkpointState.items[req.OperationID]
	checkpointState.Unlock()
	if !ok {
		return response{Error: "network checkpoint is no longer active"}
	}
	answer := "No\n"
	if req.Operation == "network.checkpoint.commit" {
		answer = "Yes\n"
	}
	if _, err := io.WriteString(checkpoint.stdin, answer); err != nil {
		return response{Error: "network checkpoint confirmation failed"}
	}
	_ = checkpoint.stdin.Close()
	return response{OK: true, Data: map[string]string{"operationId": req.OperationID, "state": strings.TrimPrefix(req.Operation, "network.checkpoint.")}}
}

func deviceHasMounts(path string, run command) bool {
	if path == "" {
		return true
	}
	output, err := run("findmnt", "-rn", "-S", path)
	return err == nil && strings.TrimSpace(string(output)) != ""
}

func validateIdentity(disk model.Disk, expected map[string]string) error {
	checks := map[string]string{"id": disk.ID, "wwn": disk.WWN, "serial": disk.Serial, "model": disk.Model, "filesystemUuid": disk.FilesystemUUID}
	for key, value := range expected {
		if actual, ok := checks[key]; ok && value != "" && actual != value {
			return fmt.Errorf("disk identity mismatch for %s", key)
		}
	}
	if value := expected["sizeBytes"]; value != "" {
		expectedSize, err := strconv.ParseUint(value, 10, 64)
		if err != nil || expectedSize != disk.SizeBytes {
			return fmt.Errorf("disk capacity mismatch")
		}
	}
	return nil
}

func executeStorage(req request, disk model.Disk, run command) response {
	path := disk.CurrentPath
	switch req.Operation {
	case "filesystem.mount":
		target := requestedString(req.RequestedState, "mountPath")
		if !safePath(target) {
			return response{Error: "mountPath must be under /srv/disks or /srv/pools"}
		}
		filesystem := requestedString(req.RequestedState, "filesystem")
		if filesystem != "" && filesystem != "ext4" && filesystem != "xfs" {
			return response{Error: "filesystem is not allow-listed"}
		}
		args := []string{path, target}
		if filesystem != "" {
			args = []string{"-t", filesystem, path, target}
		}
		if _, err := run("mount", args...); err != nil {
			return response{Error: "mount failed"}
		}
		return response{OK: true, Data: map[string]string{"path": path, "mountPath": target}}
	case "filesystem.unmount":
		target := requestedString(req.RequestedState, "mountPath")
		if !safePath(target) {
			return response{Error: "mountPath must be under /srv/disks or /srv/pools"}
		}
		if _, err := run("umount", "--", target); err != nil {
			return response{Error: "unmount failed"}
		}
		return response{OK: true, Data: map[string]string{"mountPath": target}}
	case "filesystem.format":
		filesystem := requestedString(req.RequestedState, "filesystem")
		if filesystem != "ext4" && filesystem != "xfs" {
			return response{Error: "format filesystem must be ext4 or xfs"}
		}
		binary := "mkfs." + filesystem
		args := []string{"-F", path}
		if filesystem == "xfs" {
			args = []string{"-f", path}
		}
		if _, err := run(binary, args...); err != nil {
			return response{Error: "filesystem format failed"}
		}
		return response{OK: true, Data: map[string]string{"filesystem": filesystem, "path": path}}
	case "disk.erase":
		if _, err := run("wipefs", "--all", "--force", path); err != nil {
			return response{Error: "disk erase failed"}
		}
		return response{OK: true, Data: map[string]string{"path": path}}
	}
	return response{Error: "storage operation is not implemented"}
}

func requestedString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func requestedInt(values map[string]any, key string, fallback int) int {
	value := values[key]
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		parsed, err := strconv.Atoi(typed)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func requestedStrings(values map[string]any, key string) []string {
	items, _ := values[key].([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			result = append(result, value)
		}
	}
	return result
}

func requestedChanges(values map[string]any) (map[string]string, error) {
	raw, _ := values["changes"].(map[string]any)
	result := make(map[string]string, len(raw))
	for key, value := range raw {
		if !allowedNetworkKey(key) {
			return nil, fmt.Errorf("network setting %q is not allow-listed", key)
		}
		text, ok := value.(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("network setting %q must be a non-empty string", key)
		}
		result[key] = text
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one network setting is required")
	}
	return result, nil
}

func allowedNetworkKey(value string) bool {
	switch value {
	case "ipv4.method", "ipv4.addresses", "ipv4.gateway", "ipv4.dns", "ipv4.dns-search", "ipv6.method", "ipv6.addresses", "ipv6.gateway", "ipv6.dns", "connection.autoconnect", "connection.metered", "802-3-ethernet.mtu":
		return true
	default:
		return false
	}
}

func validOperationID(value string) bool {
	return regexp.MustCompile(`^[a-zA-Z0-9._-]{1,128}$`).MatchString(value)
}
func validUUID(value string) bool {
	return regexp.MustCompile(`^[a-fA-F0-9-]{8,64}$`).MatchString(value)
}
func validDeviceName(value string) bool {
	return regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,32}$`).MatchString(value)
}

func safePath(value string) bool {
	return strings.HasPrefix(filepath.Clean(value), "/srv/disks/") || strings.HasPrefix(filepath.Clean(value), "/srv/pools/")
}

func allowedService(value string) bool {
	switch value {
	case "mynas-privd.service", "mynasd.service", "mynas-web.service", "docker.service", "smbd.service", "nfs-server.service", "ssh.service":
		return true
	default:
		return false
	}
}

func safeSnapraidConfig(value string) bool {
	clean := filepath.Clean(value)
	return clean == "/etc/snapraid.conf" || strings.HasPrefix(clean, "/etc/mynas/") || strings.HasPrefix(clean, "/var/lib/mynas/")
}

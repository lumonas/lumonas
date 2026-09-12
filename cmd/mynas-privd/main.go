package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
)

type request struct {
	Operation        string            `json:"operation"`
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
	case "network.apply", "acl.apply", "power.action":
		return response{Error: fmt.Sprintf("typed operation %q requires its dedicated checkpointed worker", req.Operation)}
	default:
		return response{Error: fmt.Sprintf("operation %q is not allow-listed", strings.TrimSpace(req.Operation))}
	}
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

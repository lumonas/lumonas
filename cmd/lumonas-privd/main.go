package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/network"
	"github.com/lumonas/lumonas/internal/power"
	"github.com/lumonas/lumonas/internal/privileged"
	commandrunner "github.com/lumonas/lumonas/internal/runner"
	"github.com/lumonas/lumonas/internal/storage"
)

type request struct {
	Operation        string            `json:"operation"`
	CorrelationID    string            `json:"correlationId,omitempty"`
	OperationID      string            `json:"operationId,omitempty"`
	PlanHash         string            `json:"planHash"`
	TargetDiskID     string            `json:"targetDiskId,omitempty"`
	ExpectedIdentity map[string]string `json:"expectedIdentity,omitempty"`
	ExpectedDisks    []expectedDisk    `json:"expectedDisks,omitempty"`
	ExpectedState    map[string]string `json:"expectedState,omitempty"`
	RequestedState   map[string]any    `json:"requestedState,omitempty"`
	ExpiresAt        time.Time         `json:"expiresAt,omitempty"`
	Confirmed        bool              `json:"confirmed"`
}
type expectedDisk struct {
	ID             string `json:"id"`
	WWN            string `json:"wwn,omitempty"`
	Serial         string `json:"serial,omitempty"`
	Model          string `json:"model,omitempty"`
	SizeBytes      uint64 `json:"sizeBytes"`
	GPTDiskGUID    string `json:"gptDiskGuid,omitempty"`
	PartitionUUID  string `json:"partitionUuid,omitempty"`
	FilesystemUUID string `json:"filesystemUuid,omitempty"`
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

var privilegedLogger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})).With("service", "lumonas-privd")

var workerDial = func(socket string) (net.Conn, error) {
	return net.DialTimeout("unix", socket, 3*time.Second)
}

var networkCheckpointCommand = exec.Command

var wireGuardApply = network.ApplyWireGuardConfigWithRunner

var tailscaleUp = network.TailscaleUpWithRunner
var tailscaleDown = network.TailscaleDownWithRunner
var tailscaleSetExitNode = network.TailscaleSetExitNodeWithRunner
var tailscaleClearExitNode = network.TailscaleClearExitNodeWithRunner

func main() {
	socket := flag.String("socket", "/run/lumonas/privd.sock", "Unix socket path")
	worker := flag.String("worker", "", "run as a restricted operation worker (storage, network, power, or general)")
	runtimeMode := flag.Bool("runtime", false, "apply the persisted runtime provisioning and exit")
	flag.Parse()
	if *runtimeMode {
		if err := runRuntimeFromEnv(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *worker != "" && !validWorker(*worker) {
		panic("unsupported privileged worker")
	}
	_ = os.Remove(*socket)
	if err := os.MkdirAll(filepath.Dir(*socket), 0o750); err != nil {
		panic(err)
	}
	listener, err := net.Listen("unix", *socket)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	mode := os.FileMode(0o660)
	if *worker != "" {
		mode = 0o600
	}
	_ = os.Chmod(*socket, mode)
	allowedGID := os.Getgid()
	var wg sync.WaitGroup
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		<-stop
		listener.Close()
		close(done)
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-done:
				wg.Wait()
				return
			default:
				continue
			}
		}
		if !peerAllowed(conn, allowedGID) {
			_ = conn.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if *worker == "" {
				serve(conn)
			} else {
				serveWorker(conn, *worker)
			}
		}()
	}
}

func serve(conn net.Conn) {
	defer conn.Close()
	scanner := privilegedScanner(conn)
	encoder := json.NewEncoder(conn)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			privilegedLogger.Warn("invalid privileged request", "error", err)
			_ = encoder.Encode(response{Error: "invalid request"})
			continue
		}
		privilegedLogger.Info("privileged request", privilegedRequestAttrs(req)...)
		result := executeBroker(req)
		resultAttrs := privilegedRequestAttrs(req)
		resultAttrs = append(resultAttrs, "ok", result.OK, "error", result.Error)
		privilegedLogger.Info("privileged result", resultAttrs...)
		_ = encoder.Encode(result)
	}
}

func serveWorker(conn net.Conn, worker string) {
	defer conn.Close()
	scanner := privilegedScanner(conn)
	encoder := json.NewEncoder(conn)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			privilegedLogger.Warn("invalid worker request", "worker", worker, "error", err)
			_ = encoder.Encode(response{Error: "invalid request"})
			continue
		}
		privilegedLogger.Info("worker request", append(privilegedRequestAttrs(req), "worker", worker)...)
		result := executeWorker(req, worker)
		resultAttrs := append(privilegedRequestAttrs(req), "worker", worker, "ok", result.OK, "error", result.Error)
		privilegedLogger.Info("worker result", resultAttrs...)
		_ = encoder.Encode(result)
	}
}

func privilegedScanner(conn net.Conn) *bufio.Scanner {
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), privileged.MaxIPCMessageBytes)
	return scanner
}

// privilegedRequestAttrs deliberately excludes RequestedState and ExpectedIdentity:
// those payloads can contain credentials or sensitive hardware metadata.
func privilegedRequestAttrs(req request) []any {
	return []any{"operation", req.Operation, "operation_id", req.OperationID, "correlation_id", req.CorrelationID, "plan_hash", req.PlanHash, "target_disk_id", req.TargetDiskID}
}

func executeBroker(req request) response {
	worker := operationWorker(req.Operation)
	if worker == "" {
		return execute(req, collector.Disks, commandRunner)
	}
	if req.PlanHash == "" {
		return response{Error: "planHash is required"}
	}
	if !req.ExpiresAt.IsZero() && !time.Now().UTC().Before(req.ExpiresAt) {
		return response{Error: "operation plan has expired"}
	}
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	return forwardToWorker(req, worker)
}

func forwardToWorker(req request, worker string) response {
	socket := filepath.Join(envOr("LUMONAS_PRIVD_WORKER_DIR", "/run/lumonas"), worker+".sock")
	connection, err := workerDial(socket)
	if err != nil {
		return response{Error: "privileged worker unavailable"}
	}
	defer connection.Close()
	if err := json.NewEncoder(connection).Encode(req); err != nil {
		return response{Error: "privileged worker request failed"}
	}
	var result response
	if err := json.NewDecoder(io.LimitReader(connection, privileged.MaxIPCMessageBytes)).Decode(&result); err != nil {
		return response{Error: "privileged worker response failed"}
	}
	return result
}

func executeWorker(req request, worker string) response {
	if operationWorker(req.Operation) != worker {
		return response{Error: "operation is not allow-listed for this worker"}
	}
	return execute(req, collector.Disks, commandRunner)
}

func operationWorker(operation string) string {
	switch operation {
	case "filesystem.mount", "filesystem.unmount", "filesystem.create", "filesystem.format", "disk.erase", "pool.mount", "pool.unmount", "snapraid.sync", "snapraid.scrub", "snapraid.fix", "snapraid.config.apply", "storage.mountpersist.apply", "runtime.zram.apply", "runtime.zram.disable", "runtime.tmpfs.apply", "runtime.tmpfs.disable", "runtime.config.apply", "snapshot.create", "snapshot.list", "snapshot.delete":
		return "storage"
	case "network.checkpoint.begin", "network.checkpoint.commit", "network.checkpoint.rollback", "network.wifi.connect", "network.wireguard.apply", "network.tailscale.up", "network.tailscale.down", "network.tailscale.exit-node", "network.wol.set", "network.wol.wake", "firewall.apply":
		return "network"
	case "power.action", "power.shutdown":
		return "power"
	case "service.reload", "service.config.apply", "identity.system-user.ensure", "samba.user.ensure", "acl.apply", "avahi.config.apply":
		return "general"
	default:
		return ""
	}
}

func validWorker(worker string) bool {
	return worker == "storage" || worker == "network" || worker == "power" || worker == "general"
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type command func(string, ...string) ([]byte, error)

var privilegedCommandTimeout = commandrunner.DefaultTimeout

func commandRunner(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), privilegedCommandTimeout)
	defer cancel()
	// Terminate the complete process group on timeout. A privileged utility
	// may spawn helpers that inherit stdout/stderr; leaving those descendants
	// alive would keep the broker request and its API job blocked.
	return commandrunner.CombinedOutputContext(ctx, name, args...)
}

func execute(req request, discover func(collector.CommandRunner) ([]model.Disk, error), run command) response {
	if req.PlanHash == "" {
		return response{Error: "planHash is required"}
	}
	if !req.ExpiresAt.IsZero() && !time.Now().UTC().Before(req.ExpiresAt) {
		return response{Error: "operation plan has expired"}
	}
	if req.Confirmed && requiresOperationID(req.Operation) && strings.TrimSpace(req.OperationID) == "" {
		return response{Error: "operationId is required"}
	}
	switch req.Operation {
	case "ping":
		return response{OK: true, Data: map[string]string{"service": "lumonas-privd"}}
	case "docker.read":
		return executeDockerRead(req)
	case "docker.command":
		return executeDockerCommand(req, run)
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
		if !model.HasStableDiskIdentity(target.ID) {
			return response{Error: "target disk has no stable identity"}
		}
		if err := validateIdentity(*target, req.ExpectedIdentity); err != nil {
			return response{Error: err.Error()}
		}
		if (req.Operation == "filesystem.format" || req.Operation == "filesystem.create" || req.Operation == "disk.erase") && (target.Mounted || target.PoolID != "") {
			return response{Error: "target disk is mounted or assigned to a pool"}
		}
		if req.Operation == "filesystem.format" || req.Operation == "filesystem.create" || req.Operation == "disk.erase" {
			mounted, mountErr := deviceHasMounts(target.CurrentPath, run)
			if mountErr != nil {
				return response{Error: "could not verify target mount state"}
			}
			if mounted {
				return response{Error: "target device or one of its partitions is mounted"}
			}
		}
		return executeStorage(req, *target, run)
	case "pool.mount":
		return executePoolMount(req, discover, run)
	case "pool.unmount":
		return executePoolUnmount(req, run)
	case "snapshot.create", "snapshot.list", "snapshot.delete":
		return executeSnapshotOperation(req, run)
	case "storage.mountpersist.apply":
		return applyMountPersistence(req, discover, run)
	case "snapraid.config.apply":
		return applySnapraidConfig(req, discover, run)
	case "runtime.status":
		return runtimeStatus(run)
	case "runtime.zram.apply":
		return applyZram(req, run)
	case "runtime.zram.disable":
		return disableZram(req, run)
	case "runtime.tmpfs.apply":
		return applyTmpfs(req, run)
	case "runtime.tmpfs.disable":
		return disableTmpfs(req, run)
	case "runtime.config.apply":
		return applyRuntimeConfig(req, run)
	case "service.reload":
		if !req.Confirmed {
			return response{Error: "operation plan is not confirmed"}
		}
		name := requestedString(req.RequestedState, "service")
		if !allowedService(name) {
			return response{Error: "service is not allow-listed"}
		}
		if _, err := run("systemctl", "reload-or-restart", name); err != nil {
			return response{Error: "service reload failed"}
		}
		return response{OK: true, Data: map[string]string{"service": name}}
	case "service.config.apply":
		return applyServiceConfig(req, run)
	case "firewall.apply":
		return applyFirewall(req, run)
	case "identity.system-user.ensure":
		return ensureSystemUser(req, run)
	case "samba.user.ensure":
		return ensureSambaUser(req, run, stdinCommandRunner)
	case "snapraid.sync", "snapraid.scrub", "snapraid.fix":
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
		} else if req.Operation == "snapraid.fix" {
			// Recovery from parity targets one data slot; the name is
			// validated so only managed dN slots can be fixed.
			dataName := requestedString(req.RequestedState, "dataName")
			if !regexp.MustCompile(`^d[0-9]+$`).MatchString(dataName) {
				return response{Error: "dataName must be a managed data slot (dN)"}
			}
			args = append(args, "fix", "-d", dataName)
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
	case "network.wifi.connect":
		return connectWiFi(req, stdinCommandRunner)
	case "network.wireguard.apply":
		return applyWireGuard(req)
	case "network.tailscale.up", "network.tailscale.down", "network.tailscale.exit-node":
		return applyTailscale(req)
	case "network.wol.set":
		if !req.Confirmed {
			return response{Error: "operation plan is not confirmed"}
		}
		iface := requestedString(req.RequestedState, "interface")
		if err := network.SetWOL(context.Background(), iface, requestedBool(req.RequestedState, "enabled"), func(_ context.Context, name string, args ...string) ([]byte, error) {
			return run(name, args...)
		}); err != nil {
			return response{Error: err.Error()}
		}
		return response{OK: true, Data: map[string]any{"interface": iface, "enabled": requestedBool(req.RequestedState, "enabled")}}
	case "network.wol.wake":
		if !req.Confirmed {
			return response{Error: "operation plan is not confirmed"}
		}
		iface := requestedString(req.RequestedState, "interface")
		mac := requestedString(req.RequestedState, "mac")
		if err := network.WakeHost(context.Background(), iface, mac, func(_ context.Context, name string, args ...string) ([]byte, error) {
			return run(name, args...)
		}); err != nil {
			return response{Error: err.Error()}
		}
		return response{OK: true, Data: map[string]any{"interface": iface, "mac": strings.ToLower(mac)}}
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
	case "power.shutdown":
		if !req.Confirmed {
			return response{Error: "operation plan is not confirmed"}
		}
		action := requestedString(req.RequestedState, "action")
		if err := power.ExecuteShutdown(context.Background(), action, func(_ context.Context, name string, args ...string) ([]byte, error) {
			return run(name, args...)
		}); err != nil {
			return response{Error: err.Error()}
		}
		return response{OK: true, Data: map[string]string{"action": action}}
	case "acl.apply":
		return applyACL(req, run)
	case "avahi.config.apply":
		return applyAvahiConfig(req, run)
	default:
		return response{Error: fmt.Sprintf("operation %q is not allow-listed", strings.TrimSpace(req.Operation))}
	}
}

func requiresOperationID(operation string) bool {
	switch operation {
	case "filesystem.mount", "filesystem.unmount", "filesystem.create", "filesystem.format", "disk.erase", "pool.mount", "pool.unmount", "storage.mountpersist.apply", "snapraid.config.apply", "snapraid.sync", "snapraid.scrub", "snapraid.fix", "network.checkpoint.begin", "network.checkpoint.commit", "network.checkpoint.rollback", "network.wifi.connect", "network.wireguard.apply", "network.tailscale.up", "network.tailscale.down", "network.tailscale.exit-node", "network.wol.set", "network.wol.wake", "firewall.apply", "service.reload", "service.config.apply", "avahi.config.apply", "identity.system-user.ensure", "samba.user.ensure", "acl.apply", "power.action", "power.shutdown", "runtime.zram.apply", "runtime.zram.disable", "runtime.tmpfs.apply", "runtime.tmpfs.disable", "runtime.config.apply", "snapshot.create", "snapshot.delete":
		return true
	default:
		return false
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
	checkpointContext, cancel := context.WithTimeout(context.Background(), time.Duration(timeout+10)*time.Second)
	command := networkCheckpointCommand("nmcli", args...)
	commandrunner.ConfigureProcessGroup(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		cancel()
		return response{Error: "network checkpoint input failed"}
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		cancel()
		return response{Error: "NetworkManager checkpoint could not start"}
	}
	done := make(chan error, 1)
	checkpointState.Lock()
	checkpointState.items[req.OperationID] = activeCheckpoint{stdin: stdin, done: done}
	checkpointState.Unlock()
	go func() {
		err := waitProcessGroup(checkpointContext, command)
		cancel()
		done <- err
		checkpointState.Lock()
		delete(checkpointState.items, req.OperationID)
		checkpointState.Unlock()
	}()
	return response{OK: true, Data: map[string]any{"operationId": req.OperationID, "timeoutSeconds": timeout, "state": "pending-confirmation"}}
}

func waitProcessGroup(ctx context.Context, command *exec.Cmd) error {
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		commandrunner.KillProcessGroup(command)
		return <-done
	}
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

// stdinRunner runs a command with optional standard-input content. Wi-Fi
// passwords are passed this way so they never appear in the process list.
type stdinRunner func(name string, args []string, stdin string) ([]byte, error)

func stdinCommandRunner(name string, args []string, stdin string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), privilegedCommandTimeout)
	defer cancel()
	return commandrunner.CombinedOutputContextWithStdin(ctx, strings.NewReader(stdin), name, args...)
}

func connectWiFi(req request, run stdinRunner) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	ssid := requestedString(req.RequestedState, "ssid")
	if len(ssid) < 1 || len(ssid) > 32 || strings.ContainsAny(ssid, "\x00\n\r") {
		return response{Error: "wifi ssid is invalid"}
	}
	ifname := requestedString(req.RequestedState, "ifname")
	if ifname != "" && !validDeviceName(ifname) {
		return response{Error: "network device name is invalid"}
	}
	timeout := requestedInt(req.RequestedState, "timeoutSeconds", 60)
	if timeout < 30 || timeout > 900 {
		return response{Error: "timeoutSeconds must be between 30 and 900"}
	}
	psk, secured := req.RequestedState["psk"]
	password, hasPassword := psk.(string)
	if secured && (!hasPassword || network.ValidateWiFiPSK(password) != nil) {
		return response{Error: "wifi psk is invalid"}
	}
	args := []string{"--wait", strconv.Itoa(timeout), "--ask", "device", "wifi", "connect", ssid}
	if ifname != "" {
		args = append(args, "ifname", ifname)
	}
	stdin := ""
	if secured {
		args = append(args, "--")
		stdin = password + "\n"
	}
	if _, err := run("nmcli", args, stdin); err != nil {
		return response{Error: "wifi connect failed"}
	}
	return response{OK: true, Data: map[string]any{"operationId": req.OperationID, "ssid": ssid, "state": "activating"}}
}

func applyWireGuard(req request) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	iface := requestedString(req.RequestedState, "interface")
	if !validDeviceName(iface) {
		return response{Error: "wireguard interface name is invalid"}
	}
	raw, ok := req.RequestedState["config"]
	if !ok {
		return response{Error: "wireguard config is required"}
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return response{Error: "wireguard config is invalid"}
	}
	var config network.WireGuardConfig
	if err := json.Unmarshal(encoded, &config); err != nil {
		return response{Error: "wireguard config is invalid"}
	}
	if config.Interface == "" {
		config.Interface = iface
	}
	if config.Interface != iface {
		return response{Error: "wireguard interface does not match the requested target"}
	}
	if err := wireGuardApply(context.Background(), iface, config, func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		return commandrunner.CombinedOutputContextWithStdin(ctx, stdin, name, args...)
	}); err != nil {
		return response{Error: err.Error()}
	}
	return response{OK: true, Data: map[string]any{"operationId": req.OperationID, "interface": iface, "state": "applied"}}
}

func applyTailscale(req request) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), privilegedCommandTimeout)
	defer cancel()
	run := network.TailscaleCommandRunner(commandrunner.OutputContext)
	switch req.Operation {
	case "network.tailscale.up":
		hostname := requestedString(req.RequestedState, "hostname")
		if err := network.ValidateTailscaleConfig(hostname); err != nil {
			return response{Error: err.Error()}
		}
		if err := tailscaleUp(ctx, hostname, requestedString(req.RequestedState, "authKey"), run); err != nil {
			return response{Error: err.Error()}
		}
		return response{OK: true, Data: map[string]any{"operationId": req.OperationID, "hostname": hostname, "state": "connected"}}
	case "network.tailscale.down":
		if err := tailscaleDown(ctx, run); err != nil {
			return response{Error: err.Error()}
		}
		return response{OK: true, Data: map[string]any{"operationId": req.OperationID, "state": "disconnected"}}
	case "network.tailscale.exit-node":
		peerIP := requestedString(req.RequestedState, "peerIp")
		if peerIP == "" {
			if err := tailscaleClearExitNode(ctx, run); err != nil {
				return response{Error: err.Error()}
			}
		} else {
			if net.ParseIP(peerIP) == nil {
				return response{Error: "peerIp must be a valid IP address"}
			}
			if err := tailscaleSetExitNode(ctx, peerIP, run); err != nil {
				return response{Error: err.Error()}
			}
		}
		return response{OK: true, Data: map[string]any{"operationId": req.OperationID, "peerIp": peerIP, "state": "updated"}}
	default:
		return response{Error: "tailscale operation is not allow-listed"}
	}
}

func deviceHasMounts(path string, run command) (bool, error) {
	if path == "" {
		return true, nil
	}
	output, err := run("findmnt", "-rn", "-S", path)
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return true, nil
	}
	// findmnt may return a non-zero status for an unmounted whole-disk source.
	// lsblk is the authoritative fallback because it also reports mounted
	// partitions beneath the target disk.
	output, err = run("lsblk", "-nrpo", "MOUNTPOINT", "--", path)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(output), "\n") {
		mountpoint := strings.TrimSpace(line)
		if mountpoint != "" && mountpoint != "-" {
			return true, nil
		}
	}
	return false, nil
}

func validateIdentity(disk model.Disk, expected map[string]string) error {
	checks := map[string]string{"id": disk.ID, "wwn": disk.WWN, "serial": disk.Serial, "model": disk.Model, "gptDiskGuid": disk.GPTDiskGUID, "partitionUuid": disk.PartitionUUID, "filesystemUuid": disk.FilesystemUUID}
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
		if requestedBool(req.RequestedState, "readOnly") {
			args = []string{"-o", "ro", path, target}
		}
		if filesystem != "" {
			args = []string{"-t", filesystem, path, target}
			if requestedBool(req.RequestedState, "readOnly") {
				args = []string{"-t", filesystem, "-o", "ro", path, target}
			}
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
	case "filesystem.create":
		filesystem := requestedString(req.RequestedState, "filesystem")
		if filesystem != "ext4" && filesystem != "xfs" {
			return response{Error: "filesystem must be ext4 or xfs"}
		}
		mountPath := requestedString(req.RequestedState, "mountPath")
		if filepath.Clean(mountPath) != storage.DiskBranchPath(disk.ID) {
			return response{Error: "mountPath must be the canonical /srv/disks/<disk-id> branch path"}
		}
		label := requestedString(req.RequestedState, "label")
		if label != "" && !storage.ValidFilesystemLabel(label) {
			return response{Error: "filesystem label is invalid"}
		}
		binary := "mkfs." + filesystem
		args := []string{}
		if filesystem == "ext4" {
			args = append(args, "-F")
		} else {
			args = append(args, "-f")
		}
		if label != "" {
			args = append(args, "-L", label)
		}
		args = append(args, path)
		if _, err := run(binary, args...); err != nil {
			return response{Error: "filesystem creation failed"}
		}
		if _, err := run("mkdir", "-p", mountPath); err != nil {
			return response{Error: "mount path could not be created"}
		}
		if _, err := run("mount", "-t", filesystem, path, mountPath); err != nil {
			return response{Error: "mount failed"}
		}
		return response{OK: true, Data: map[string]string{"filesystem": filesystem, "path": path, "mountPath": mountPath, "label": label}}
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

func executePoolMount(req request, discover func(collector.CommandRunner) ([]model.Disk, error), run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	if len(req.ExpectedDisks) == 0 {
		return response{Error: "expected pool disks are required"}
	}
	path := requestedString(req.RequestedState, "mountPath")
	if !safePoolPath(path) {
		return response{Error: "pool mount path is not allow-listed"}
	}
	branches := requestedStrings(req.RequestedState, "branches")
	if len(branches) != len(req.ExpectedDisks) {
		return response{Error: "pool branches do not match expected disks"}
	}
	actual, err := discover(nil)
	if err != nil {
		return response{Error: "disk identity discovery failed"}
	}
	byID := make(map[string]model.Disk, len(actual))
	for _, disk := range actual {
		byID[disk.ID] = disk
	}
	for index, expected := range req.ExpectedDisks {
		if !model.HasStableDiskIdentity(expected.ID) {
			return response{Error: "pool disk has no stable identity"}
		}
		disk, ok := byID[expected.ID]
		if !ok {
			return response{Error: "pool disk is no longer present"}
		}
		if err := validateExpectedDisk(disk, expected); err != nil {
			return response{Error: err.Error()}
		}
		if branches[index] != storage.DiskBranchPath(expected.ID) {
			return response{Error: "pool branch identity mismatch"}
		}
		mounted, err := run("findmnt", "-rn", "-T", branches[index])
		if err != nil || strings.TrimSpace(string(mounted)) == "" {
			return response{Error: "pool branch is not mounted: " + branches[index]}
		}
	}
	if mounted, err := run("findmnt", "-rn", "-T", path); err == nil && strings.TrimSpace(string(mounted)) != "" {
		return response{Error: "pool mount path is already mounted"}
	}
	if _, err := run("mkdir", "-p", path); err != nil {
		return response{Error: "pool mount path could not be created"}
	}
	if _, err := run("mount", "-t", "fuse.mergerfs", "-o", "defaults,allow_other,use_ino,category.create=mfs", strings.Join(branches, ":"), path); err != nil {
		return response{Error: "mergerfs pool mount failed"}
	}
	return response{OK: true, Data: map[string]any{"mountPath": path, "branches": branches}}
}

func executePoolUnmount(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	path := requestedString(req.RequestedState, "mountPath")
	if !safePoolPath(path) {
		return response{Error: "pool mount path is not allow-listed"}
	}
	mounted, err := run("findmnt", "-rn", "-o", "FSTYPE", "-T", path)
	if err != nil || strings.TrimSpace(string(mounted)) == "" {
		return response{Error: "pool mount path is not mounted"}
	}
	if !strings.Contains(strings.ToLower(string(mounted)), "mergerfs") {
		return response{Error: "refusing to unmount a non-mergerfs mount"}
	}
	if _, err := run("umount", "--", path); err != nil {
		return response{Error: "pool unmount failed"}
	}
	return response{OK: true, Data: map[string]string{"mountPath": path}}
}

func validateExpectedDisk(actual model.Disk, expected expectedDisk) error {
	if !model.HasStableDiskIdentity(expected.ID) || !model.HasStableDiskIdentity(actual.ID) {
		return fmt.Errorf("disk %q has no stable identity", expected.ID)
	}
	if expected.WWN != "" && actual.WWN != expected.WWN {
		return fmt.Errorf("pool disk %q WWN mismatch", expected.ID)
	}
	if expected.Serial != "" && actual.Serial != expected.Serial {
		return fmt.Errorf("pool disk %q serial mismatch", expected.ID)
	}
	if expected.Model != "" && actual.Model != expected.Model {
		return fmt.Errorf("pool disk %q model mismatch", expected.ID)
	}
	if expected.SizeBytes != 0 && actual.SizeBytes != expected.SizeBytes {
		return fmt.Errorf("pool disk %q capacity mismatch", expected.ID)
	}
	if expected.GPTDiskGUID != "" && actual.GPTDiskGUID != expected.GPTDiskGUID {
		return fmt.Errorf("pool disk %q GPT disk GUID mismatch", expected.ID)
	}
	if expected.PartitionUUID != "" && actual.PartitionUUID != expected.PartitionUUID {
		return fmt.Errorf("pool disk %q partition UUID mismatch", expected.ID)
	}
	if expected.FilesystemUUID != "" && actual.FilesystemUUID != expected.FilesystemUUID {
		return fmt.Errorf("pool disk %q filesystem UUID mismatch", expected.ID)
	}
	if actual.PoolID != "" {
		return fmt.Errorf("pool disk %q is already assigned to pool %q", expected.ID, actual.PoolID)
	}
	if actual.Health == model.Critical {
		return fmt.Errorf("pool disk %q is critically unhealthy", expected.ID)
	}
	if actual.Filesystem != "" && actual.Filesystem != "ext4" && actual.Filesystem != "xfs" {
		return fmt.Errorf("pool disk %q uses unsupported filesystem %q", expected.ID, actual.Filesystem)
	}
	return nil
}

func safePoolPath(value string) bool {
	clean := filepath.Clean(value)
	return strings.HasPrefix(clean, "/srv/pools/") && clean != "/srv/pools/" && !strings.Contains(strings.TrimPrefix(clean, "/srv/pools/"), "/")
}

func requestedString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func requestedBool(values map[string]any, key string) bool {
	value := values[key]
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(typed)
		return err == nil && parsed
	default:
		return false
	}
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
	switch items := values[key].(type) {
	case []string:
		return append([]string(nil), items...)
	case []any:
		result := make([]string, 0, len(items))
		for _, item := range items {
			if value, ok := item.(string); ok {
				result = append(result, value)
			}
		}
		return result
	default:
		return nil
	}
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
	case "ipv4.method", "ipv4.addresses", "ipv4.gateway", "ipv4.dns", "ipv4.dns-search", "ipv4.route-metric", "ipv4.routes", "ipv6.method", "ipv6.addresses", "ipv6.gateway", "ipv6.dns", "ipv6.dns-search", "ipv6.route-metric", "ipv6.routes", "connection.autoconnect", "connection.metered", "connection.type", "connection.master", "connection.members", "vlan.parent", "vlan.id", "802-3-ethernet.mtu":
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
	case "lumonas-privd.service", "lumonasd.service", "lumonas-web.service", "docker.service", "smbd.service", "nfs-server.service", "ssh.service", "rsync.service", "vsftpd.service":
		return true
	default:
		return false
	}
}

func applyServiceConfig(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	service := requestedString(req.RequestedState, "service")
	if !allowedService(service) {
		return response{Error: "service is not allow-listed"}
	}
	source := requestedString(req.RequestedState, "sourcePath")
	if source != "" {
		target, err := managedServiceConfigTarget(service, source)
		if err != nil {
			return response{Error: err.Error()}
		}
		if requestedBool(req.RequestedState, "remove") {
			if _, err := run("rm", "-f", target); err != nil {
				return response{Error: "service configuration removal failed"}
			}
		} else if _, err := run("install", "-D", "-m", "0640", source, target); err != nil {
			return response{Error: "service configuration activation failed"}
		}
	}
	if _, err := run("systemctl", "reload-or-restart", service); err != nil {
		return response{Error: "service configuration reload failed"}
	}
	return response{OK: true, Data: map[string]string{"service": service, "state": "reloaded"}}
}

func managedServiceConfigTarget(service, source string) (string, error) {
	clean := filepath.Clean(source)
	if !strings.HasPrefix(clean, "/var/lib/lumonas/generated/") {
		return "", fmt.Errorf("service configuration source is not allow-listed")
	}
	allowed := map[string]string{
		"nfs-server.service": "exports",
		"ssh.service":        "sshd-sftp.conf",
	}
	name, ok := allowed[service]
	if !ok {
		return "", fmt.Errorf("service configuration does not require privileged installation")
	}
	if filepath.Base(clean) != name {
		return "", fmt.Errorf("service configuration source does not match %s", service)
	}
	targets := map[string]string{
		"nfs-server.service": "/etc/exports.d/lumonas.exports",
		"ssh.service":        "/etc/ssh/sshd_config.d/90-lumonas-sftp.conf",
	}
	return targets[service], nil
}

func applyFirewall(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	path := requestedString(req.RequestedState, "configPath")
	if !safeFirewallConfig(path) {
		return response{Error: "firewall config path is not allow-listed"}
	}
	if _, err := run("nft", "-c", "-f", path); err != nil {
		return response{Error: "firewall validation failed"}
	}
	if _, err := run("nft", "-f", path); err != nil {
		return response{Error: "firewall activation failed"}
	}
	return response{OK: true, Data: map[string]string{"configPath": path, "state": "active"}}
}

func ensureSystemUser(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	name := requestedString(req.RequestedState, "name")
	if !validUnixName(name) {
		return response{Error: "system user name is invalid"}
	}
	if requestedBool(req.RequestedState, "disabled") {
		if _, err := run("usermod", "-L", name); err != nil {
			return response{Error: "system user lock failed"}
		}
		return response{OK: true, Data: map[string]string{"name": name, "state": "locked"}}
	}
	if requestedBool(req.RequestedState, "enable") {
		if _, err := run("usermod", "-U", name); err != nil {
			return response{Error: "system user unlock failed"}
		}
		return response{OK: true, Data: map[string]string{"name": name, "state": "unlocked"}}
	}
	// Account creation remains the default so existing callers keep working.
	create := true
	if _, exists := req.RequestedState["create"]; exists {
		create = requestedBool(req.RequestedState, "create")
	}
	if !create {
		return response{Error: "system user operation requires create, disabled, or enable"}
	}
	uid := requestedInt(req.RequestedState, "uid", 0)
	gid := requestedInt(req.RequestedState, "gid", 0)
	if uid < 100 || uid > 60000 || gid < 100 || gid > 60000 {
		return response{Error: "system user uid and gid must be between 100 and 60000"}
	}
	home := requestedString(req.RequestedState, "home")
	if home != "" && !safePath(home) {
		return response{Error: "system user home is not allow-listed"}
	}
	shell := requestedString(req.RequestedState, "shell")
	if shell == "" {
		shell = "/usr/sbin/nologin"
	}
	if shell != "/usr/sbin/nologin" && shell != "/bin/false" {
		return response{Error: "system user shell is not allow-listed"}
	}
	args := []string{"--system", "--uid", strconv.Itoa(uid), "--gid", strconv.Itoa(gid), "--shell", shell}
	if home != "" {
		args = append(args, "--home-dir", home)
	}
	args = append(args, name)
	if _, err := run("useradd", args...); err != nil {
		return response{Error: "system user configuration failed"}
	}
	return response{OK: true, Data: map[string]string{"name": name, "state": "configured"}}
}

// ensureSambaUser creates, rotates, disables, or enables a Samba account.
// Passwords are supplied on standard input (pdbedit -t) so they never appear
// in the process list or the broker log.
func ensureSambaUser(req request, run command, stdinRun stdinRunner) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	name := requestedString(req.RequestedState, "name")
	if !validUnixName(name) {
		return response{Error: "Samba user name is invalid"}
	}
	password, hasPassword := "", false
	if value, exists := req.RequestedState["password"]; exists {
		password, hasPassword = value.(string)
	}
	disabled := requestedBool(req.RequestedState, "disabled")
	if disabled && hasPassword && password != "" {
		return response{Error: "Samba user password and disabled are mutually exclusive"}
	}
	switch {
	case disabled:
		if _, err := run("pdbedit", "-u", name, "-d"); err != nil {
			return response{Error: "Samba user disable failed"}
		}
		return response{OK: true, Data: map[string]string{"name": name, "state": "disabled"}}
	case requestedBool(req.RequestedState, "enable"):
		if _, err := run("pdbedit", "-u", name, "-e"); err != nil {
			return response{Error: "Samba user enable failed"}
		}
		return response{OK: true, Data: map[string]string{"name": name, "state": "enabled"}}
	case requestedBool(req.RequestedState, "create"):
		if !validSambaPassword(password) {
			return response{Error: "Samba user password is invalid"}
		}
		if _, err := stdinRun("pdbedit", []string{"-a", "-t", "-u", name}, sambaPasswordStdin(password)); err != nil {
			return response{Error: "Samba user configuration failed"}
		}
		return response{OK: true, Data: map[string]string{"name": name, "state": "configured"}}
	case hasPassword && password != "":
		if !validSambaPassword(password) {
			return response{Error: "Samba user password is invalid"}
		}
		if _, err := stdinRun("pdbedit", []string{"-t", "-u", name}, sambaPasswordStdin(password)); err != nil {
			return response{Error: "Samba user password rotation failed"}
		}
		return response{OK: true, Data: map[string]string{"name": name, "state": "rotated"}}
	default:
		return response{Error: "Samba user operation requires password, create, disabled, or enable"}
	}
}

func sambaPasswordStdin(password string) string {
	return password + "\n" + password + "\n"
}

func validSambaPassword(password string) bool {
	if len(password) < 8 || len(password) > 128 {
		return false
	}
	return !strings.ContainsAny(password, "\x00\n\r")
}

func applyACL(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	path := requestedString(req.RequestedState, "path")
	if !safePath(path) {
		return response{Error: "ACL path is not allow-listed"}
	}
	entries, ok := req.RequestedState["entries"].([]any)
	if !ok || len(entries) == 0 || len(entries) > 1000 {
		return response{Error: "ACL entries must contain between 1 and 1000 items"}
	}
	args := []string{}
	if requestedBool(req.RequestedState, "recursive") {
		args = append(args, "-R")
	}
	args = append(args, "-m")
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			return response{Error: "ACL entry is invalid"}
		}
		principal, _ := entry["principal"].(string)
		level, _ := entry["level"].(string)
		if !validUnixName(principal) {
			return response{Error: "ACL principal is invalid"}
		}
		var permission string
		switch level {
		case "none":
			permission = ""
		case "read":
			permission = "r-X"
		case "write":
			permission = "rwX"
		default:
			return response{Error: "ACL level is invalid"}
		}
		if permission == "" {
			args = append(args, "u:"+principal+":")
		} else {
			args = append(args, "u:"+principal+":"+permission)
		}
	}
	args = append(args, path)
	if _, err := run("setfacl", args...); err != nil {
		return response{Error: "ACL application failed"}
	}
	return response{OK: true, Data: map[string]string{"path": path, "state": "applied"}}
}

func validUnixName(value string) bool {
	return regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,31}$`).MatchString(value)
}

func safeSnapraidConfig(value string) bool {
	clean := filepath.Clean(value)
	// The configured managed config path (shared with lumonasd) is always
	// acceptable; deployments that relocate it via env stay allow-listed.
	if configured := os.Getenv("LUMONAS_SNAPRAID_CONFIG"); configured != "" && clean == filepath.Clean(configured) {
		return true
	}
	return clean == "/etc/snapraid.conf" || strings.HasPrefix(clean, "/etc/lumonas/") || strings.HasPrefix(clean, "/var/lib/lumonas/")
}

func safeFirewallConfig(value string) bool {
	clean := filepath.Clean(value)
	return strings.HasPrefix(clean, "/etc/lumonas/") || strings.HasPrefix(clean, "/var/lib/lumonas/")
}

// requestedDataSlots decodes an explicit [{name, diskId}] mapping used by
// replacement flows so a retired disk's data name can be preserved.
func requestedDataSlots(values map[string]any, key string) ([]storage.DataSlot, bool) {
	items, ok := values[key].([]any)
	if !ok || len(items) == 0 {
		return nil, false
	}
	slots := make([]storage.DataSlot, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		name, _ := entry["name"].(string)
		diskID, _ := entry["diskId"].(string)
		if name == "" || diskID == "" {
			return nil, false
		}
		slots = append(slots, storage.DataSlot{Name: name, DiskID: diskID})
	}
	return slots, true
}

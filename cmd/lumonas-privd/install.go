package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	commandrunner "github.com/lumonas/lumonas/internal/runner"
)

// installCommandTimeout bounds the whole disk provisioning sequence. A real
// bootstrap takes minutes; the bound exists so a wedged installer cannot hold
// the storage worker forever.
var installCommandTimeout = 45 * time.Minute

// installScriptOverride allows tests and the QEMU harness to point the apply
// step at a stub instead of the packaged provisioner.
func installScriptPath() string {
	return envOr("LUMONAS_INSTALL_SCRIPT", "/usr/share/lumonas/install-disk")
}

var installHostnamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var installAdminPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// applyDiskInstall provisions the target disk as the LumoNAS system disk by
// delegating to the packaged provisioner script. The broker owns validation —
// target identity was revalidated by the caller, and the operator choices are
// re-checked here — while the script owns the exact partition, filesystem,
// bootstrap, and bootloader commands. The administrator credential never
// appears in argv: the password is delivered via stdin and stored only in the
// root-owned first-boot environment file of the installed system.
func applyDiskInstall(req request, disk model.Disk, run command, stdinRun stdinRunner) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	filesystem := requestedString(req.RequestedState, "filesystem")
	if filesystem != "ext4" && filesystem != "xfs" {
		return response{Error: "filesystem must be ext4 or xfs"}
	}
	hostname := requestedString(req.RequestedState, "hostname")
	if !installHostnamePattern.MatchString(hostname) {
		return response{Error: "hostname is invalid"}
	}
	adminName := requestedString(req.RequestedState, "adminUsername")
	if !installAdminPattern.MatchString(adminName) {
		return response{Error: "administrator name is invalid"}
	}
	adminPassword, _ := req.RequestedState["adminPassword"].(string)
	if len(adminPassword) < 12 || strings.ContainsAny(adminPassword, "\x00\r\n") {
		return response{Error: "administrator password must contain at least 12 characters"}
	}
	uefi := requestedBool(req.RequestedState, "uefi")
	if disk.Mounted || disk.PoolID != "" {
		return response{Error: "target disk is mounted or assigned to a pool"}
	}
	script := installScriptPath()
	info, err := os.Stat(script)
	if err != nil {
		return response{Error: "system installer is not available on this medium"}
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return response{Error: "system installer script is not executable"}
	}

	mode := "bios"
	if uefi {
		mode = "uefi"
	}
	ctx, cancel := context.WithTimeout(context.Background(), installCommandTimeout)
	defer cancel()
	output, err := commandrunner.CombinedOutputContextWithStdin(ctx, strings.NewReader(adminPassword+"\n"),
		script, disk.CurrentPath, filesystem, mode, hostname, adminName)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > 400 {
			detail = detail[len(detail)-400:]
		}
		if detail == "" {
			detail = err.Error()
		}
		return response{Error: fmt.Sprintf("disk installation failed: %s", detail)}
	}
	if err := ctx.Err(); err != nil {
		return response{Error: "disk installation timed out"}
	}
	return response{OK: true, Data: map[string]any{
		"device":     disk.CurrentPath,
		"filesystem": filesystem,
		"mode":       mode,
		"hostname":   hostname,
	}}
}

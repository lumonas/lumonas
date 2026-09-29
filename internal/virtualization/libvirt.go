// Package virtualization exposes a bounded management surface for VMs already
// defined by the host libvirt service.
package virtualization

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/lumonas/lumonas/internal/runner"
)

type CommandRunner func(context.Context, string, ...string) ([]byte, error)

type Service struct {
	Run            CommandRunner
	Virsh          string
	KVMPath        string
	MemInfo        string
	QEMUImg        string
	VMDir          string
	ISODir         string
	ConsoleRuntime ConsoleRuntime
}

type HostStatus struct {
	LibvirtAvailable bool   `json:"libvirtAvailable"`
	KVMAvailable     bool   `json:"kvmAvailable"`
	Architecture     string `json:"architecture"`
	CPUCount         int    `json:"cpuCount"`
	MemoryBytes      uint64 `json:"memoryBytes"`
	Reason           string `json:"reason,omitempty"`
}

type Domain struct {
	Name          string `json:"name"`
	UUID          string `json:"uuid,omitempty"`
	State         string `json:"state"`
	VCPUs         int    `json:"vcpus,omitempty"`
	MemoryKiB     uint64 `json:"memoryKiB,omitempty"`
	MaximumMemory uint64 `json:"maximumMemoryKiB,omitempty"`
}

var domainNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

func New(run CommandRunner) Service {
	if run == nil {
		run = runner.CombinedOutputContext
	}
	return Service{Run: run, Virsh: "virsh", KVMPath: "/dev/kvm", MemInfo: "/proc/meminfo", ConsoleRuntime: newConsoleRuntime()}
}

// WithMediaDirs relocates installation media and VM disk images. The packaged
// appliance keeps the libvirt image store defaults, so any override must still
// be writable by the daemon under the lumonasd systemd sandbox.
func WithMediaDirs(service Service, isoDir, vmDir string) Service {
	if isoDir != "" {
		service.ISODir = isoDir
	}
	if vmDir != "" {
		service.VMDir = vmDir
	}
	return service
}

func (s Service) command(ctx context.Context, args ...string) ([]byte, error) {
	name := s.Virsh
	if name == "" {
		name = "virsh"
	}
	return s.run(ctx, name, args...)
}

func (s Service) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if s.Run == nil {
		return runner.CombinedOutputContext(ctx, name, args...)
	}
	return s.Run(ctx, name, args...)
}

func (s Service) Status(ctx context.Context) HostStatus {
	status := HostStatus{Architecture: runtime.GOARCH, CPUCount: runtime.NumCPU()}
	kvm := s.KVMPath
	if kvm == "" {
		kvm = "/dev/kvm"
	}
	if info, err := os.Stat(kvm); err == nil && info.Mode()&os.ModeDevice != 0 {
		status.KVMAvailable = true
	}
	memPath := s.MemInfo
	if memPath == "" {
		memPath = "/proc/meminfo"
	}
	if file, err := os.Open(memPath); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				if value, parseErr := strconv.ParseUint(fields[1], 10, 64); parseErr == nil {
					status.MemoryBytes = value * 1024
				}
				break
			}
		}
		_ = file.Close()
	}
	if _, err := s.command(ctx, "uri"); err != nil {
		status.Reason = "libvirt is unavailable to the LumoNAS service account; install and enable libvirt to manage virtual machines"
		return status
	}
	status.LibvirtAvailable = true
	if !status.KVMAvailable {
		status.Reason = "libvirt is available, but /dev/kvm is missing; VMs may be limited to software emulation"
	}
	return status
}

func (s Service) Domains(ctx context.Context) ([]Domain, error) {
	output, err := s.command(ctx, "list", "--all", "--name")
	if err != nil {
		return nil, fmt.Errorf("list libvirt domains: %w", err)
	}
	domains := make([]Domain, 0)
	for _, name := range strings.Split(string(output), "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !domainNamePattern.MatchString(name) {
			return nil, errors.New("libvirt returned an invalid domain name")
		}
		info, infoErr := s.command(ctx, "dominfo", name)
		if infoErr != nil {
			return nil, fmt.Errorf("read VM %q details: %w", name, infoErr)
		}
		domains = append(domains, parseDomain(name, string(info)))
	}
	return domains, nil
}

func (s Service) Action(ctx context.Context, name, action string) (Domain, error) {
	if !domainNamePattern.MatchString(name) {
		return Domain{}, errors.New("VM name is invalid")
	}
	verb := ""
	switch action {
	case "start":
		verb = "start"
	case "shutdown":
		verb = "shutdown"
	case "reboot":
		verb = "reboot"
	case "suspend":
		verb = "suspend"
	case "resume":
		verb = "resume"
	default:
		return Domain{}, errors.New("unsupported VM action")
	}
	if _, err := s.command(ctx, verb, name); err != nil {
		return Domain{}, fmt.Errorf("%s VM %q: %w", action, name, err)
	}
	output, err := s.command(ctx, "dominfo", name)
	if err != nil {
		return Domain{Name: name, State: "transitioning"}, nil
	}
	return parseDomain(name, string(output)), nil
}

func parseDomain(name, raw string) Domain {
	result := Domain{Name: name, State: "unknown"}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.SplitN(line, ":", 2)
		if len(fields) != 2 {
			continue
		}
		key, value := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
		switch key {
		case "UUID":
			result.UUID = value
		case "State":
			result.State = strings.ToLower(value)
		case "CPU(s)":
			result.VCPUs, _ = strconv.Atoi(value)
		case "Used memory", "Max memory":
			fields := strings.Fields(value)
			if len(fields) == 0 {
				continue
			}
			memory, _ := strconv.ParseUint(fields[0], 10, 64)
			if key == "Used memory" {
				result.MemoryKiB = memory
			} else {
				result.MaximumMemory = memory
			}
		}
	}
	return result
}

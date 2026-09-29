package virtualization

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const MaxISOUploadBytes = 8 << 30

type Media struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
}

type CreateInput struct {
	Name      string `json:"name"`
	ISO       string `json:"iso"`
	VCPUs     int    `json:"vcpus"`
	MemoryMiB uint64 `json:"memoryMiB"`
	DiskGiB   uint64 `json:"diskGiB"`
}

type ProvisionedDomain struct {
	Domain
	DiskPath string `json:"diskPath"`
	ISO      string `json:"iso"`
}

func (s Service) mediaDir() string {
	if s.ISODir != "" {
		return s.ISODir
	}
	return "/var/lib/libvirt/images/lumonas"
}

func (s Service) vmDir() string {
	if s.VMDir != "" {
		return s.VMDir
	}
	return "/var/lib/libvirt/images/lumonas"
}

func safeMediaName(name string) bool {
	return name != "" && filepath.Base(name) == name && !strings.ContainsRune(name, '\\') && !strings.ContainsRune(name, 0) && strings.EqualFold(filepath.Ext(name), ".iso")
}

func (s Service) Media() ([]Media, error) {
	entries, err := os.ReadDir(s.mediaDir())
	if errors.Is(err, os.ErrNotExist) {
		return []Media{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read installation media: %w", err)
	}
	media := make([]Media, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !safeMediaName(entry.Name()) {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		media = append(media, Media{Name: entry.Name(), SizeBytes: info.Size()})
	}
	return media, nil
}

func (s Service) StoreISO(name string, source io.Reader) (Media, error) {
	if !safeMediaName(name) {
		return Media{}, errors.New("installation media must be a local .iso file with a simple filename")
	}
	if source == nil {
		return Media{}, errors.New("installation media is empty")
	}
	dir := s.mediaDir()
	if err := os.MkdirAll(dir, 0750); err != nil {
		return Media{}, fmt.Errorf("create installation media directory: %w", err)
	}
	path := filepath.Join(dir, name)
	if _, err := os.Lstat(path); err == nil {
		return Media{}, errors.New("installation media with this filename already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Media{}, fmt.Errorf("check installation media path: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".upload-*.iso")
	if err != nil {
		return Media{}, fmt.Errorf("create temporary media file: %w", err)
	}
	tempPath := temporary.Name()
	defer os.Remove(tempPath)
	if err := temporary.Chmod(0640); err != nil {
		_ = temporary.Close()
		return Media{}, fmt.Errorf("secure temporary media file: %w", err)
	}
	limited := &io.LimitedReader{R: source, N: MaxISOUploadBytes + 1}
	size, copyErr := io.Copy(temporary, limited)
	if copyErr != nil {
		_ = temporary.Close()
		return Media{}, fmt.Errorf("store installation media: %w", copyErr)
	}
	if size == 0 || size > MaxISOUploadBytes {
		_ = temporary.Close()
		return Media{}, errors.New("installation media must be between 1 byte and 8 GiB")
	}
	var descriptor [7]byte
	if size < 32_768+int64(len(descriptor)) {
		_ = temporary.Close()
		return Media{}, errors.New("file is too small to be an ISO-9660 installer image")
	}
	if _, err := temporary.ReadAt(descriptor[:], 32_768); err != nil || descriptor[0] != 1 || string(descriptor[1:6]) != "CD001" || descriptor[6] != 1 {
		_ = temporary.Close()
		return Media{}, errors.New("file does not contain a valid ISO-9660 primary volume descriptor")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return Media{}, fmt.Errorf("flush installation media: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Media{}, fmt.Errorf("close installation media: %w", err)
	}
	if err := os.Link(tempPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Media{}, errors.New("installation media with this filename already exists")
		}
		return Media{}, fmt.Errorf("publish installation media: %w", err)
	}
	return Media{Name: name, SizeBytes: size}, nil
}

func (s Service) Create(ctx context.Context, input CreateInput) (ProvisionedDomain, error) {
	if !domainNamePattern.MatchString(input.Name) {
		return ProvisionedDomain{}, errors.New("VM name must use 1–63 letters, numbers, dots, underscores, or hyphens")
	}
	if input.VCPUs < 1 || input.VCPUs > 64 {
		return ProvisionedDomain{}, errors.New("vCPUs must be between 1 and 64")
	}
	if input.MemoryMiB < 512 || input.MemoryMiB > 1_048_576 {
		return ProvisionedDomain{}, errors.New("memory must be between 512 MiB and 1 TiB")
	}
	if input.DiskGiB < 8 || input.DiskGiB > 16_384 {
		return ProvisionedDomain{}, errors.New("disk size must be between 8 GiB and 16 TiB")
	}
	host := s.Status(ctx)
	if !host.LibvirtAvailable {
		return ProvisionedDomain{}, errors.New("libvirt is unavailable to the LumoNAS service account")
	}
	if input.VCPUs > host.CPUCount {
		return ProvisionedDomain{}, fmt.Errorf("vCPUs cannot exceed the host's %d logical CPUs", host.CPUCount)
	}
	if host.MemoryBytes < 1536<<20 {
		return ProvisionedDomain{}, errors.New("host memory is unavailable or too small to safely create a VM")
	}
	availableMiB := host.MemoryBytes / (1024 * 1024)
	if input.MemoryMiB > availableMiB-1024 {
		return ProvisionedDomain{}, fmt.Errorf("leave at least 1 GiB for the host; maximum VM memory is %d MiB", availableMiB-1024)
	}
	if !safeMediaName(input.ISO) {
		return ProvisionedDomain{}, errors.New("select a valid uploaded ISO image")
	}
	iso := filepath.Join(s.mediaDir(), input.ISO)
	isoInfo, err := os.Lstat(iso)
	if err != nil || !isoInfo.Mode().IsRegular() {
		return ProvisionedDomain{}, errors.New("selected ISO image is no longer available")
	}
	if domains, listErr := s.Domains(ctx); listErr == nil {
		for _, domain := range domains {
			if strings.EqualFold(domain.Name, input.Name) {
				return ProvisionedDomain{}, errors.New("a VM with this name already exists")
			}
		}
	} else {
		return ProvisionedDomain{}, fmt.Errorf("verify existing VMs: %w", listErr)
	}
	vmDir := s.vmDir()
	if err := os.MkdirAll(vmDir, 0750); err != nil {
		return ProvisionedDomain{}, fmt.Errorf("create VM storage directory: %w", err)
	}
	freeBytes, err := availableStorageBytes(vmDir)
	if err != nil {
		return ProvisionedDomain{}, fmt.Errorf("check VM storage capacity: %w", err)
	}
	if input.DiskGiB > freeBytes/(1<<30) {
		return ProvisionedDomain{}, errors.New("requested virtual disk exceeds currently free VM storage; free space or choose a smaller disk")
	}
	disk := filepath.Join(vmDir, input.Name+".qcow2")
	xmlPath := filepath.Join(vmDir, input.Name+".xml")
	if _, err := os.Lstat(disk); err == nil {
		return ProvisionedDomain{}, errors.New("VM disk already exists; choose a different name")
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProvisionedDomain{}, fmt.Errorf("check VM disk path: %w", err)
	}
	qemuImg := s.QEMUImg
	if qemuImg == "" {
		qemuImg = "qemu-img"
	}
	if _, err := s.run(ctx, qemuImg, "create", "-f", "qcow2", disk, strconv.FormatUint(input.DiskGiB, 10)+"G"); err != nil {
		return ProvisionedDomain{}, fmt.Errorf("create VM disk: %w", err)
	}
	if _, err := os.Lstat(disk); err != nil {
		return ProvisionedDomain{}, errors.New("qemu-img reported success but did not create the VM disk")
	}
	if err := os.Chmod(disk, 0660); err != nil {
		_ = os.Remove(disk)
		return ProvisionedDomain{}, fmt.Errorf("set VM disk permissions: %w", err)
	}
	domainType := "kvm"
	if !host.KVMAvailable {
		domainType = "qemu"
	}
	xmlData, err := buildDomainXML(input, disk, iso, domainType)
	if err != nil {
		_ = os.Remove(disk)
		return ProvisionedDomain{}, err
	}
	if err := os.WriteFile(xmlPath, xmlData, 0600); err != nil {
		_ = os.Remove(disk)
		return ProvisionedDomain{}, fmt.Errorf("write VM definition: %w", err)
	}
	if _, err := s.command(ctx, "define", "--validate", xmlPath); err != nil {
		_ = os.Remove(disk)
		_ = os.Remove(xmlPath)
		return ProvisionedDomain{}, fmt.Errorf("define VM: %w", err)
	}
	if _, err := s.command(ctx, "start", input.Name); err != nil {
		_, _ = s.command(ctx, "undefine", input.Name)
		_ = os.Remove(disk)
		_ = os.Remove(xmlPath)
		return ProvisionedDomain{}, fmt.Errorf("start VM installer: %w", err)
	}
	output, err := s.command(ctx, "dominfo", input.Name)
	if err != nil {
		return ProvisionedDomain{Domain: Domain{Name: input.Name, State: "starting"}, DiskPath: disk, ISO: input.ISO}, nil
	}
	return ProvisionedDomain{Domain: parseDomain(input.Name, string(output)), DiskPath: disk, ISO: input.ISO}, nil
}

type domainXML struct {
	XMLName  xml.Name       `xml:"domain"`
	Type     string         `xml:"type,attr"`
	Name     string         `xml:"name"`
	Memory   xmlValue       `xml:"memory"`
	Current  xmlValue       `xml:"currentMemory"`
	VCPU     int            `xml:"vcpu"`
	OS       domainOS       `xml:"os"`
	Features domainFeatures `xml:"features"`
	Devices  domainDevices  `xml:"devices"`
}

type xmlValue struct {
	Unit  string `xml:"unit,attr"`
	Value uint64 `xml:",chardata"`
}

type domainOS struct {
	Type domainOSType `xml:"type"`
	Boot []domainBoot `xml:"boot"`
}

type domainOSType struct {
	Arch string `xml:"arch,attr"`
	Kind string `xml:",chardata"`
}

type domainBoot struct {
	Device string `xml:"dev,attr"`
}

type domainFeatures struct {
	ACPI string `xml:"acpi"`
	APIC string `xml:"apic"`
}

type domainDevices struct {
	Emulator string         `xml:"emulator,omitempty"`
	Disks    []domainDisk   `xml:"disk"`
	Ifaces   []domainIface  `xml:"interface"`
	Graphics domainGraphics `xml:"graphics"`
	Serial   domainConsole  `xml:"serial"`
	Console  domainConsole  `xml:"console"`
}

type domainDisk struct {
	Type   string `xml:"type,attr"`
	Device string `xml:"device,attr"`
	Driver struct {
		Name string `xml:"name,attr"`
		Type string `xml:"type,attr"`
	} `xml:"driver"`
	Source struct {
		File string `xml:"file,attr"`
	} `xml:"source"`
	Target struct {
		Dev string `xml:"dev,attr"`
		Bus string `xml:"bus,attr"`
	} `xml:"target"`
}

type domainIface struct {
	Type   string `xml:"type,attr"`
	Source struct {
		Network string `xml:"network,attr"`
	} `xml:"source"`
	Model struct {
		Type string `xml:"type,attr"`
	} `xml:"model"`
}

type domainGraphics struct {
	Type     string `xml:"type,attr"`
	AutoPort string `xml:"autoport,attr"`
	Listen   string `xml:"listen,attr"`
}

type domainConsole struct {
	Type   string `xml:"type,attr"`
	Target struct {
		Type string `xml:"type,attr"`
		Port string `xml:"port,attr"`
	} `xml:"target"`
}

func buildDomainXML(input CreateInput, disk, iso, domainType string) ([]byte, error) {
	arch := "x86_64"
	if runtime.GOARCH == "arm64" {
		arch = "aarch64"
	}
	var diskDriver struct {
		Name string `xml:"name,attr"`
		Type string `xml:"type,attr"`
	}
	diskDriver.Name, diskDriver.Type = "qemu", "qcow2"
	var diskSource struct {
		File string `xml:"file,attr"`
	}
	var diskTarget struct {
		Dev string `xml:"dev,attr"`
		Bus string `xml:"bus,attr"`
	}
	diskSource.File, diskTarget.Dev, diskTarget.Bus = disk, "vda", "virtio"
	var cdDriver struct {
		Name string `xml:"name,attr"`
		Type string `xml:"type,attr"`
	}
	cdDriver.Name, cdDriver.Type = "qemu", "raw"
	var cdSource struct {
		File string `xml:"file,attr"`
	}
	var cdTarget struct {
		Dev string `xml:"dev,attr"`
		Bus string `xml:"bus,attr"`
	}
	cdSource.File, cdTarget.Dev, cdTarget.Bus = iso, "sda", "sata"
	var ifaceSource struct {
		Network string `xml:"network,attr"`
	}
	var ifaceModel struct {
		Type string `xml:"type,attr"`
	}
	ifaceSource.Network, ifaceModel.Type = "default", "virtio"
	definition := domainXML{
		Type: domainType, Name: input.Name,
		Memory:  xmlValue{Unit: "MiB", Value: input.MemoryMiB},
		Current: xmlValue{Unit: "MiB", Value: input.MemoryMiB},
		VCPU:    input.VCPUs, OS: domainOS{Type: domainOSType{Arch: arch, Kind: "hvm"}, Boot: []domainBoot{{Device: "cdrom"}, {Device: "hd"}}},
		Features: domainFeatures{ACPI: "", APIC: ""},
		Devices: domainDevices{
			Disks: []domainDisk{
				{Type: "file", Device: "disk", Driver: diskDriver, Source: diskSource, Target: diskTarget},
				{Type: "file", Device: "cdrom", Driver: cdDriver, Source: cdSource, Target: cdTarget},
			},
			Ifaces:   []domainIface{{Type: "network", Source: ifaceSource, Model: ifaceModel}},
			Graphics: domainGraphics{Type: "vnc", AutoPort: "yes", Listen: "127.0.0.1"},
			Serial: domainConsole{Type: "pty", Target: struct {
				Type string `xml:"type,attr"`
				Port string `xml:"port,attr"`
			}{Type: "isa-serial", Port: "0"}},
			Console: domainConsole{Type: "pty", Target: struct {
				Type string `xml:"type,attr"`
				Port string `xml:"port,attr"`
			}{Type: "serial", Port: "0"}},
		},
	}
	data, err := xml.MarshalIndent(definition, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("build VM definition: %w", err)
	}
	return append([]byte(xml.Header), append(data, '\n')...), nil
}

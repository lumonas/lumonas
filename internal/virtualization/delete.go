package virtualization

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DeleteResult struct {
	Name               string   `json:"name"`
	DiskPath           string   `json:"diskPath"`
	DiskPaths          []string `json:"diskPaths,omitempty"`
	DiskRemoved        bool     `json:"diskRemoved"`
	DefinitionRetained bool     `json:"definitionRetained"`
}

type RecoverableDefinition struct {
	Name      string `json:"name"`
	DiskPath  string `json:"diskPath"`
	DiskBytes int64  `json:"diskBytes"`
}

// Delete removes a LumoNAS-managed libvirt definition. Guests must already be
// shut off; callers cannot use this operation to force-stop a running VM.
func (s Service) Delete(ctx context.Context, name string, deleteDisk bool) (DeleteResult, error) {
	if !domainNamePattern.MatchString(name) {
		return DeleteResult{}, errors.New("VM name is invalid")
	}
	vmDir := filepath.Clean(s.vmDir())
	if !filepath.IsAbs(vmDir) {
		return DeleteResult{}, errors.New("managed VM storage directory must be absolute")
	}
	definitionPath := filepath.Join(vmDir, name+".xml")
	definitionInfo, err := os.Lstat(definitionPath)
	if err != nil || !definitionInfo.Mode().IsRegular() {
		return DeleteResult{}, errors.New("VM is not a LumoNAS-managed guest")
	}
	if definitionInfo.Size() > 1<<20 {
		return DeleteResult{}, errors.New("managed VM definition is unexpectedly large")
	}
	definitionData, err := os.ReadFile(definitionPath)
	if err != nil {
		return DeleteResult{}, fmt.Errorf("read managed VM definition: %w", err)
	}
	managedDisks := make([]string, 0)
	definition, parseErr := parseManagedVMDefinition(name, vmDir, s.mediaDir(), definitionData)
	if parseErr != nil {
		return DeleteResult{}, parseErr
	}
	for _, disk := range definition.Devices.Disks {
		if disk.Device == "disk" {
			managedDisks = append(managedDisks, filepath.Clean(disk.Source.File))
		}
	}
	state, err := s.command(ctx, "domstate", name)
	if err != nil {
		return DeleteResult{}, fmt.Errorf("check VM state before deletion: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(string(state))) != "shut off" {
		return DeleteResult{}, errors.New("shut down the VM before deleting it")
	}
	for _, managedDisk := range managedDisks {
		info, statErr := os.Lstat(managedDisk)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return DeleteResult{}, fmt.Errorf("check managed VM disk before deletion: %w", statErr)
		}
		if statErr == nil && !info.Mode().IsRegular() {
			return DeleteResult{}, errors.New("managed VM disk is not a regular file")
		}
		if !deleteDisk && statErr != nil {
			return DeleteResult{}, errors.New("VM disk is missing and cannot be retained for recovery")
		}
	}
	if !deleteDisk {
		for _, disk := range definition.Devices.Disks {
			if disk.Device == "cdrom" && disk.Source.File != "" {
				if err := s.ValidateBackupMedia(filepath.Base(disk.Source.File), disk.Source.File); err != nil {
					return DeleteResult{}, fmt.Errorf("VM installer media cannot be retained safely: %w", err)
				}
			}
		}
	}
	if _, err := s.command(ctx, "undefine", name, "--managed-save", "--snapshots-metadata"); err != nil {
		return DeleteResult{}, fmt.Errorf("remove VM definition: %w", err)
	}
	result := DeleteResult{Name: name, DiskPaths: append([]string(nil), managedDisks...), DefinitionRetained: !deleteDisk}
	if len(managedDisks) > 0 {
		result.DiskPath = managedDisks[0]
	}
	if deleteDisk {
		if err := os.Remove(definitionPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, fmt.Errorf("remove saved VM definition: %w", err)
		}
		for _, managedDisk := range managedDisks {
			if err := os.Remove(managedDisk); err != nil && !errors.Is(err, os.ErrNotExist) {
				return result, fmt.Errorf("remove managed VM disk: %w", err)
			}
		}
		result.DiskRemoved = true
	}
	return result, nil
}

// RecoverableDefinitions finds managed VM definitions left behind by a
// definition-only delete. It excludes domains that are still defined in
// libvirt and requires both the XML and qcow2 disk to be regular files.
func (s Service) RecoverableDefinitions(ctx context.Context) ([]RecoverableDefinition, error) {
	domains, err := s.Domains(ctx)
	if err != nil {
		return nil, err
	}
	defined := make(map[string]bool, len(domains))
	for _, domain := range domains {
		defined[domain.Name] = true
	}
	vmDir := filepath.Clean(s.vmDir())
	entries, err := os.ReadDir(vmDir)
	if errors.Is(err, os.ErrNotExist) {
		return []RecoverableDefinition{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read managed VM image directory: %w", err)
	}
	recoverable := make([]RecoverableDefinition, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".xml" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".xml")
		if !domainNamePattern.MatchString(name) || defined[name] {
			continue
		}
		definitionPath := filepath.Join(vmDir, entry.Name())
		definitionInfo, statErr := os.Lstat(definitionPath)
		if statErr != nil || !definitionInfo.Mode().IsRegular() || definitionInfo.Size() > 1<<20 {
			continue
		}
		data, readErr := os.ReadFile(definitionPath)
		if readErr != nil {
			continue
		}
		definition, parseErr := parseManagedVMDefinition(name, vmDir, s.mediaDir(), data)
		if parseErr != nil {
			continue
		}
		if definition.Name != name {
			continue
		}
		var diskPath string
		var diskBytes int64
		complete := true
		for _, disk := range definition.Devices.Disks {
			if disk.Device != "disk" {
				if disk.Device == "cdrom" && disk.Source.File != "" {
					if err := s.ValidateBackupMedia(filepath.Base(disk.Source.File), disk.Source.File); err != nil {
						complete = false
						break
					}
				}
				continue
			}
			diskInfo, diskErr := os.Lstat(disk.Source.File)
			if diskErr != nil || !diskInfo.Mode().IsRegular() {
				complete = false
				break
			}
			if diskPath == "" {
				diskPath = disk.Source.File
			}
			diskBytes += diskInfo.Size()
		}
		if complete && diskPath != "" {
			recoverable = append(recoverable, RecoverableDefinition{Name: name, DiskPath: diskPath, DiskBytes: diskBytes})
		}
	}
	return recoverable, nil
}

func (s Service) RestoreDefinition(ctx context.Context, name string) (Domain, error) {
	if !domainNamePattern.MatchString(name) {
		return Domain{}, errors.New("VM name is invalid")
	}
	domains, err := s.Domains(ctx)
	if err != nil {
		return Domain{}, fmt.Errorf("check existing VM definitions: %w", err)
	}
	for _, domain := range domains {
		if domain.Name == name {
			return Domain{}, errors.New("a VM with this name is already defined")
		}
	}
	vmDir := filepath.Clean(s.vmDir())
	definitionPath := filepath.Join(vmDir, name+".xml")
	if !filepath.IsAbs(vmDir) {
		return Domain{}, errors.New("managed VM storage directory must be absolute")
	}
	definitionInfo, err := os.Lstat(definitionPath)
	if err != nil || !definitionInfo.Mode().IsRegular() || definitionInfo.Size() > 1<<20 {
		return Domain{}, errors.New("saved managed VM definition is unavailable")
	}
	definitionData, err := os.ReadFile(definitionPath)
	if err != nil {
		return Domain{}, fmt.Errorf("read saved VM definition: %w", err)
	}
	definition, err := parseManagedVMDefinition(name, vmDir, s.mediaDir(), definitionData)
	if err != nil {
		return Domain{}, err
	}
	if definition.Name != name {
		return Domain{}, errors.New("saved VM definition does not match the requested guest")
	}
	for _, disk := range definition.Devices.Disks {
		if disk.Device == "disk" {
			if _, err := os.Lstat(disk.Source.File); err != nil {
				return Domain{}, errors.New("saved VM disk is unavailable or unsafe")
			}
			if err := s.CheckBackupDisk(ctx, disk.Source.File); err != nil {
				return Domain{}, fmt.Errorf("saved VM disk verification failed: %w", err)
			}
		} else if disk.Device == "cdrom" && disk.Source.File != "" {
			if err := s.ValidateBackupMedia(filepath.Base(disk.Source.File), disk.Source.File); err != nil {
				return Domain{}, fmt.Errorf("saved VM installer media verification failed: %w", err)
			}
		}
	}
	if _, err := s.command(ctx, "define", "--validate", definitionPath); err != nil {
		return Domain{}, fmt.Errorf("restore VM definition: %w", err)
	}
	output, err := s.command(ctx, "dominfo", name)
	if err != nil {
		return Domain{Name: name, State: "shut off"}, nil
	}
	return parseDomain(name, string(output)), nil
}

func parseManagedVMDefinition(name, vmDir, mediaDir string, data []byte) (domainXML, error) {
	var definition domainXML
	if err := xml.Unmarshal(data, &definition); err != nil || definition.Name != name {
		return domainXML{}, errors.New("managed VM definition does not match the requested guest")
	}
	managedDisks := 0
	seenTargets := make(map[string]bool)
	seenSources := make(map[string]bool)
	for _, disk := range definition.Devices.Disks {
		switch disk.Device {
		case "disk":
			cleanPath := filepath.Clean(disk.Source.File)
			if filepath.Dir(cleanPath) != filepath.Clean(vmDir) || filepath.Ext(cleanPath) != ".qcow2" || !validDiskTarget(disk.Target.Dev) || seenTargets[disk.Target.Dev] || seenSources[cleanPath] {
				return domainXML{}, errors.New("VM disk is outside its LumoNAS-managed image path or has an invalid target")
			}
			seenTargets[disk.Target.Dev] = true
			seenSources[cleanPath] = true
			managedDisks++
		case "cdrom":
			if disk.Source.File != "" {
				cleanPath := filepath.Clean(disk.Source.File)
				if filepath.Dir(cleanPath) != filepath.Clean(mediaDir) || !safeMediaName(filepath.Base(cleanPath)) {
					return domainXML{}, errors.New("VM installer media is outside its LumoNAS-managed ISO directory")
				}
			}
		default:
			return domainXML{}, errors.New("VM definition contains an unsupported disk device")
		}
	}
	if managedDisks < 1 || managedDisks > 32 {
		return domainXML{}, errors.New("VM definition must contain between one and 32 managed disks")
	}
	return definition, nil
}

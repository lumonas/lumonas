package virtualization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxVMBackupDuration = 24 * time.Hour

type BackupArtifact struct {
	Name       string               `json:"name"`
	Disks      []BackupFileArtifact `json:"disks"`
	Definition BackupFileArtifact   `json:"definition"`
	Media      []BackupFileArtifact `json:"media"`
}

type BackupFileArtifact struct {
	Name     string `json:"name"`
	Target   string `json:"target,omitempty"`
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Checksum string `json:"checksum"`
}

type domainBackupXML struct {
	XMLName xml.Name              `xml:"domainbackup"`
	Disks   []domainBackupDiskXML `xml:"disks>disk"`
}

type domainBackupDiskXML struct {
	Name   string           `xml:"name,attr"`
	Type   string           `xml:"type,attr,omitempty"`
	Backup string           `xml:"backup,attr,omitempty"`
	Target *backupTargetXML `xml:"target,omitempty"`
	Driver *backupDriverXML `xml:"driver,omitempty"`
}

type backupTargetXML struct {
	File string `xml:"file,attr"`
}

type backupDriverXML struct {
	Type string `xml:"type,attr"`
}

type vmBackupDiskWork struct {
	definition domainDisk
	source     string
	staged     string
	published  string
}

// BackupManagedDisks makes full, point-in-time disk backups for the
// LumoNAS-managed guests. Running guests use libvirt's live block-backup API;
// stopped guests are converted offline. Artifacts are staged on the VM image
// filesystem for QEMU, then atomically published under recoveryRoot.
func (s Service) BackupManagedDisks(ctx context.Context, recoveryRoot, runID string) ([]BackupArtifact, error) {
	if !domainNamePattern.MatchString(runID) {
		return nil, errors.New("backup run id is invalid")
	}
	recoveryRoot = filepath.Clean(recoveryRoot)
	if !filepath.IsAbs(recoveryRoot) {
		return nil, errors.New("VM backup recovery directory must be absolute")
	}
	vmDir := filepath.Clean(s.vmDir())
	if !filepath.IsAbs(vmDir) {
		return nil, errors.New("managed VM storage directory must be absolute")
	}
	domains, err := s.Domains(ctx)
	if err != nil {
		entries, inventoryErr := os.ReadDir(vmDir)
		if errors.Is(inventoryErr, os.ErrNotExist) {
			return []BackupArtifact{}, nil
		}
		if inventoryErr == nil {
			managedDefinitions := false
			for _, entry := range entries {
				if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".xml") && domainNamePattern.MatchString(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))) {
					managedDefinitions = true
					break
				}
			}
			if !managedDefinitions {
				return []BackupArtifact{}, nil
			}
		}
		return nil, fmt.Errorf("list virtual machines for backup: %w", err)
	}
	if len(domains) == 0 {
		return []BackupArtifact{}, nil
	}
	workDir := filepath.Join(vmDir, ".lumonas-backup-"+runID)
	if err := os.Mkdir(workDir, 0700); err != nil {
		return nil, fmt.Errorf("create VM backup staging directory: %w", err)
	}
	defer os.RemoveAll(workDir)
	if err := prepareQemuBackupDirectory(workDir); err != nil {
		return nil, fmt.Errorf("prepare VM backup staging permissions: %w", err)
	}
	publishedDir := filepath.Join(recoveryRoot, "virtual-machines", runID)
	if _, err := os.Lstat(publishedDir); err == nil {
		return nil, errors.New("VM backup artifacts already exist for this run")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("check VM backup artifact path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(publishedDir), 0700); err != nil {
		return nil, fmt.Errorf("create VM backup recovery directory: %w", err)
	}
	if err := os.Mkdir(publishedDir, 0700); err != nil {
		return nil, fmt.Errorf("create VM backup run directory: %w", err)
	}
	published := make([]BackupArtifact, 0, len(domains))
	cleanupPublished := true
	defer func() {
		if cleanupPublished {
			_ = os.RemoveAll(publishedDir)
		}
	}()
	for _, domain := range domains {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		definitionPath := filepath.Join(vmDir, domain.Name+".xml")
		definitionInfo, statErr := os.Lstat(definitionPath)
		if statErr != nil || !definitionInfo.Mode().IsRegular() || definitionInfo.Size() > 1<<20 {
			return nil, fmt.Errorf("VM %q is not managed by LumoNAS; its disk cannot be included in a verified backup", domain.Name)
		}
		definitionData, readErr := os.ReadFile(definitionPath)
		if readErr != nil {
			return nil, fmt.Errorf("read managed VM %q definition: %w", domain.Name, readErr)
		}
		definition, parseErr := parseManagedVMDefinition(domain.Name, vmDir, s.mediaDir(), definitionData)
		if parseErr != nil {
			return nil, fmt.Errorf("VM %q definition is not safely recoverable: %w", domain.Name, parseErr)
		}
		if domain.State == "paused" {
			return nil, fmt.Errorf("VM %q is paused; resume or shut it down before backing up", domain.Name)
		}
		if domain.State != "running" && domain.State != "shut off" {
			return nil, fmt.Errorf("VM %q has unsupported state %q for backup", domain.Name, domain.State)
		}
		vmPublishedDir := filepath.Join(publishedDir, domain.Name)
		if err := os.MkdirAll(filepath.Join(vmPublishedDir, "disks"), 0700); err != nil {
			return nil, fmt.Errorf("create VM %q disk artifact directory: %w", domain.Name, err)
		}
		artifact := BackupArtifact{Name: domain.Name}
		seenTargets := make(map[string]bool)
		works := make([]vmBackupDiskWork, 0)
		var allocatedTotal uint64
		freeBytes, statErr := availableStorageBytes(vmDir)
		if statErr != nil {
			return nil, fmt.Errorf("check free VM backup space: %w", statErr)
		}
		publishedFreeBytes, statErr := availableStorageBytes(vmPublishedDir)
		if statErr != nil {
			return nil, fmt.Errorf("check recovery filesystem capacity: %w", statErr)
		}
		for _, disk := range definition.Devices.Disks {
			if disk.Device != "disk" {
				continue
			}
			target := disk.Target.Dev
			if !validDiskTarget(target) || seenTargets[target] {
				return nil, fmt.Errorf("VM %q has a missing or duplicate disk target", domain.Name)
			}
			seenTargets[target] = true
			sourceDisk := filepath.Clean(disk.Source.File)
			if filepath.Dir(sourceDisk) != vmDir || filepath.Ext(sourceDisk) != ".qcow2" {
				return nil, fmt.Errorf("VM %q disk %q is outside its managed qcow2 directory", domain.Name, target)
			}
			diskInfo, statErr := os.Lstat(sourceDisk)
			if statErr != nil || !diskInfo.Mode().IsRegular() {
				return nil, fmt.Errorf("VM %q disk %q is unavailable or unsafe", domain.Name, target)
			}
			allocatedBytes, statErr := allocatedFileBytes(diskInfo)
			if statErr != nil {
				return nil, fmt.Errorf("read VM disk allocation: %w", statErr)
			}
			if allocatedBytes > ^uint64(0)-allocatedTotal {
				return nil, fmt.Errorf("VM %q disk allocation overflows available capacity", domain.Name)
			}
			allocatedTotal += allocatedBytes
			stagedDisk := filepath.Join(workDir, domain.Name+"-"+target+".qcow2")
			publishedDisk := filepath.Join(vmPublishedDir, "disks", target+".qcow2")
			works = append(works, vmBackupDiskWork{definition: disk, source: sourceDisk, staged: stagedDisk, published: publishedDisk})
		}
		if allocatedTotal > freeBytes {
			return nil, fmt.Errorf("not enough free space to back up VM %q disks (%d bytes allocated, %d bytes free)", domain.Name, allocatedTotal, freeBytes)
		}
		if allocatedTotal > publishedFreeBytes {
			return nil, fmt.Errorf("not enough recovery filesystem space to back up VM %q disks (%d bytes allocated, %d bytes free)", domain.Name, allocatedTotal, publishedFreeBytes)
		}
		if domain.State == "running" {
			if err := s.backupRunningDisks(ctx, domain.Name, works); err != nil {
				return nil, fmt.Errorf("live backup of VM %q failed: %w", domain.Name, err)
			}
		} else {
			for _, work := range works {
				if _, err := s.run(ctx, s.qemuImg(), "convert", "-p", "-f", "qcow2", "-O", "qcow2", work.source, work.staged); err != nil {
					return nil, fmt.Errorf("offline backup of VM %q disk %q failed: %w", domain.Name, work.definition.Target.Dev, err)
				}
			}
		}
		publishedFreeBytes, statErr = availableStorageBytes(vmPublishedDir)
		if statErr != nil {
			return nil, fmt.Errorf("recheck recovery filesystem capacity: %w", statErr)
		}
		var stagedAllocated uint64
		for _, work := range works {
			info, statErr := os.Lstat(work.staged)
			if statErr != nil || !info.Mode().IsRegular() {
				return nil, fmt.Errorf("VM %q staged disk %q is unavailable", domain.Name, work.definition.Target.Dev)
			}
			allocated, statErr := allocatedFileBytes(info)
			if statErr != nil || allocated > ^uint64(0)-stagedAllocated {
				return nil, fmt.Errorf("VM %q staged disk allocation is invalid", domain.Name)
			}
			stagedAllocated += allocated
		}
		if stagedAllocated > publishedFreeBytes {
			return nil, fmt.Errorf("not enough recovery filesystem space for VM %q backup snapshot (%d bytes allocated, %d bytes free)", domain.Name, stagedAllocated, publishedFreeBytes)
		}
		for _, work := range works {
			target := work.definition.Target.Dev
			stagedDisk := work.staged
			if _, err := s.run(ctx, s.qemuImg(), "check", "-f", "qcow2", stagedDisk); err != nil {
				return nil, fmt.Errorf("verify VM %q disk %q backup image: %w", domain.Name, target, err)
			}
			publishedDisk := work.published
			if _, err := s.run(ctx, s.qemuImg(), "convert", "-p", "-f", "qcow2", "-O", "qcow2", stagedDisk, publishedDisk); err != nil {
				return nil, fmt.Errorf("publish VM %q disk %q backup image: %w", domain.Name, target, err)
			}
			if err := os.Chmod(publishedDisk, 0600); err != nil {
				return nil, fmt.Errorf("protect published VM %q disk %q backup image: %w", domain.Name, target, err)
			}
			if _, err := s.run(ctx, s.qemuImg(), "check", "-f", "qcow2", publishedDisk); err != nil {
				return nil, fmt.Errorf("verify published VM %q disk %q backup image: %w", domain.Name, target, err)
			}
			checksum, bytes, err := checksumFile(publishedDisk)
			if err != nil {
				return nil, err
			}
			artifact.Disks = append(artifact.Disks, BackupFileArtifact{Name: filepath.Base(work.source), Target: target, Path: publishedDisk, Bytes: bytes, Checksum: checksum})
		}
		mediaNames := make(map[string]bool)
		for _, disk := range definition.Devices.Disks {
			if disk.Device != "cdrom" || disk.Source.File == "" {
				continue
			}
			mediaPath := filepath.Clean(disk.Source.File)
			if filepath.Dir(mediaPath) != filepath.Clean(s.mediaDir()) || !safeMediaName(filepath.Base(mediaPath)) {
				return nil, fmt.Errorf("VM %q installer media is outside the managed ISO directory", domain.Name)
			}
			if mediaNames[filepath.Base(mediaPath)] {
				continue
			}
			mediaNames[filepath.Base(mediaPath)] = true
			mediaInfo, statErr := os.Lstat(mediaPath)
			if statErr != nil || !mediaInfo.Mode().IsRegular() {
				return nil, fmt.Errorf("VM %q installer media %q is unavailable or unsafe", domain.Name, filepath.Base(mediaPath))
			}
			if err := s.ValidateBackupMedia(filepath.Base(mediaPath), mediaPath); err != nil {
				return nil, fmt.Errorf("VM %q installer media %q is invalid: %w", domain.Name, filepath.Base(mediaPath), err)
			}
			publishedMediaDir := filepath.Join(vmPublishedDir, "media")
			if err := os.MkdirAll(publishedMediaDir, 0700); err != nil {
				return nil, fmt.Errorf("create VM %q media artifact directory: %w", domain.Name, err)
			}
			publishedMedia := filepath.Join(publishedMediaDir, filepath.Base(mediaPath))
			mediaFreeBytes, statErr := availableStorageBytes(publishedMediaDir)
			if statErr != nil {
				return nil, fmt.Errorf("check recovery space for VM %q installer media: %w", domain.Name, statErr)
			}
			if mediaInfo.Size() < 0 || uint64(mediaInfo.Size()) > mediaFreeBytes {
				return nil, fmt.Errorf("not enough recovery filesystem space for VM %q installer media", domain.Name)
			}
			if err := copyRegularFile(mediaPath, publishedMedia, 0600); err != nil {
				return nil, fmt.Errorf("publish VM %q installer media: %w", domain.Name, err)
			}
			checksum, bytes, err := checksumFile(publishedMedia)
			if err != nil {
				return nil, err
			}
			artifact.Media = append(artifact.Media, BackupFileArtifact{Name: filepath.Base(mediaPath), Path: publishedMedia, Bytes: bytes, Checksum: checksum})
		}
		publishedDefinition := filepath.Join(vmPublishedDir, "definition.xml")
		if err := writeFileAtomic(publishedDefinition, definitionData, 0600); err != nil {
			return nil, fmt.Errorf("publish VM %q recovery definition: %w", domain.Name, err)
		}
		definitionChecksum, definitionBytes, err := checksumFile(publishedDefinition)
		if err != nil {
			return nil, err
		}
		artifact.Definition = BackupFileArtifact{Name: "definition.xml", Path: publishedDefinition, Bytes: definitionBytes, Checksum: definitionChecksum}
		published = append(published, artifact)
	}
	cleanupPublished = false
	return published, nil
}

func (s Service) backupRunningDisks(ctx context.Context, domain string, disks []vmBackupDiskWork) error {
	if len(disks) == 0 {
		return errors.New("VM has no disks to back up")
	}
	backupDisks := make([]domainBackupDiskXML, 0, len(disks))
	var totalCapacity uint64
	for _, disk := range disks {
		target := disk.definition.Target.Dev
		info, err := s.command(ctx, "domblkinfo", domain, target)
		if err != nil {
			return fmt.Errorf("read virtual disk %q capacity: %w", target, err)
		}
		capacity, err := parseBlockCapacity(string(info))
		if err != nil {
			return fmt.Errorf("disk %q: %w", target, err)
		}
		if capacity > ^uint64(0)-totalCapacity {
			return errors.New("VM virtual disk capacity overflows available storage")
		}
		totalCapacity += capacity
		if _, err := s.run(ctx, s.qemuImg(), "create", "-f", "qcow2", disk.staged, strconv.FormatUint(capacity, 10)); err != nil {
			return fmt.Errorf("prepare live backup image for disk %q: %w", target, err)
		}
		if err := prepareQemuBackupFile(disk.staged); err != nil {
			return fmt.Errorf("prepare live backup image permissions for disk %q: %w", target, err)
		}
		backupDisks = append(backupDisks, domainBackupDiskXML{Name: target, Type: "file", Backup: "yes", Target: &backupTargetXML{File: disk.staged}, Driver: &backupDriverXML{Type: "qcow2"}})
	}
	freeBytes, err := availableStorageBytes(filepath.Dir(disks[0].staged))
	if err != nil {
		return fmt.Errorf("check space for live VM backup targets: %w", err)
	}
	if totalCapacity > freeBytes {
		return fmt.Errorf("live VM backup could require %d bytes but only %d bytes are free for its disk targets", totalCapacity, freeBytes)
	}
	backupXML, err := xml.Marshal(domainBackupXML{Disks: backupDisks})
	if err != nil {
		return err
	}
	backupXMLPath := disks[0].staged + ".xml"
	if err := os.WriteFile(backupXMLPath, append([]byte(xml.Header), backupXML...), 0600); err != nil {
		return fmt.Errorf("write libvirt backup definition: %w", err)
	}
	if _, err := s.command(ctx, "backup-begin", domain, backupXMLPath, "--reuse-external"); err != nil {
		return fmt.Errorf("start live disk backup: %w", err)
	}
	return s.waitForBackupJob(ctx, domain)
}

func validDiskTarget(target string) bool {
	if len(target) < 1 || len(target) > 16 || !asciiAlphaNumeric(target[0]) {
		return false
	}
	for index := 1; index < len(target); index++ {
		value := target[index]
		if !asciiAlphaNumeric(value) && value != '_' && value != '.' && value != '-' {
			return false
		}
	}
	return true
}

func asciiAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func (s Service) waitForBackupJob(ctx context.Context, domain string) error {
	deadlineCtx, cancel := context.WithTimeout(ctx, maxVMBackupDuration)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		status, err := s.command(deadlineCtx, "domjobinfo", domain)
		if err != nil {
			if deadlineCtx.Err() != nil {
				s.abortBackup(domain)
				return deadlineCtx.Err()
			}
			s.abortBackup(domain)
			return fmt.Errorf("read live backup status: %w", err)
		}
		if strings.Contains(string(status), "Job type: None") {
			completed, completedErr := s.command(deadlineCtx, "domjobinfo", domain, "--completed", "--keep-completed")
			if completedErr != nil {
				s.abortBackup(domain)
				return fmt.Errorf("read completed backup status: %w", completedErr)
			}
			text := string(completed)
			if strings.Contains(text, "Operation: Backup") && strings.Contains(text, "Job type: Completed") {
				return nil
			}
			return errors.New("libvirt backup ended without a successful completed-backup record")
		}
		select {
		case <-deadlineCtx.Done():
			s.abortBackup(domain)
			return deadlineCtx.Err()
		case <-ticker.C:
		}
	}
}

func (s Service) abortBackup(domain string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _ = s.command(ctx, "domjobabort", domain)
}

func (s Service) qemuImg() string {
	if s.QEMUImg != "" {
		return s.QEMUImg
	}
	return "qemu-img"
}

func parseBlockCapacity(output string) (uint64, error) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.SplitN(line, ":", 2)
		if len(fields) != 2 || strings.TrimSpace(fields[0]) != "Capacity" {
			continue
		}
		value := strings.Fields(strings.TrimSpace(fields[1]))
		if len(value) == 0 {
			break
		}
		capacity, err := strconv.ParseUint(value[0], 10, 64)
		if err != nil || capacity == 0 {
			break
		}
		return capacity, nil
	}
	return 0, errors.New("libvirt did not report a valid VM disk capacity")
}

func writeFileAtomic(target string, contents []byte, mode os.FileMode) error {
	output, err := os.CreateTemp(filepath.Dir(target), ".vm-definition-*")
	if err != nil {
		return err
	}
	temporary := output.Name()
	defer os.Remove(temporary)
	if err := output.Chmod(mode); err != nil {
		_ = output.Close()
		return err
	}
	if _, err := output.Write(contents); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, target)
}

func checksumFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	bytes, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), bytes, nil
}

// ValidateBackupDefinition confirms that a recovered domain XML is a safe
// LumoNAS-managed definition and points only at its managed VM disk path.
func (s Service) ValidateBackupDefinition(name string, data []byte) error {
	if !domainNamePattern.MatchString(name) {
		return errors.New("VM name is invalid")
	}
	definition, err := parseManagedVMDefinition(name, filepath.Clean(s.vmDir()), filepath.Clean(s.mediaDir()), data)
	if err != nil {
		return err
	}
	if definition.Name != name {
		return errors.New("VM backup definition does not match the requested guest")
	}
	return nil
}

func (s Service) ValidateBackupDefinitionInventory(name string, data []byte, disks map[string]string, mediaNames []string) error {
	if err := s.ValidateBackupDefinition(name, data); err != nil {
		return err
	}
	definition, err := parseManagedVMDefinition(name, filepath.Clean(s.vmDir()), filepath.Clean(s.mediaDir()), data)
	if err != nil {
		return err
	}
	expectedDisks := make(map[string]string)
	expectedMedia := make(map[string]bool)
	for _, disk := range definition.Devices.Disks {
		if disk.Device == "disk" {
			expectedDisks[disk.Target.Dev] = filepath.Base(disk.Source.File)
		} else if disk.Device == "cdrom" && disk.Source.File != "" {
			expectedMedia[filepath.Base(disk.Source.File)] = true
		}
	}
	for target, filename := range disks {
		if expectedDisks[target] == "" || expectedDisks[target] != filename {
			return errors.New("VM backup manifest disk list does not match its definition")
		}
	}
	providedMedia := make(map[string]bool, len(mediaNames))
	for _, value := range mediaNames {
		if providedMedia[value] || !expectedMedia[value] {
			return errors.New("VM backup manifest installer media list does not match its definition")
		}
		providedMedia[value] = true
	}
	if len(disks) != len(expectedDisks) || len(providedMedia) != len(expectedMedia) {
		return errors.New("VM backup manifest is missing a disk or installer media file")
	}
	return nil
}

func copyRegularFile(source, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("source is not a regular file")
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		_ = os.Remove(target)
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		_ = os.Remove(target)
		return err
	}
	if err := output.Close(); err != nil {
		_ = os.Remove(target)
		return err
	}
	return nil
}

// CheckBackupDisk runs qemu-img's structural check on a downloaded qcow2 disk.
func (s Service) CheckBackupDisk(ctx context.Context, path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("VM backup disk is unavailable or unsafe")
	}
	if _, err := s.run(ctx, s.qemuImg(), "check", "-f", "qcow2", path); err != nil {
		return fmt.Errorf("VM backup disk verification failed: %w", err)
	}
	return nil
}

func (s Service) ValidateBackupMedia(name, path string) error {
	if !safeMediaName(name) {
		return errors.New("VM backup installer media filename is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxISOUploadBytes || info.Size() < 32_768+7 {
		return errors.New("VM backup installer media is not a supported ISO image")
	}
	var descriptor [7]byte
	if _, err := file.ReadAt(descriptor[:], 32_768); err != nil || descriptor[0] != 1 || string(descriptor[1:6]) != "CD001" || descriptor[6] != 1 {
		return errors.New("VM backup installer media is not a valid ISO-9660 image")
	}
	return nil
}

// RestoreBackupGuest installs all verified disks, installer media, and the
// definition without starting the VM. Existing files are never replaced.
func (s Service) RestoreBackupGuest(ctx context.Context, name, definitionSource string, diskSources, mediaSources map[string]string) (Domain, error) {
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
	mediaDir := filepath.Clean(s.mediaDir())
	if !filepath.IsAbs(vmDir) || !filepath.IsAbs(mediaDir) {
		return Domain{}, errors.New("managed VM and media storage directories must be absolute")
	}
	if err := os.MkdirAll(vmDir, 0750); err != nil {
		return Domain{}, fmt.Errorf("create managed VM storage directory: %w", err)
	}
	definitionData, err := os.ReadFile(definitionSource)
	if err != nil {
		return Domain{}, fmt.Errorf("read verified VM backup definition: %w", err)
	}
	definition, err := parseManagedVMDefinition(name, vmDir, mediaDir, definitionData)
	if err != nil {
		return Domain{}, err
	}
	diskDefinitions := make(map[string]string)
	mediaDefinitions := make(map[string]string)
	for _, disk := range definition.Devices.Disks {
		if disk.Device == "disk" {
			diskDefinitions[filepath.Base(disk.Source.File)] = disk.Source.File
		} else if disk.Device == "cdrom" && disk.Source.File != "" {
			mediaDefinitions[filepath.Base(disk.Source.File)] = disk.Source.File
		}
	}
	if len(diskSources) != len(diskDefinitions) || len(mediaSources) != len(mediaDefinitions) {
		return Domain{}, errors.New("VM backup is missing a disk or installer media file required by its definition")
	}
	freeBytes, err := availableStorageBytes(vmDir)
	if err != nil {
		return Domain{}, fmt.Errorf("check VM restore capacity: %w", err)
	}
	var allocatedTotal uint64
	diskTargets := make([]struct{ source, target string }, 0, len(diskDefinitions))
	for filename, target := range diskDefinitions {
		source := diskSources[filename]
		info, err := os.Lstat(source)
		if err != nil || !info.Mode().IsRegular() {
			return Domain{}, fmt.Errorf("verified VM disk %q is unavailable or unsafe", filename)
		}
		allocated, err := allocatedFileBytes(info)
		if err != nil || allocated > ^uint64(0)-allocatedTotal {
			return Domain{}, errors.New("VM backup disk allocation is invalid")
		}
		allocatedTotal += allocated
		if err := s.CheckBackupDisk(ctx, source); err != nil {
			return Domain{}, fmt.Errorf("VM disk %q verification failed: %w", filename, err)
		}
		diskTargets = append(diskTargets, struct{ source, target string }{source, target})
	}
	if allocatedTotal > freeBytes {
		return Domain{}, errors.New("VM backup disks exceed currently free VM storage")
	}
	mediaTargets := make([]struct{ source, target string }, 0, len(mediaDefinitions))
	if len(mediaDefinitions) > 0 {
		if err := os.MkdirAll(mediaDir, 0750); err != nil {
			return Domain{}, fmt.Errorf("create installer media directory: %w", err)
		}
	}
	mediaFreeBytes, err := availableStorageBytes(mediaDir)
	if err != nil {
		return Domain{}, fmt.Errorf("check installer media restore capacity: %w", err)
	}
	var mediaRequired uint64
	for filename, target := range mediaDefinitions {
		source := mediaSources[filename]
		if err := s.ValidateBackupMedia(filename, source); err != nil {
			return Domain{}, fmt.Errorf("installer media %q verification failed: %w", filename, err)
		}
		if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
			info, statErr := os.Stat(source)
			if statErr != nil || info.Size() < 0 || uint64(info.Size()) > ^uint64(0)-mediaRequired {
				return Domain{}, errors.New("VM installer media allocation is invalid")
			}
			mediaRequired += uint64(info.Size())
		} else if err != nil {
			return Domain{}, fmt.Errorf("check existing installer media: %w", err)
		}
		mediaTargets = append(mediaTargets, struct{ source, target string }{source, target})
	}
	if mediaRequired > mediaFreeBytes {
		return Domain{}, errors.New("VM installer media exceeds currently free media storage")
	}
	definitionTarget := filepath.Join(vmDir, name+".xml")
	for _, target := range []string{definitionTarget} {
		if _, err := os.Lstat(target); err == nil {
			return Domain{}, errors.New("managed VM recovery files already exist; choose a different name or remove them first")
		} else if !errors.Is(err, os.ErrNotExist) {
			return Domain{}, fmt.Errorf("check VM restore path: %w", err)
		}
	}
	for _, disk := range diskTargets {
		if _, err := os.Lstat(disk.target); err == nil {
			return Domain{}, errors.New("managed VM recovery files already exist; choose a different name or remove them first")
		} else if !errors.Is(err, os.ErrNotExist) {
			return Domain{}, fmt.Errorf("check VM disk restore path: %w", err)
		}
	}
	installedDisks := make([]string, 0, len(diskTargets))
	installedMedia := make([]string, 0, len(mediaTargets))
	cleanup := true
	defer func() {
		if cleanup {
			for _, path := range installedDisks {
				_ = os.Remove(path)
			}
			for _, path := range installedMedia {
				_ = os.Remove(path)
			}
			_ = os.Remove(definitionTarget)
		}
	}()
	for _, media := range mediaTargets {
		if err := os.MkdirAll(filepath.Dir(media.target), 0750); err != nil {
			return Domain{}, fmt.Errorf("create installer media directory: %w", err)
		}
		if _, err := os.Lstat(media.target); err == nil {
			existingChecksum, _, existingErr := checksumFile(media.target)
			backupChecksum, _, backupErr := checksumFile(media.source)
			if existingErr != nil || backupErr != nil || existingChecksum != backupChecksum {
				return Domain{}, fmt.Errorf("installer media %q already exists with different contents", filepath.Base(media.target))
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return Domain{}, fmt.Errorf("check installer media restore path: %w", err)
		}
		if err := copyRegularFile(media.source, media.target, 0640); err != nil {
			return Domain{}, fmt.Errorf("install installer media %q: %w", filepath.Base(media.target), err)
		}
		installedMedia = append(installedMedia, media.target)
	}
	for _, disk := range diskTargets {
		if err := s.publishBackupDiskNoReplace(ctx, disk.source, disk.target); err != nil {
			return Domain{}, fmt.Errorf("install VM backup disk %q: %w", filepath.Base(disk.target), err)
		}
		installedDisks = append(installedDisks, disk.target)
	}
	if err := writeFileNoReplace(definitionTarget, definitionData, 0600); err != nil {
		return Domain{}, fmt.Errorf("install VM backup definition: %w", err)
	}
	if _, err := s.command(ctx, "define", "--validate", definitionTarget); err != nil {
		return Domain{}, fmt.Errorf("register restored VM definition: %w", err)
	}
	cleanup = false
	output, err := s.command(ctx, "dominfo", name)
	if err != nil {
		return Domain{Name: name, State: "shut off"}, nil
	}
	return parseDomain(name, string(output)), nil
}

func (s Service) publishBackupDiskNoReplace(ctx context.Context, source, target string) error {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".vm-restore-disk-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	_ = os.Remove(temporaryPath)
	if _, err := s.run(ctx, s.qemuImg(), "convert", "-p", "-f", "qcow2", "-O", "qcow2", source, temporaryPath); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	defer os.Remove(temporaryPath)
	if err := os.Chmod(temporaryPath, 0660); err != nil {
		return err
	}
	if _, err := s.run(ctx, s.qemuImg(), "check", "-f", "qcow2", temporaryPath); err != nil {
		return err
	}
	return os.Link(temporaryPath, target)
}

func publishFileNoReplace(source, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary, err := os.CreateTemp(filepath.Dir(target), ".vm-restore-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := io.Copy(temporary, input); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Link(temporaryPath, target)
}

func writeFileNoReplace(target string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

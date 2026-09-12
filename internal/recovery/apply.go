package recovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ApplyOptions identifies the offline filesystem root that receives a
// verified recovery bundle. Root must be absolute so a recovery invocation
// cannot accidentally write into the caller's working directory.
type ApplyOptions struct {
	Root            string
	AppdataMaxBytes int64
}

type ApplyResult struct {
	Manifest         Manifest `json:"manifest"`
	Verified         bool     `json:"verified"`
	AppliedFiles     []string `json:"appliedFiles"`
	SecretsRestored  bool     `json:"secretsRestored"`
	DatabaseRestored bool     `json:"databaseRestored"`
	AppdataRestored  []string `json:"appdataRestored,omitempty"`
}

// Apply verifies the complete bundle before writing anything. It is intended
// for an offline recovery environment; the running appliance API deliberately
// exposes staging and planning separately.
func Apply(bundle, key []byte, options ApplyOptions) (ApplyResult, error) {
	root := strings.TrimSpace(options.Root)
	if root == "" || !filepath.IsAbs(root) {
		return ApplyResult{}, errors.New("offline recovery root must be an absolute path")
	}
	plan, err := Plan(bundle, key)
	if err != nil {
		return ApplyResult{}, err
	}
	if !plan.DatabaseValid || !plan.DesiredStateValid || !plan.ComposeValid {
		return ApplyResult{}, errors.New("recovery payload validation failed")
	}
	files, err := readBundleFiles(bundle)
	if err != nil {
		return ApplyResult{}, err
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o750); err != nil {
		return ApplyResult{}, fmt.Errorf("create recovery root: %w", err)
	}

	result := ApplyResult{Manifest: plan.Manifest, Verified: true, AppliedFiles: make([]string, 0), AppdataRestored: make([]string, 0)}
	appdataLimit := options.AppdataMaxBytes
	if appdataLimit <= 0 {
		appdataLimit = DefaultAppdataArchiveLimit
	}
	for _, record := range plan.Appdata {
		archive, ok := files[record.ArchivePath]
		if !ok {
			return ApplyResult{}, fmt.Errorf("appdata archive is missing: %s", record.ArchivePath)
		}
		target := filepath.Join(root, strings.TrimPrefix(filepath.Clean(record.HostPath), string(filepath.Separator)))
		if !withinRoot(root, target) || !validAppdataHostPath(filepath.Clean(record.HostPath)) {
			return ApplyResult{}, fmt.Errorf("unsafe appdata restore target: %s", record.HostPath)
		}
		if err := rejectSymlinkPath(root, filepath.Dir(target)); err != nil {
			return ApplyResult{}, err
		}
		if err := ExtractAppdata(archive, target, appdataLimit); err != nil {
			return ApplyResult{}, fmt.Errorf("restore appdata %s: %w", record.Stack, err)
		}
		result.AppdataRestored = append(result.AppdataRestored, record.Stack+":"+record.ContainerPath)
		result.AppliedFiles = append(result.AppliedFiles, record.ArchivePath)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "manifest.json" || name == "checksums.sha256" || name == "encrypted-secrets.bin" {
			continue
		}
		target, mode, ok, err := recoveryTarget(root, name)
		if err != nil {
			return ApplyResult{}, err
		}
		if !ok {
			continue
		}
		if err := atomicRecoveryWrite(root, target, files[name], mode); err != nil {
			return ApplyResult{}, fmt.Errorf("restore %s: %w", name, err)
		}
		result.AppliedFiles = append(result.AppliedFiles, name)
		if name == "lumonas.db" {
			result.DatabaseRestored = true
		}
	}
	if encrypted, ok := files["encrypted-secrets.bin"]; ok {
		secrets, err := decrypt(encrypted, key)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("decrypt recovery secrets: %w", err)
		}
		target := filepath.Join(root, "var", "lib", "lumonas", "secrets", "recovered-secrets.bin")
		if err := atomicRecoveryWrite(root, target, secrets, 0o600); err != nil {
			return ApplyResult{}, fmt.Errorf("restore encrypted secrets: %w", err)
		}
		result.AppliedFiles = append(result.AppliedFiles, "encrypted-secrets.bin")
		result.SecretsRestored = true
	}
	return result, nil
}

func recoveryTarget(root, name string) (string, os.FileMode, bool, error) {
	var relative string
	switch {
	case name == "desired-state.json":
		relative = filepath.Join("var", "lib", "lumonas", "recovery", "restored", "desired-state.json")
	case name == "lumonas.db":
		relative = filepath.Join("var", "lib", "lumonas", "lumonas.db")
	case strings.HasPrefix(name, "docker/stacks/"):
		relative = filepath.Join("srv", "lumonas", "docker", "stacks", filepath.FromSlash(strings.TrimPrefix(name, "docker/stacks/")))
	case name == "config/shares.json":
		relative = filepath.Join("var", "lib", "lumonas", "shares.json")
	case strings.HasPrefix(name, "config/"):
		relative = filepath.Join("etc", "lumonas", "recovery", filepath.FromSlash(strings.TrimPrefix(name, "config/")))
	case name == "storage/snapraid.conf":
		relative = filepath.Join("etc", "lumonas", "snapraid.conf")
	case strings.HasPrefix(name, "storage/"):
		relative = filepath.Join("etc", "lumonas", "recovery", filepath.FromSlash(strings.TrimPrefix(name, "storage/")))
	case strings.HasPrefix(name, "acl/"):
		relative = filepath.Join("etc", "lumonas", "acl", filepath.FromSlash(strings.TrimPrefix(name, "acl/")))
	case strings.HasPrefix(name, "certificates/"):
		relative = filepath.Join("etc", "lumonas", "certificates", filepath.FromSlash(strings.TrimPrefix(name, "certificates/")))
	case strings.HasPrefix(name, "encrypted-secrets/"):
		relative = filepath.Join("var", "lib", "lumonas", "secrets", filepath.FromSlash(strings.TrimPrefix(name, "encrypted-secrets/")))
	default:
		return "", 0, false, nil
	}
	mode := os.FileMode(0o640)
	if name == "lumonas.db" || strings.HasPrefix(name, "encrypted-secrets/") || strings.HasPrefix(name, "certificates/") {
		mode = 0o600
	}
	target := filepath.Join(root, relative)
	if !withinRoot(root, target) {
		return "", 0, false, fmt.Errorf("unsafe restore target for %q", name)
	}
	return target, mode, true, nil
}

func withinRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func atomicRecoveryWrite(root, target string, data []byte, mode os.FileMode) error {
	if !withinRoot(root, target) {
		return errors.New("restore target escapes recovery root")
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	if err := rejectSymlinkPath(root, parent); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".lumonas-restore-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = temporary.Close(); _ = os.Remove(temporaryPath) }
	if err := temporary.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}

func rejectSymlinkPath(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("restore path escapes recovery root")
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("restore path contains symlink: %s", current)
		}
	}
	return nil
}

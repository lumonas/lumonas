package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/lumonas/lumonas/internal/storage"
)

var lumonasMountUnitPattern = regexp.MustCompile(`^srv-(disks|pools)-[A-Za-z0-9._-]+\.mount$`)

// applyMountPersistence renders the declarative systemd mount units for the
// requested mount state, prunes stale LumoNAS units, and reloads systemd.
// The requested state is the complete desired set, making the operation
// idempotent and self-healing.
func applyMountPersistence(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	raw, _ := req.RequestedState["entries"].([]any)
	entries := make([]storage.MountEntry, 0, len(raw))
	for _, item := range raw {
		encoded, err := json.Marshal(item)
		if err != nil {
			return response{Error: "mount entry is invalid"}
		}
		var entry storage.MountEntry
		if err := json.Unmarshal(encoded, &entry); err != nil {
			return response{Error: "mount entry is invalid"}
		}
		entries = append(entries, entry)
	}
	units, err := storage.RenderMountUnits(entries)
	if err != nil {
		return response{Error: err.Error()}
	}
	directory := envOr("LUMONAS_UNIT_DIR", "/etc/systemd/system")
	names := make([]string, 0, len(units))
	desired := make(map[string]bool, len(units))
	for name := range units {
		names = append(names, name)
		desired[name] = true
		if err := writeUnitAtomic(filepath.Join(directory, name), []byte(units[name])); err != nil {
			return response{Error: "mount unit " + name + " could not be written"}
		}
	}
	sort.Strings(names)
	removed, err := pruneLumonasMountUnits(directory, desired, run)
	if err != nil {
		return response{Error: "stale mount units could not be pruned"}
	}
	if _, err := run("systemctl", "daemon-reload"); err != nil {
		return response{Error: "systemd reload failed"}
	}
	for _, name := range names {
		if _, err := run("systemctl", "enable", name); err != nil {
			return response{Error: "mount unit " + name + " could not be enabled"}
		}
	}
	return response{OK: true, Data: map[string]any{"units": names, "removed": removed}}
}

// writeUnitAtomic replaces a unit file only when its content changed so
// repeated reconciliations do not touch disk unnecessarily.
func writeUnitAtomic(path string, content []byte) error {
	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".lumonas-unit-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func pruneLumonasMountUnits(directory string, desired map[string]bool, run command) ([]string, error) {
	items, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	removed := make([]string, 0)
	for _, item := range items {
		name := item.Name()
		if item.IsDir() || !lumonasMountUnitPattern.MatchString(name) || desired[name] {
			continue
		}
		_, _ = run("systemctl", "disable", name)
		if err := os.Remove(filepath.Join(directory, name)); err != nil {
			return nil, err
		}
		removed = append(removed, name)
	}
	sort.Strings(removed)
	return removed, nil
}

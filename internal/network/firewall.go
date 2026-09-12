package network

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteNftables atomically activates a generated ruleset. The content is
// intentionally accepted only from RenderNftables callers in the daemon.
func WriteNftables(path, content string) error {
	if !strings.HasPrefix(content, "table inet mynas {") || !strings.Contains(content, "chain input") {
		return fmt.Errorf("nftables content is not a generated MyNAS ruleset")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".nftables-validated-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

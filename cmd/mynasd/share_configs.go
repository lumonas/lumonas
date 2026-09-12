package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/shares"
)

type preparedShareConfig struct {
	temporary string
	path      string
	previous  []byte
	mode      os.FileMode
	existed   bool
}

func (s *apiServer) prepareShareConfigs(values []shares.ManagedShare) ([]preparedShareConfig, error) {
	protocols := map[string]bool{}
	for _, value := range values {
		for _, protocol := range value.Protocols {
			protocols[protocol.Name] = true
		}
	}
	configs := make([]struct {
		path, content string
		validate      func(string) error
	}, 0, 5)
	if protocols["smb"] {
		legacy := make([]shares.Share, 0, len(values))
		for _, value := range values {
			legacy = append(legacy, value.Legacy())
		}
		content, err := shares.RenderSamba(legacy)
		if err != nil {
			return nil, err
		}
		configs = append(configs, struct {
			path, content string
			validate      func(string) error
		}{envOr("MYNAS_SAMBA_CONFIG", "/var/lib/mynas/generated/smb.conf"), content, shares.ValidateSamba})
	}
	if protocols["nfs"] {
		content, err := shares.RenderNFS(values)
		if err != nil {
			return nil, err
		}
		configs = append(configs, struct {
			path, content string
			validate      func(string) error
		}{envOr("MYNAS_NFS_EXPORTS", "/var/lib/mynas/generated/exports"), content, validateGeneratedConfig})
	}
	if protocols["sftp"] {
		content, err := shares.RenderSFTP(values)
		if err != nil {
			return nil, err
		}
		configs = append(configs, struct {
			path, content string
			validate      func(string) error
		}{envOr("MYNAS_SFTP_CONFIG", "/var/lib/mynas/generated/sshd-sftp.conf"), content, validateGeneratedConfig})
	}
	if protocols["ftp"] || protocols["ftps"] {
		content, err := shares.RenderFTP(values)
		if err != nil {
			return nil, err
		}
		configs = append(configs, struct {
			path, content string
			validate      func(string) error
		}{envOr("MYNAS_FTP_CONFIG", "/var/lib/mynas/generated/ftp.conf"), content, validateGeneratedConfig})
	}
	if protocols["rsync"] {
		content, err := shares.RenderRsync(values)
		if err != nil {
			return nil, err
		}
		configs = append(configs, struct {
			path, content string
			validate      func(string) error
		}{envOr("MYNAS_RSYNC_CONFIG", "/var/lib/mynas/generated/rsync.conf"), content, validateGeneratedConfig})
	}
	prepared := make([]preparedShareConfig, 0, len(configs))
	for _, config := range configs {
		if err := os.MkdirAll(filepath.Dir(config.path), 0o750); err != nil {
			cleanupShareConfigs(prepared)
			return nil, err
		}
		temporary, err := os.CreateTemp(filepath.Dir(config.path), ".share-validated-*")
		if err != nil {
			cleanupShareConfigs(prepared)
			return nil, err
		}
		temporaryPath := temporary.Name()
		if _, err := temporary.WriteString(config.content); err != nil {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
			cleanupShareConfigs(prepared)
			return nil, err
		}
		if err := temporary.Chmod(0o640); err != nil {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
			cleanupShareConfigs(prepared)
			return nil, err
		}
		if err := temporary.Close(); err != nil {
			_ = os.Remove(temporaryPath)
			cleanupShareConfigs(prepared)
			return nil, err
		}
		if err := config.validate(temporaryPath); err != nil {
			_ = os.Remove(temporaryPath)
			cleanupShareConfigs(prepared)
			return nil, fmt.Errorf("%s configuration validation failed: %w", filepath.Base(config.path), err)
		}
		item := preparedShareConfig{temporary: temporaryPath, path: config.path, mode: 0o640}
		if previous, err := os.ReadFile(config.path); err == nil {
			item.previous, item.existed = previous, true
			if info, statErr := os.Stat(config.path); statErr == nil {
				item.mode = info.Mode().Perm()
			}
		} else if !os.IsNotExist(err) {
			_ = os.Remove(temporaryPath)
			cleanupShareConfigs(prepared)
			return nil, err
		}
		prepared = append(prepared, item)
	}
	return prepared, nil
}

func activateShareConfigs(values []preparedShareConfig) error {
	for index := range values {
		if err := os.Rename(values[index].temporary, values[index].path); err != nil {
			for rollback := index - 1; rollback >= 0; rollback-- {
				restoreShareConfig(values[rollback])
			}
			cleanupShareConfigs(values[index:])
			return err
		}
	}
	return nil
}

func restoreShareConfig(value preparedShareConfig) {
	if value.existed {
		_ = os.WriteFile(value.path, value.previous, value.mode)
		return
	}
	_ = os.Remove(value.path)
}

func restoreShareConfigs(values []preparedShareConfig) {
	for _, value := range values {
		restoreShareConfig(value)
	}
}

func cleanupShareConfigs(values []preparedShareConfig) {
	for _, value := range values {
		_ = os.Remove(value.temporary)
	}
}

func validateGeneratedConfig(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > 1<<20 {
		return fmt.Errorf("generated configuration is too large")
	}
	return nil
}

func (s *apiServer) reloadShareServices(ctx context.Context, values []shares.ManagedShare) error {
	services := map[string]bool{}
	for _, value := range values {
		for _, protocol := range value.Protocols {
			switch protocol.Name {
			case "smb":
				services["smbd.service"] = true
			case "nfs":
				services["nfs-server.service"] = true
			case "sftp":
				services["ssh.service"] = true
			}
		}
	}
	if len(services) == 0 {
		return nil
	}
	socket := envOr("MYNAS_PRIVD_SOCKET", "/run/mynas/privd.sock")
	if _, err := os.Stat(socket); os.IsNotExist(err) {
		// Development environments may not run the privileged broker. The
		// validated files remain canonical and will be picked up on startup.
		return nil
	}
	for service := range services {
		result, err := (privileged.Client{Socket: socket}).Execute(ctx, privileged.Request{
			Operation: "service.reload", OperationID: newID("reload"), PlanHash: newID("reload-plan"),
			RequestedState: map[string]any{"service": service}, Confirmed: true,
		})
		if err != nil {
			return err
		}
		if !result.OK {
			return fmt.Errorf("reload %s: %s", service, result.Error)
		}
	}
	return nil
}

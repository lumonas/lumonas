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
	remove    bool
}

func (s *apiServer) prepareShareConfigs(values []shares.ManagedShare) ([]preparedShareConfig, error) {
	protocols := map[string]bool{}
	for _, value := range values {
		for _, protocol := range value.Protocols {
			protocols[protocol.Name] = true
			if protocol.Name == "timemachine" {
				protocols["smb"] = true
			}
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
		}{envOr("LUMONAS_SAMBA_CONFIG", "/var/lib/lumonas/generated/smb.conf"), content, shares.ValidateSamba})
	}
	if protocols["nfs"] {
		content, err := shares.RenderNFS(values)
		if err != nil {
			return nil, err
		}
		configs = append(configs, struct {
			path, content string
			validate      func(string) error
		}{envOr("LUMONAS_NFS_EXPORTS", "/var/lib/lumonas/generated/exports"), content, validateGeneratedConfig})
	}
	if protocols["sftp"] {
		content, err := shares.RenderSFTP(values)
		if err != nil {
			return nil, err
		}
		configs = append(configs, struct {
			path, content string
			validate      func(string) error
		}{envOr("LUMONAS_SFTP_CONFIG", "/var/lib/lumonas/generated/sshd-sftp.conf"), content, validateGeneratedConfig})
	}
	staleFTPUsers := make([]preparedShareConfig, 0)
	if protocols["ftp"] || protocols["ftps"] {
		ftp, err := shares.RenderFTPConfig(values, ftpOptions())
		if err != nil {
			return nil, err
		}
		userDirectory := envOr("LUMONAS_FTP_USER_DIR", "/var/lib/lumonas/generated/vsftpd-users")
		configs = append(configs, struct {
			path, content string
			validate      func(string) error
		}{envOr("LUMONAS_FTP_CONFIG", "/var/lib/lumonas/generated/ftp.conf"), ftp.Main, validateGeneratedConfig})
		for username, content := range ftp.Users {
			if !validFTPUserName(username) {
				return nil, fmt.Errorf("FTP principal name %q is invalid", username)
			}
			configs = append(configs, struct {
				path, content string
				validate      func(string) error
			}{filepath.Join(userDirectory, username), content, validateGeneratedConfig})
		}
		if entries, readErr := os.ReadDir(userDirectory); readErr == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if _, mapped := ftp.Users[entry.Name()]; mapped {
					continue
				}
				stalePath := filepath.Join(userDirectory, entry.Name())
				previous, previousErr := os.ReadFile(stalePath)
				if previousErr != nil {
					continue
				}
				staleFTPUsers = append(staleFTPUsers, preparedShareConfig{path: stalePath, previous: previous, mode: 0o640, existed: true, remove: true})
			}
		}
	}
	if protocols["rsync"] {
		content, err := shares.RenderRsync(values)
		if err != nil {
			return nil, err
		}
		configs = append(configs, struct {
			path, content string
			validate      func(string) error
		}{envOr("LUMONAS_RSYNC_CONFIG", "/var/lib/lumonas/generated/rsync.conf"), content, validateGeneratedConfig})
	}
	prepared := make([]preparedShareConfig, 0, len(configs)+len(staleFTPUsers))
	prepared = append(prepared, staleFTPUsers...)
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
	for protocol, path := range managedConfigPaths() {
		if protocols[protocol] || (protocol == "ftp" || protocol == "ftps") && (protocols["ftp"] || protocols["ftps"]) {
			continue
		}
		previous, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			cleanupShareConfigs(prepared)
			return nil, err
		}
		item := preparedShareConfig{path: path, previous: previous, mode: 0o640, existed: true, remove: true}
		if info, statErr := os.Stat(path); statErr == nil {
			item.mode = info.Mode().Perm()
		}
		prepared = append(prepared, item)
	}
	if !protocols["ftp"] && !protocols["ftps"] {
		userDirectory := envOr("LUMONAS_FTP_USER_DIR", "/var/lib/lumonas/generated/vsftpd-users")
		if entries, readErr := os.ReadDir(userDirectory); readErr == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				stalePath := filepath.Join(userDirectory, entry.Name())
				previous, previousErr := os.ReadFile(stalePath)
				if previousErr != nil {
					continue
				}
				prepared = append(prepared, preparedShareConfig{path: stalePath, previous: previous, mode: 0o640, existed: true, remove: true})
			}
		}
	}
	return prepared, nil
}

func ftpOptions() shares.FTPOptions {
	return shares.FTPOptions{
		UserConfigDir: envOr("LUMONAS_FTP_USER_DIR", "/var/lib/lumonas/generated/vsftpd-users"),
		TLSCertFile:   envOr("LUMONAS_WEB_TLS_CERT", "/etc/lumonas/tls/tls.crt"),
		TLSKeyFile:    envOr("LUMONAS_WEB_TLS_KEY", "/etc/lumonas/tls/tls.key"),
	}
}

func validFTPUserName(value string) bool {
	for _, char := range value {
		valid := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-'
		if !valid {
			return false
		}
	}
	return value != ""
}

func activateShareConfigs(values []preparedShareConfig) error {
	for index := range values {
		var err error
		if values[index].remove {
			err = os.Remove(values[index].path)
			if os.IsNotExist(err) {
				err = nil
			}
		} else {
			err = os.Rename(values[index].temporary, values[index].path)
		}
		if err != nil {
			for rollback := index - 1; rollback >= 0; rollback-- {
				restoreShareConfig(values[rollback])
			}
			cleanupShareConfigs(values[index:])
			return err
		}
	}
	return nil
}

func managedConfigPaths() map[string]string {
	return map[string]string{
		"smb":   envOr("LUMONAS_SAMBA_CONFIG", "/var/lib/lumonas/generated/smb.conf"),
		"nfs":   envOr("LUMONAS_NFS_EXPORTS", "/var/lib/lumonas/generated/exports"),
		"sftp":  envOr("LUMONAS_SFTP_CONFIG", "/var/lib/lumonas/generated/sshd-sftp.conf"),
		"ftp":   envOr("LUMONAS_FTP_CONFIG", "/var/lib/lumonas/generated/ftp.conf"),
		"ftps":  envOr("LUMONAS_FTP_CONFIG", "/var/lib/lumonas/generated/ftp.conf"),
		"rsync": envOr("LUMONAS_RSYNC_CONFIG", "/var/lib/lumonas/generated/rsync.conf"),
	}
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
			case "smb", "timemachine":
				services["smbd.service"] = true
			case "nfs":
				services["nfs-server.service"] = true
			case "sftp":
				services["ssh.service"] = true
			case "ftp", "ftps":
				services["vsftpd.service"] = true
			}
		}
	}
	if len(services) == 0 {
		return nil
	}
	socket := envOr("LUMONAS_PRIVD_SOCKET", "/run/lumonas/privd.sock")
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

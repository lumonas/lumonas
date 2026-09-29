package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/runner"
)

const maxRcloneConfigBytes = 64 << 10

var rcloneRemoteName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
var rcloneAllowedTypes = map[string]bool{"drive": true, "dropbox": true, "onedrive": true, "box": true, "b2": true, "azureblob": true, "webdav": true}
var runRcloneCommand = runner.OutputContext

func parseRcloneTarget(target string) (string, string, error) {
	index := strings.IndexByte(target, ':')
	if index < 1 || index+1 >= len(target) {
		return "", "", errors.New("cloud target must use remote-name:path syntax")
	}
	remote, prefix := target[:index], target[index+1:]
	if !rcloneRemoteName.MatchString(remote) {
		return "", "", errors.New("cloud remote name contains unsupported characters")
	}
	if strings.Contains(prefix, "\\") || path.IsAbs(prefix) || path.Clean(prefix) != prefix || prefix == "." || strings.HasPrefix(prefix, "../") || prefix == ".." {
		return "", "", errors.New("cloud destination prefix must be a relative path without traversal")
	}
	return remote, prefix, nil
}

func validateRcloneConfig(config, remote string) error {
	if len(config) == 0 || len(config) > maxRcloneConfigBytes {
		return errors.New("rclone configuration must be between 1 byte and 64 KiB")
	}
	section := ""
	types := map[string]string{}
	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if !rcloneRemoteName.MatchString(section) {
				return errors.New("rclone configuration has an invalid remote section name")
			}
			continue
		}
		if section == "" {
			return errors.New("rclone configuration contains a setting outside a remote section")
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return errors.New("rclone configuration contains a malformed setting")
		}
		if strings.TrimSpace(key) == "type" {
			types[section] = strings.TrimSpace(value)
		}
	}
	remoteType := types[remote]
	if remoteType == "" {
		return fmt.Errorf("rclone configuration is missing remote [%s]", remote)
	}
	if !rcloneAllowedTypes[remoteType] {
		return fmt.Errorf("rclone provider %q is not supported", remoteType)
	}
	return nil
}

func ValidateRcloneConfiguration(target, config string) error {
	remote, _, err := parseRcloneTarget(target)
	if err != nil {
		return err
	}
	return validateRcloneConfig(config, remote)
}

func withRcloneConfig(ctx context.Context, target string, credentials Credentials, callback func(string, string) error) error {
	remote, _, err := parseRcloneTarget(target)
	if err != nil {
		return err
	}
	if err := validateRcloneConfig(credentials.RcloneConfig, remote); err != nil {
		return err
	}
	file, err := os.CreateTemp("", ".lumonas-rclone-*.conf")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.WriteString(credentials.RcloneConfig); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return callback(name, remote)
}

func rcloneRemotePath(target, object string) (string, error) {
	remote, prefix, err := parseRcloneTarget(target)
	if err != nil {
		return "", err
	}
	if err := validateRemoteObject(object); err != nil {
		return "", err
	}
	return remote + ":" + path.Join(prefix, object), nil
}

func rcloneCopy(ctx context.Context, destination Destination, credentials Credentials, object, source string) error {
	target, err := rcloneRemotePath(destination.Target, object)
	if err != nil {
		return err
	}
	return withRcloneConfig(ctx, destination.Target, credentials, func(config, _ string) error {
		_, err := runRcloneCommand(ctx, "rclone", "--config", config, "--log-level", "ERROR", "copyto", source, target)
		return err
	})
}

func rcloneDelete(ctx context.Context, destination Destination, credentials Credentials, object string) error {
	target, err := rcloneRemotePath(destination.Target, object)
	if err != nil {
		return err
	}
	return withRcloneConfig(ctx, destination.Target, credentials, func(config, _ string) error {
		_, err := runRcloneCommand(ctx, "rclone", "--config", config, "--log-level", "ERROR", "deletefile", target)
		return err
	})
}

func rcloneDownload(ctx context.Context, destination Destination, credentials Credentials, object, target string) error {
	remotePath, err := rcloneRemotePath(destination.Target, object)
	if err != nil {
		return err
	}
	return withRcloneConfig(ctx, destination.Target, credentials, func(config, _ string) error {
		_, err := runRcloneCommand(ctx, "rclone", "--config", config, "--log-level", "ERROR", "copyto", remotePath, target)
		return err
	})
}

func listRclone(ctx context.Context, destination Destination, credentials Credentials, prefix string) ([]RemoteFile, error) {
	if prefix != "" {
		if err := validateRemoteObject(prefix); err != nil {
			return nil, err
		}
	}
	remotePath := destination.Target
	if prefix != "" {
		var err error
		remotePath, err = rcloneRemotePath(destination.Target, prefix)
		if err != nil {
			return nil, err
		}
	}
	var output []byte
	err := withRcloneConfig(ctx, destination.Target, credentials, func(config, _ string) error {
		var runErr error
		output, runErr = runRcloneCommand(ctx, "rclone", "--config", config, "--log-level", "ERROR", "lsjson", "--recursive", "--files-only", remotePath)
		return runErr
	})
	if err != nil {
		return nil, err
	}
	var entries []struct {
		Path    string    `json:"Path"`
		Size    int64     `json:"Size"`
		ModTime time.Time `json:"ModTime"`
		IsDir   bool      `json:"IsDir"`
	}
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("parse rclone listing: %w", err)
	}
	files := make([]RemoteFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		if err := validateRemoteObject(entry.Path); err != nil {
			return nil, err
		}
		files = append(files, RemoteFile{Path: entry.Path, Size: entry.Size, ModTime: entry.ModTime})
	}
	return files, nil
}

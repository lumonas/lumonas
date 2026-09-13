package shares

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxGeneratedConfigBytes = 1 << 20

func generatedConfigLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > maxGeneratedConfigBytes {
		return nil, fmt.Errorf("generated configuration is too large")
	}
	if strings.ContainsRune(string(data), '\x00') {
		return nil, fmt.Errorf("generated configuration contains a NUL byte")
	}
	return strings.Split(string(data), "\n"), nil
}

func generatedAbsolutePath(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value && !strings.Contains(value, "..") && !strings.ContainsAny(value, "\x00\r\n")
}

// ValidateNFSExports validates the generated exports grammar without
// changing the host export table.
func ValidateNFSExports(path string) error {
	lines, err := generatedConfigLines(path)
	if err != nil {
		return err
	}
	for lineNumber, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !generatedAbsolutePath(fields[0]) {
			return fmt.Errorf("invalid NFS export on line %d", lineNumber+1)
		}
		for _, client := range fields[1:] {
			if strings.ContainsAny(client, "\x00\r\n") || strings.Contains(client, "(") != strings.Contains(client, ")") {
				return fmt.Errorf("invalid NFS client on line %d", lineNumber+1)
			}
		}
	}
	return nil
}

// ValidateSFTPConfig validates the generated sshd SFTP include without
// invoking sshd or mutating the host daemon configuration.
func ValidateSFTPConfig(path string) error {
	lines, err := generatedConfigLines(path)
	if err != nil {
		return err
	}
	subsystem := false
	for lineNumber, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case line == "Subsystem sftp internal-sftp":
			subsystem = true
		case strings.HasPrefix(line, "Match User "):
			if strings.TrimSpace(strings.TrimPrefix(line, "Match User ")) == "" {
				return fmt.Errorf("SFTP Match User is empty on line %d", lineNumber+1)
			}
		case strings.HasPrefix(line, "ChrootDirectory "):
			if !generatedAbsolutePath(strings.TrimSpace(strings.TrimPrefix(line, "ChrootDirectory "))) {
				return fmt.Errorf("invalid SFTP chroot on line %d", lineNumber+1)
			}
		case line == "ForceCommand internal-sftp" || line == "ForceCommand internal-sftp -R" || line == "X11Forwarding no" || line == "AllowTcpForwarding no":
		default:
			return fmt.Errorf("unsupported generated SFTP directive on line %d", lineNumber+1)
		}
	}
	if !subsystem {
		return fmt.Errorf("SFTP Subsystem directive is missing")
	}
	return nil
}

func parseGeneratedKeyValues(path string) (map[string]string, error) {
	lines, err := generatedConfigLines(path)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for lineNumber, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid key/value directive on line %d", lineNumber+1)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if strings.ContainsAny(key+value, "\x00\r\n") {
			return nil, fmt.Errorf("control character in directive on line %d", lineNumber+1)
		}
		values[key] = value
	}
	return values, nil
}

// ValidateFTPConfig validates the generated vsftpd global configuration.
func ValidateFTPConfig(path string) error {
	values, err := parseGeneratedKeyValues(path)
	if err != nil {
		return err
	}
	for _, key := range []string{"listen", "anonymous_enable", "local_enable", "chroot_local_user", "user_config_dir", "pasv_min_port", "pasv_max_port"} {
		if _, ok := values[key]; !ok {
			return fmt.Errorf("FTP directive %q is missing", key)
		}
	}
	if values["listen"] != "YES" || values["anonymous_enable"] != "NO" || values["local_enable"] != "YES" {
		return fmt.Errorf("FTP listener policy is unsafe")
	}
	if !generatedAbsolutePath(values["user_config_dir"]) || !generatedAbsolutePath(values["secure_chroot_dir"]) {
		return fmt.Errorf("FTP paths must be absolute and normalized")
	}
	start, startErr := strconv.Atoi(values["pasv_min_port"])
	end, endErr := strconv.Atoi(values["pasv_max_port"])
	if startErr != nil || endErr != nil || start < 1024 || end > 65535 || start > end {
		return fmt.Errorf("FTP passive port range is invalid")
	}
	if values["ssl_enable"] == "YES" {
		if !generatedAbsolutePath(values["rsa_cert_file"]) || !generatedAbsolutePath(values["rsa_private_key_file"]) {
			return fmt.Errorf("FTPS certificate paths must be absolute and normalized")
		}
	}
	return nil
}

// ValidateFTPUserConfig validates the per-principal vsftpd override.
func ValidateFTPUserConfig(path string) error {
	values, err := parseGeneratedKeyValues(path)
	if err != nil {
		return err
	}
	if !generatedAbsolutePath(values["local_root"]) || (values["write_enable"] != "YES" && values["write_enable"] != "NO") {
		return fmt.Errorf("FTP user policy is invalid")
	}
	return nil
}

// ValidateRsyncConfig validates generated rsync daemon modules.
func ValidateRsyncConfig(path string) error {
	lines, err := generatedConfigLines(path)
	if err != nil {
		return err
	}
	module := ""
	paths := 0
	for lineNumber, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			module = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if module == "" {
				return fmt.Errorf("empty rsync module on line %d", lineNumber+1)
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid rsync directive on line %d", lineNumber+1)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "uid", "gid", "use chroot":
		case "path":
			if module == "" || !generatedAbsolutePath(value) {
				return fmt.Errorf("invalid rsync module path on line %d", lineNumber+1)
			}
			paths++
		case "read only":
			if value != "true" && value != "false" {
				return fmt.Errorf("invalid rsync read-only value on line %d", lineNumber+1)
			}
		default:
			return fmt.Errorf("unsupported rsync directive %q", key)
		}
	}
	if paths == 0 {
		return fmt.Errorf("rsync configuration has no modules")
	}
	return nil
}

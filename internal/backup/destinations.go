package backup

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/runner"
)

type sftpCommandRunner func(context.Context, io.Reader, string, ...string) ([]byte, error)

var runSFTPCommand sftpCommandRunner = runner.CombinedOutputContextWithStdin

func UploadWithTimeout(destination Destination, credentials Credentials, source, object string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return Upload(ctx, destination, credentials, source, object)
}

func DeleteWithTimeout(destination Destination, credentials Credentials, object string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return Delete(ctx, destination, credentials, object)
}

func Upload(ctx context.Context, destination Destination, credentials Credentials, source, object string) error {
	if err := destination.Validate(); err != nil {
		return err
	}
	switch destination.Type {
	case DestinationLocal:
		if err := requireMountedBackupTarget(destination.Target); err != nil {
			return err
		}
		return uploadLocal(destination.Target, source, object)
	case DestinationSFTP:
		return uploadSFTP(ctx, destination.Target, credentials, source, object)
	case DestinationS3:
		return uploadS3(ctx, destination.Target, credentials, source, object, destination.Retention.ProviderObjectLock, destination.Retention.ImmutableDays)
	case DestinationRclone:
		return rcloneCopy(ctx, destination, credentials, object, source)
	default:
		return fmt.Errorf("unsupported backup destination type %q", destination.Type)
	}
}

func Delete(ctx context.Context, destination Destination, credentials Credentials, object string) error {
	if err := destination.Validate(); err != nil {
		return err
	}
	switch destination.Type {
	case DestinationLocal:
		if err := requireMountedBackupTarget(destination.Target); err != nil {
			return err
		}
		target, err := safeJoin(destination.Target, object)
		if err != nil {
			return err
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	case DestinationSFTP:
		return deleteSFTP(ctx, destination.Target, credentials, object)
	case DestinationS3:
		return deleteS3(ctx, destination.Target, credentials, object)
	case DestinationRclone:
		return rcloneDelete(ctx, destination, credentials, object)
	default:
		return fmt.Errorf("unsupported backup destination type %q", destination.Type)
	}
}

func UploadAndVerifyWithTimeout(destination Destination, credentials Credentials, source, object, expectedChecksum string, expectedBytes int64) error {
	if err := UploadWithTimeout(destination, credentials, source, object); err != nil {
		return err
	}
	verifyPath, err := os.CreateTemp("", ".lumonas-backup-verify-*")
	if err != nil {
		return err
	}
	verifyPathName := verifyPath.Name()
	if err := verifyPath.Close(); err != nil {
		_ = os.Remove(verifyPathName)
		return err
	}
	defer os.Remove(verifyPathName)
	if err := DownloadWithTimeout(destination, credentials, object, verifyPathName); err != nil {
		return fmt.Errorf("download backup copy for verification: %w", err)
	}
	digest, size, err := SHA256File(verifyPathName)
	if err != nil {
		return fmt.Errorf("hash downloaded backup copy: %w", err)
	}
	if size != expectedBytes || !strings.EqualFold(digest, expectedChecksum) {
		return fmt.Errorf("backup copy verification mismatch: expected %s/%d, got %s/%d", expectedChecksum, expectedBytes, digest, size)
	}
	return nil
}

func UploadAndVerifyWithRetry(destination Destination, credentials Credentials, source, object, expectedChecksum string, expectedBytes int64, attempts int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return UploadAndVerifyWithRetryContext(ctx, destination, credentials, source, object, expectedChecksum, expectedBytes, attempts)
}

// UploadAndVerifyWithRetryContext uploads and downloads a backup object under
// the caller's deadline. Large VM images need longer deadlines than bundles.
func UploadAndVerifyWithRetryContext(ctx context.Context, destination Destination, credentials Credentials, source, object, expectedChecksum string, expectedBytes int64, attempts int) error {
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := UploadAndVerifyContext(ctx, destination, credentials, source, object, expectedChecksum, expectedBytes); err == nil {
			return nil
		} else {
			last = err
		}
		if attempt+1 < attempts {
			timer := time.NewTimer(time.Duration(1<<attempt) * 100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return last
}

func UploadAndVerifyContext(ctx context.Context, destination Destination, credentials Credentials, source, object, expectedChecksum string, expectedBytes int64) error {
	if err := Upload(ctx, destination, credentials, source, object); err != nil {
		return err
	}
	verifyPath, err := os.CreateTemp("", ".lumonas-backup-verify-*")
	if err != nil {
		return err
	}
	verifyPathName := verifyPath.Name()
	if err := verifyPath.Close(); err != nil {
		_ = os.Remove(verifyPathName)
		return err
	}
	defer os.Remove(verifyPathName)
	if err := Download(ctx, destination, credentials, object, verifyPathName); err != nil {
		return fmt.Errorf("download backup copy for verification: %w", err)
	}
	digest, size, err := SHA256File(verifyPathName)
	if err != nil {
		return fmt.Errorf("hash downloaded backup copy: %w", err)
	}
	if size != expectedBytes || !strings.EqualFold(digest, expectedChecksum) {
		return fmt.Errorf("backup copy verification mismatch: expected %s/%d, got %s/%d", expectedChecksum, expectedBytes, digest, size)
	}
	return nil
}

func DownloadWithTimeout(destination Destination, credentials Credentials, object, target string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return Download(ctx, destination, credentials, object, target)
}

func Download(ctx context.Context, destination Destination, credentials Credentials, object, target string) error {
	if err := destination.Validate(); err != nil {
		return err
	}
	if !filepath.IsAbs(target) || filepath.Clean(target) != target {
		return errors.New("backup download target must be a clean absolute path")
	}
	if err := validateRemoteObject(object); err != nil {
		return err
	}
	switch destination.Type {
	case DestinationLocal:
		if err := requireMountedBackupTarget(destination.Target); err != nil {
			return err
		}
		source, err := safeJoin(destination.Target, object)
		if err != nil {
			return err
		}
		input, err := os.Open(source)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(output, input); err != nil {
			_ = output.Close()
			return err
		}
		if err := output.Sync(); err != nil {
			_ = output.Close()
			return err
		}
		return output.Close()
	case DestinationSFTP:
		return downloadSFTP(ctx, destination.Target, credentials, object, target)
	case DestinationS3:
		return downloadS3(ctx, destination.Target, credentials, object, target)
	case DestinationRclone:
		return rcloneDownload(ctx, destination, credentials, object, target)
	default:
		return fmt.Errorf("unsupported backup destination type %q", destination.Type)
	}
}

type RemoteFile struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// ListRemote returns regular files under an SFTP or S3 destination prefix.
func ListRemote(ctx context.Context, destination Destination, credentials Credentials, prefix string) ([]RemoteFile, error) {
	if err := destination.Validate(); err != nil {
		return nil, err
	}
	if prefix != "" {
		if err := validateRemoteObject(prefix); err != nil {
			return nil, err
		}
	}
	switch destination.Type {
	case DestinationSFTP:
		return listSFTP(ctx, destination.Target, credentials, prefix)
	case DestinationS3:
		return listS3(ctx, destination.Target, credentials, prefix)
	case DestinationRclone:
		return listRclone(ctx, destination, credentials, prefix)
	default:
		return nil, fmt.Errorf("unsupported remote listing destination %q", destination.Type)
	}
}

func uploadLocal(root, source, object string) error {
	target, err := safeJoin(root, object)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".upload-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = temporary.Close(); _ = os.Remove(temporaryPath) }
	input, err := os.Open(source)
	if err != nil {
		cleanup()
		return err
	}
	if _, err := io.Copy(temporary, input); err != nil {
		input.Close()
		cleanup()
		return err
	}
	if err := input.Close(); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Chmod(0o600); err != nil || temporary.Close() != nil {
		cleanup()
		return errors.New("close local backup copy")
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		cleanup()
		return err
	}
	return syncDirectory(filepath.Dir(target))
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

func uploadSFTP(ctx context.Context, target string, credentials Credentials, source, object string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" {
		return errors.New("invalid SFTP target")
	}
	user := parsed.User.Username()
	if user == "" {
		user = credentials.Username
	}
	if user == "" || credentials.PrivateKeyPath == "" {
		return errors.New("SFTP credentials require username and private key path")
	}
	if err := validateRemoteObject(object); err != nil {
		return err
	}
	remote := path.Join(parsed.Path, object)
	if !strings.HasPrefix(remote, "/") {
		return errors.New("SFTP target path must be absolute")
	}
	args := []string{"-oBatchMode=yes", "-i", credentials.PrivateKeyPath}
	if parsed.Port() != "" {
		args = append(args, "-P", parsed.Port())
	}
	args = append(args, "-b", "-", user+"@"+parsed.Host)
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Errorf("create temporary SFTP object name: %w", err)
	}
	temporary := remote + ".lumonas-upload-" + hex.EncodeToString(suffix[:])
	previous := remote + ".lumonas-previous-" + hex.EncodeToString(suffix[:])
	script := "-mkdir " + sftpQuote(filepath.ToSlash(path.Dir(remote))) + "\n-rename " + sftpQuote(remote) + " " + sftpQuote(previous) + "\nput " + sftpQuote(source) + " " + sftpQuote(temporary) + "\nrename " + sftpQuote(temporary) + " " + sftpQuote(remote) + "\n-rm " + sftpQuote(previous) + "\n"
	if output, err := runSFTPCommand(ctx, strings.NewReader(script), "sftp", args...); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, _ = runSFTPCommand(cleanupCtx, strings.NewReader("-rename "+sftpQuote(previous)+" "+sftpQuote(remote)+"\n-rm "+sftpQuote(temporary)+"\n"), "sftp", args...)
		cancel()
		return fmt.Errorf("SFTP upload failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func deleteSFTP(ctx context.Context, target string, credentials Credentials, object string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" || credentials.Username == "" || credentials.PrivateKeyPath == "" {
		return errors.New("SFTP delete requires a valid target and credentials")
	}
	if err := validateRemoteObject(object); err != nil {
		return err
	}
	remote := path.Join(parsed.Path, object)
	args := []string{"-oBatchMode=yes", "-i", credentials.PrivateKeyPath, "-b", "-"}
	if parsed.Port() != "" {
		args = append(args[:3], append([]string{"-P", parsed.Port()}, args[3:]...)...)
	}
	args = append(args, credentials.Username+"@"+parsed.Host)
	if output, err := runSFTPCommand(ctx, strings.NewReader("rm "+sftpQuote(remote)+"\n"), "sftp", args...); err != nil {
		return fmt.Errorf("SFTP delete failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func downloadSFTP(ctx context.Context, target string, credentials Credentials, object, destination string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" || credentials.PrivateKeyPath == "" {
		return errors.New("SFTP download requires a valid target and private key")
	}
	user := parsed.User.Username()
	if user == "" {
		user = credentials.Username
	}
	if user == "" {
		return errors.New("SFTP credentials require username")
	}
	if err := validateRemoteObject(object); err != nil {
		return err
	}
	remote := path.Join(parsed.Path, object)
	args := []string{"-oBatchMode=yes", "-i", credentials.PrivateKeyPath, "-b", "-"}
	if parsed.Port() != "" {
		args = append(args[:3], append([]string{"-P", parsed.Port()}, args[3:]...)...)
	}
	args = append(args, user+"@"+parsed.Host)
	if output, err := runSFTPCommand(ctx, strings.NewReader("get "+sftpQuote(remote)+" "+sftpQuote(destination)+"\n"), "sftp", args...); err != nil {
		return fmt.Errorf("SFTP download failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func listSFTP(ctx context.Context, target string, credentials Credentials, prefix string) ([]RemoteFile, error) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" || credentials.PrivateKeyPath == "" {
		return nil, errors.New("SFTP listing requires a valid target and private key")
	}
	user := parsed.User.Username()
	if user == "" {
		user = credentials.Username
	}
	if user == "" {
		return nil, errors.New("SFTP credentials require username")
	}
	args := []string{"-oBatchMode=yes", "-i", credentials.PrivateKeyPath}
	if parsed.Port() != "" {
		args = append(args, "-P", parsed.Port())
	}
	args = append(args, "-b", "-", user+"@"+parsed.Host)
	root := path.Join(parsed.Path, prefix)
	type directory struct{ remote, relative string }
	queue := []directory{{remote: root}}
	visited := map[string]bool{}
	result := make([]RemoteFile, 0)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current.remote] {
			continue
		}
		visited[current.remote] = true
		output, err := runSFTPCommand(ctx, strings.NewReader("ls -la "+sftpQuote(current.remote)+"\n"), "sftp", args...)
		if err != nil {
			return nil, fmt.Errorf("SFTP list failed: %s", strings.TrimSpace(string(output)))
		}
		scanner := bufio.NewScanner(strings.NewReader(string(output)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			fields := strings.Fields(line)
			if len(fields) < 9 || len(fields[0]) < 1 {
				continue
			}
			mode := fields[0]
			if mode[0] != 'd' && mode[0] != '-' {
				continue
			}
			size, parseErr := strconv.ParseInt(fields[4], 10, 64)
			if parseErr != nil {
				continue
			}
			name := strings.Join(fields[8:], " ")
			if name == "." || name == ".." || name == "" {
				continue
			}
			relative := path.Join(current.relative, name)
			if err := validateRemoteObject(relative); err != nil {
				continue
			}
			modified := parseSFTPListingTime(fields[5], fields[6], fields[7])
			if mode[0] == 'd' {
				queue = append(queue, directory{remote: path.Join(current.remote, name), relative: relative})
				continue
			}
			result = append(result, RemoteFile{Path: relative, Size: size, ModTime: modified})
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func parseSFTPListingTime(month, day, clockOrYear string) time.Time {
	now := time.Now()
	if _, err := strconv.Atoi(clockOrYear); err == nil {
		parsed, parseErr := time.ParseInLocation("Jan 02 2006", month+" "+day+" "+clockOrYear, time.Local)
		if parseErr != nil {
			return time.Time{}
		}
		return parsed.UTC()
	}
	parsed, err := time.ParseInLocation("Jan 02 2006 15:04", month+" "+day+" "+strconv.Itoa(now.Year())+" "+clockOrYear, time.Local)
	if err != nil {
		return time.Time{}
	}
	if parsed.After(now.Add(24 * time.Hour)) {
		parsed = parsed.AddDate(-1, 0, 0)
	}
	return parsed.UTC()
}

func validateRemoteObject(object string) error {
	if object == "" || strings.ContainsAny(object, "\\\x00\r\n\t\"") {
		return errors.New("backup object path is unsafe")
	}
	for _, segment := range strings.Split(object, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("backup object path is unsafe")
		}
	}
	return nil
}

func sftpQuote(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func safeJoin(root, object string) (string, error) {
	if root == "" || !filepath.IsAbs(root) || object == "" || strings.ContainsAny(object, "\\\x00\r\n") {
		return "", errors.New("backup object path is unsafe")
	}
	target := filepath.Join(root, filepath.FromSlash(object))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("backup object escapes destination")
	}
	return target, nil
}

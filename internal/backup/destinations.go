package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/runner"
)

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
		return uploadLocal(destination.Target, source, object)
	case DestinationSFTP:
		return uploadSFTP(ctx, destination.Target, credentials, source, object)
	case DestinationS3:
		return uploadS3(ctx, destination.Target, credentials, source, object)
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
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := UploadAndVerifyWithTimeout(destination, credentials, source, object, expectedChecksum, expectedBytes); err == nil {
			return nil
		} else {
			last = err
		}
		if attempt+1 < attempts {
			time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
		}
	}
	return last
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
		return output.Close()
	case DestinationSFTP:
		return downloadSFTP(ctx, destination.Target, credentials, object, target)
	case DestinationS3:
		return downloadS3(ctx, destination.Target, credentials, object, target)
	default:
		return fmt.Errorf("unsupported backup destination type %q", destination.Type)
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
	if err := temporary.Chmod(0o600); err != nil || temporary.Close() != nil {
		cleanup()
		return errors.New("close local backup copy")
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		cleanup()
		return err
	}
	return nil
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
	bounded, cancel := runner.Context(ctx)
	defer cancel()
	command := exec.CommandContext(bounded, "sftp", args...)
	command.Stdin = strings.NewReader("-mkdir " + filepath.ToSlash(path.Dir(remote)) + "\nput " + source + " " + remote + "\n")
	if output, err := command.CombinedOutput(); err != nil {
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
	bounded, cancel := runner.Context(ctx)
	defer cancel()
	command := exec.CommandContext(bounded, "sftp", args...)
	command.Stdin = strings.NewReader("rm " + remote + "\n")
	if output, err := command.CombinedOutput(); err != nil {
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
	bounded, cancel := runner.Context(ctx)
	defer cancel()
	command := exec.CommandContext(bounded, "sftp", args...)
	command.Stdin = strings.NewReader("get " + remote + " " + destination + "\n")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("SFTP download failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func validateRemoteObject(object string) error {
	if object == "" || strings.ContainsAny(object, "\\\x00\r\n \t") {
		return errors.New("backup object path is unsafe")
	}
	for _, segment := range strings.Split(object, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("backup object path is unsafe")
		}
	}
	return nil
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

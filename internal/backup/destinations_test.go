package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSFTPCommandsUseBoundedRunnerAndValidatedArguments(t *testing.T) {
	original := runSFTPCommand
	t.Cleanup(func() { runSFTPCommand = original })
	var commands []string
	runSFTPCommand = func(_ context.Context, stdin io.Reader, command string, args ...string) ([]byte, error) {
		data, _ := io.ReadAll(stdin)
		commands = append(commands, command+" "+strings.Join(args, " ")+" :: "+string(data))
		return nil, nil
	}
	destination := Destination{Type: DestinationSFTP, Target: "sftp://backup.example/base", Retention: DefaultRetention()}
	credentials := Credentials{Username: "nas", PrivateKeyPath: "/etc/lumonas/key"}
	if err := uploadSFTP(context.Background(), destination.Target, credentials, "/tmp/bundle.mrb", "recovery/latest.mrb"); err != nil {
		t.Fatal(err)
	}
	if err := deleteSFTP(context.Background(), destination.Target, credentials, "recovery/latest.mrb"); err != nil {
		t.Fatal(err)
	}
	if err := downloadSFTP(context.Background(), destination.Target, credentials, "recovery/latest.mrb", "/tmp/restored.mrb"); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 3 || !strings.Contains(commands[0], "sftp") || !strings.Contains(commands[0], `put "/tmp/bundle.mrb" "/base/recovery/latest.mrb.lumonas-upload-`) || !strings.Contains(commands[0], `rename "/base/recovery/latest.mrb.lumonas-upload-`) || !strings.Contains(commands[0], `" "/base/recovery/latest.mrb"`) || !strings.Contains(commands[1], `rm "/base/recovery/latest.mrb"`) || !strings.Contains(commands[2], `get "/base/recovery/latest.mrb" "/tmp/restored.mrb"`) {
		t.Fatalf("unexpected SFTP commands: %v", commands)
	}
}

func TestSFTPPathsWithSpacesAreQuoted(t *testing.T) {
	original := runSFTPCommand
	t.Cleanup(func() { runSFTPCommand = original })
	var script string
	runSFTPCommand = func(_ context.Context, input io.Reader, _ string, _ ...string) ([]byte, error) {
		data, _ := io.ReadAll(input)
		script = string(data)
		return nil, nil
	}
	if err := uploadSFTP(context.Background(), "sftp://backup.example/base", Credentials{Username: "nas", PrivateKeyPath: "/tmp/ssh key"}, "/tmp/source file", "Family Photos/image 1.jpg"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, `put "/tmp/source file" "/base/Family Photos/image 1.jpg.lumonas-upload-`) || !strings.Contains(script, `rename "/base/Family Photos/image 1.jpg.lumonas-upload-`) {
		t.Fatalf("SFTP paths were not safely quoted: %q", script)
	}
}

func TestSFTPListRecursesAndReturnsRelativeFileMetadata(t *testing.T) {
	original := runSFTPCommand
	t.Cleanup(func() { runSFTPCommand = original })
	var commands []string
	runSFTPCommand = func(_ context.Context, input io.Reader, _ string, _ ...string) ([]byte, error) {
		data, _ := io.ReadAll(input)
		commands = append(commands, string(data))
		if strings.Contains(string(data), `"/base/snapshots/Family Photos"`) {
			return []byte("-rw-r--r-- 1 nas users 7 Sep 28 2026 image 1.jpg\n"), nil
		}
		return []byte("drwxr-xr-x 2 nas users 0 Sep 28 2026 Family Photos\n-rw-r--r-- 1 nas users 3 Sep 28 2026 root.txt\n"), nil
	}
	files, err := listSFTP(context.Background(), "sftp://backup.example/base", Credentials{Username: "nas", PrivateKeyPath: "/tmp/key"}, "snapshots")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "root.txt" || files[1].Path != "Family Photos/image 1.jpg" || files[1].Size != 7 || files[1].ModTime.IsZero() {
		t.Fatalf("unexpected SFTP listing: %#v", files)
	}
	if len(commands) != 2 {
		t.Fatalf("expected recursive listing, got %v", commands)
	}
}

func TestSFTPListReturnsRemoteErrors(t *testing.T) {
	original := runSFTPCommand
	t.Cleanup(func() { runSFTPCommand = original })
	runSFTPCommand = func(context.Context, io.Reader, string, ...string) ([]byte, error) {
		return []byte("permission denied"), errors.New("exit status 1")
	}
	_, err := listSFTP(context.Background(), "sftp://backup.example/base", Credentials{Username: "nas", PrivateKeyPath: "/tmp/key"}, "source")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected actionable SFTP listing error, got %v", err)
	}
}

func TestLocalUploadIsAtomicAndRejectsEscapes(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.mrb")
	if err := os.WriteFile(source, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	destination := Destination{ID: "local", Name: "Local", Type: DestinationLocal, Target: target, Retention: DefaultRetention()}
	if err := Upload(context.Background(), destination, Credentials{}, source, "recovery/latest.mrb"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "recovery", "latest.mrb"))
	if err != nil || string(data) != "bundle" {
		t.Fatalf("unexpected uploaded data %q: %v", data, err)
	}
	if err := Upload(context.Background(), destination, Credentials{}, source, "../escape.mrb"); err == nil {
		t.Fatal("expected escaping object to fail")
	}
}

func TestLocalUploadAndDownloadVerificationChecksThePromotedCopy(t *testing.T) {
	source := filepath.Join(t.TempDir(), "bundle.mrb")
	data := []byte("verified bundle")
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	destination := Destination{ID: "local", Name: "Local", Type: DestinationLocal, Target: t.TempDir(), Retention: DefaultRetention()}
	object := "recovery/verified.mrb"
	if err := UploadAndVerifyWithRetry(destination, Credentials{}, source, object, hex.EncodeToString(digest[:]), int64(len(data)), 2); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "downloaded.mrb")
	if err := Download(context.Background(), destination, Credentials{}, object, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != string(data) {
		t.Fatalf("unexpected downloaded copy %q: %v", got, err)
	}
	if err := Download(context.Background(), destination, Credentials{}, "../escape.mrb", target); err == nil {
		t.Fatal("expected unsafe remote object to be rejected")
	}
}

func TestLocalUploadPublishesDurableMode0600Copy(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.mrb")
	if err := os.WriteFile(source, []byte("durable bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	destination := Destination{ID: "local", Name: "Local", Type: DestinationLocal, Target: target, Retention: DefaultRetention()}
	if err := Upload(context.Background(), destination, Credentials{}, source, "recovery/durable.mrb"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(target, "recovery", "durable.mrb")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("local recovery copy mode = %o, want 600", info.Mode().Perm())
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "durable bundle" {
		t.Fatalf("unexpected durable copy %q: %v", data, err)
	}
}

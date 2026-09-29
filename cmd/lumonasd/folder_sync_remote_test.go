package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/foldersync"
)

func TestRemoteSyncManifestUsesLiveListingAndHashesDeepChecks(t *testing.T) {
	remote := backup.Destination{ID: "remote-test", Type: backup.DestinationS3, Enabled: true, Target: "https://s3.example.test/bucket"}
	files := []backup.RemoteFile{{Path: "added-outside-lumonas.txt", Size: int64(len("remote-content")), ModTime: time.Now().UTC()}}
	var listedPrefix, downloadedObject string
	manifest, err := remoteSyncManifest(context.Background(), remote, backup.Credentials{}, "sync-prefix", true,
		func(_ context.Context, _ backup.Destination, _ backup.Credentials, prefix string) ([]backup.RemoteFile, error) {
			listedPrefix = prefix
			return files, nil
		},
		func(_ context.Context, _ backup.Destination, _ backup.Credentials, object, target string) error {
			downloadedObject = object
			return os.WriteFile(target, []byte("remote-content"), 0o600)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if listedPrefix != "sync-prefix" || downloadedObject != "sync-prefix/added-outside-lumonas.txt" {
		t.Fatalf("remote prefix was not applied consistently: listed=%q downloaded=%q", listedPrefix, downloadedObject)
	}
	entry, ok := manifest["added-outside-lumonas.txt"]
	if !ok || entry.Hash == "" || entry.Size != int64(len("remote-content")) {
		t.Fatalf("remote deep manifest did not include verified content metadata: %#v", manifest)
	}
	source := map[string]foldersync.Entry{}
	plan := foldersync.BuildPlan(source, manifest, true, true)
	if plan.Deletes != 1 || len(plan.Changes) != 1 || plan.Changes[0].Path != "added-outside-lumonas.txt" {
		t.Fatalf("mirror plan missed a remotely added file: %#v", plan)
	}
}

func TestRemoteSyncManifestRejectsUnsafePathsAndSizeChanges(t *testing.T) {
	remote := backup.Destination{ID: "remote-test", Type: backup.DestinationSFTP, Enabled: true, Target: "sftp://user@example.test/backups"}
	list := func(_ context.Context, _ backup.Destination, _ backup.Credentials, _ string) ([]backup.RemoteFile, error) {
		return []backup.RemoteFile{{Path: "../outside", Size: 1}}, nil
	}
	if _, err := remoteSyncManifest(context.Background(), remote, backup.Credentials{}, "", false, list, nil); err == nil {
		t.Fatal("unsafe remote listing path was accepted")
	}
	list = func(_ context.Context, _ backup.Destination, _ backup.Credentials, _ string) ([]backup.RemoteFile, error) {
		return []backup.RemoteFile{{Path: "file.txt", Size: 20}}, nil
	}
	_, err := remoteSyncManifest(context.Background(), remote, backup.Credentials{}, "", true, list,
		func(_ context.Context, _ backup.Destination, _ backup.Credentials, _ string, target string) error {
			return os.WriteFile(target, []byte("short"), 0o600)
		})
	if err == nil {
		t.Fatal("deep manifest accepted a remote file that changed size during download")
	}
}

func TestRemoteSyncPreviewLeavesNoTemporaryDownload(t *testing.T) {
	remote := backup.Destination{ID: "remote-test", Type: backup.DestinationS3, Enabled: true, Target: "https://s3.example.test/bucket"}
	before, err := filepath.Glob(filepath.Join(os.TempDir(), ".lumonas-sync-preview-*"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = remoteSyncManifest(context.Background(), remote, backup.Credentials{}, "", true,
		func(_ context.Context, _ backup.Destination, _ backup.Credentials, _ string) ([]backup.RemoteFile, error) {
			return []backup.RemoteFile{{Path: "file.txt", Size: 4}}, nil
		},
		func(_ context.Context, _ backup.Destination, _ backup.Credentials, _ string, target string) error {
			return os.WriteFile(target, []byte("data"), 0o600)
		})
	if err != nil {
		t.Fatal(err)
	}
	after, err := filepath.Glob(filepath.Join(os.TempDir(), ".lumonas-sync-preview-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("temporary remote preview files leaked: before=%v after=%v", before, after)
	}
}

package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppdataSourcesResolvesBindMountsAndNamedVolumes(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "media")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  media:\n    image: example/media:latest\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	service := New(root, func(_ context.Context, name string, args ...string) ([]byte, error) {
		command := name + " " + strings.Join(args, " ")
		if strings.Contains(command, "config --format json") {
			return []byte(`{"services":{"media":{"volumes":[{"type":"bind","source":"/srv/pools/apps/media","target":"/config"},{"type":"volume","source":"media-db","target":"/var/lib/db"}]}}}`), nil
		}
		if strings.Contains(command, "volume inspect") {
			return []byte(`"/var/lib/docker/volumes/media-db/_data"`), nil
		}
		return nil, nil
	})
	stack := Stack{Name: "media", Recovery: &RecoveryContract{AppdataPaths: []string{"/config", "/var/lib/db"}}}
	sources, err := service.AppdataSources(context.Background(), stack)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].HostPath != "/srv/pools/apps/media" || sources[1].HostPath != "/var/lib/docker/volumes/media-db/_data" {
		t.Fatalf("unexpected appdata sources: %#v", sources)
	}
}

func TestAppdataSourcesRejectsUnapprovedMountsAndMissingTargets(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "unsafe")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  app:\n    image: example/app:latest\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	service := New(root, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte(`{"services":{"app":{"volumes":[{"type":"bind","source":"/etc","target":"/config"}]}}}`), nil
	})
	stack := Stack{Name: "unsafe", Recovery: &RecoveryContract{AppdataPaths: []string{"/config"}}}
	if _, err := service.AppdataSources(context.Background(), stack); err == nil {
		t.Fatal("unapproved appdata source was accepted")
	}
	stack.Recovery.AppdataPaths = []string{"/missing"}
	if _, err := service.AppdataSources(context.Background(), stack); err == nil {
		t.Fatal("missing appdata target was accepted")
	}
}

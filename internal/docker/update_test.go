package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func updateTestStack(t *testing.T, service Service, name string) Stack {
	t.Helper()
	stackDir := filepath.Join(service.Root, name)
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	compose := "services:\n  " + name + ":\n    image: example/app:latest\n"
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte(compose), 0o640); err != nil {
		t.Fatal(err)
	}
	return Stack{ID: "stack-" + name, Name: name}
}

func fastOptions() UpdateOptions {
	return UpdateOptions{GracePeriod: time.Millisecond, Interval: time.Millisecond, Timeout: 20 * time.Millisecond}
}

func TestUpdateStackRollsBackWhenProbeKeepsFailing(t *testing.T) {
	var commands [][]string
	root := t.TempDir()
	service := New(root, func(_ context.Context, name string, args ...string) ([]byte, error) {
		command := append([]string{name}, args...)
		commands = append(commands, command)
		joined := strings.Join(command, " ")
		switch {
		case strings.HasSuffix(joined, "images --format json"):
			return []byte(`{"Repository":"example/app","Tag":"latest","ID":"sha256:oldid"}` + "\n"), nil
		case strings.HasSuffix(joined, "pull"):
			return nil, nil
		case strings.Contains(joined, "up -d --remove-orphans"):
			return nil, nil
		case strings.Contains(joined, "tag sha256:oldid example/app:latest"):
			return nil, nil
		case strings.Contains(joined, "up -d --force-recreate"):
			return nil, nil
		}
		return nil, nil
	})
	stack := updateTestStack(t, service, "rollback")

	attempts := 0
	probe := func(context.Context) error {
		attempts++
		return errors.New("container app is restarting")
	}

	result, err := service.UpdateStack(context.Background(), stack, probe, fastOptions())
	if err == nil {
		t.Fatal("expected update to fail its health gate")
	}
	if !result.RolledBack {
		t.Fatalf("expected rollback, got %#v", result)
	}
	if attempts < 2 {
		t.Fatalf("probe should have been retried, ran %d time(s)", attempts)
	}
	tagged := false
	for _, command := range commands {
		if strings.Join(command, " ") == "docker tag sha256:oldid example/app:latest" {
			tagged = true
		}
	}
	if !tagged {
		t.Fatal("rollback did not re-point the previous image")
	}
}

func TestUpdateStackSucceedsWhenProbePasses(t *testing.T) {
	root := t.TempDir()
	service := New(root, func(_ context.Context, name string, args ...string) ([]byte, error) {
		joined := strings.Join(append([]string{name}, args...), " ")
		if strings.HasSuffix(joined, "images --format json") {
			return []byte(`{"Repository":"example/app","Tag":"latest","ID":"sha256:current"}` + "\n"), nil
		}
		return nil, nil
	})
	stack := updateTestStack(t, service, "happy")

	probes := 0
	probe := func(context.Context) error {
		probes++
		return nil
	}

	result, err := service.UpdateStack(context.Background(), stack, probe, fastOptions())
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if !result.Updated || result.RolledBack {
		t.Fatalf("unexpected result: %#v", result)
	}
	if probes != 1 {
		t.Fatalf("healthy stack should probe once, got %d", probes)
	}
}

func TestUpdateStackFailsFastOnPullError(t *testing.T) {
	root := t.TempDir()
	service := New(root, func(_ context.Context, name string, args ...string) ([]byte, error) {
		joined := strings.Join(append([]string{name}, args...), " ")
		if strings.HasSuffix(joined, "images --format json") {
			return []byte(`{"Repository":"example/app","Tag":"latest","ID":"sha256:old"}` + "\n"), nil
		}
		if strings.HasSuffix(joined, "pull") {
			return nil, errors.New("registry unreachable")
		}
		return nil, nil
	})
	stack := updateTestStack(t, service, "pullfail")

	probeCalled := false
	_, err := service.UpdateStack(context.Background(), stack, func(context.Context) error {
		probeCalled = true
		return nil
	}, fastOptions())
	if err == nil {
		t.Fatal("expected pull failure to abort the update")
	}
	if !strings.Contains(err.Error(), "pull failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if probeCalled {
		t.Fatal("health probe ran despite pull failure")
	}
}

func TestUpdateStackRequiresProbe(t *testing.T) {
	service := New(t.TempDir(), nil)
	if _, err := service.UpdateStack(context.Background(), Stack{Name: "x"}, nil, fastOptions()); err == nil {
		t.Fatal("expected missing probe to be rejected")
	}
}

func TestCheckImageUpdatesFlagsChangedDigests(t *testing.T) {
	service := New(t.TempDir(), func(_ context.Context, name string, args ...string) ([]byte, error) {
		joined := strings.Join(append([]string{name}, args...), " ")
		switch {
		case strings.HasSuffix(joined, "images --format {{json .}}"):
			return []byte("{\"ID\":\"img1\",\"Repository\":\"example/app\",\"Tag\":\"latest\",\"Size\":\"100MB\",\"CreatedSince\":\"2 days ago\"}\n" +
				"{\"ID\":\"img2\",\"Repository\":\"example/stale\",\"Tag\":\"1.0\",\"Size\":\"50MB\",\"CreatedSince\":\"30 days ago\"}\n"), nil
		case strings.HasSuffix(joined, "ps -a --format {{json .}}"):
			return []byte(`{"ID":"c1","Names":"app","Image":"example/app:latest","State":"Up 1 hour","Labels":"com.docker.compose.project=media"}` + "\n"), nil
		case strings.Contains(joined, "image inspect --format {{index .RepoDigests 0}} example/app:latest"):
			return []byte("example/app@sha256:samelocal\n"), nil
		case strings.Contains(joined, "manifest inspect --verbose example/app:latest"):
			return []byte(`{"Descriptor":{"digest":"sha256:samelocal"}}`), nil
		case strings.Contains(joined, "image inspect --format {{index .RepoDigests 0}} example/stale:1.0"):
			return []byte("example/stale@sha256:localold\n"), nil
		case strings.Contains(joined, "manifest inspect --verbose example/stale:1.0"):
			return []byte(`{"Descriptor":{"digest":"sha256:remoteupstream"}}`), nil
		}
		return nil, nil
	})

	images, err := service.CheckImageUpdates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 {
		t.Fatalf("unexpected images %#v", images)
	}
	byRepo := map[string]Image{}
	for _, image := range images {
		byRepo[image.Repo] = image
	}
	if byRepo["example/app"].UpdateAvailable {
		t.Fatal("unchanged digest flagged as update")
	}
	if !byRepo["example/app"].InUse {
		t.Fatal("in-use image was not marked in use")
	}
	if !byRepo["example/stale"].UpdateAvailable {
		t.Fatal("changed digest was not flagged")
	}
}

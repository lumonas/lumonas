package docker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEngineAPIReadOnlyCollectors(t *testing.T) {
	client := testEngineClient(func(path string) (int, string) {
		switch path {
		case "/version":
			return http.StatusOK, `{"Version":"27.0.0"}`
		case "/containers/json":
			return http.StatusOK, `[{"Id":"container-1","Names":["/media"],"Image":"example/media:latest","ImageID":"sha256:image-1","State":"running","Status":"Up 2 hours (unhealthy)","Labels":{"com.docker.compose.project":"media"},"Ports":[{"PublicPort":8096,"PrivatePort":8096,"Type":"tcp"}]}]`
		case "/containers/container-1/json":
			return http.StatusOK, `{"RestartCount":3,"State":{"StartedAt":"2026-09-13T10:00:00.000000000Z"}}`
		case "/containers/container-1/stats":
			return http.StatusOK, `{"memory_stats":{"usage":1048576},"cpu_stats":{"cpu_usage":{"total_usage":200,"percpu_usage":[100,100]},"system_cpu_usage":1000,"online_cpus":2},"precpu_stats":{"cpu_usage":{"total_usage":100},"system_cpu_usage":500}}`
		case "/images/json":
			payload, _ := json.Marshal([]map[string]any{
				{"Id": "sha256:image-1", "RepoTags": []string{"example/media:latest", "example/media:stable"}, "Size": uint64(1234), "Created": time.Now().Add(-48 * time.Hour).Unix()},
				{"Id": "sha256:image-2", "RepoTags": []string{"example/unused:latest"}, "Size": uint64(5678), "Created": time.Now().Add(-72 * time.Hour).Unix()},
			})
			return http.StatusOK, string(payload)
		case "/volumes":
			return http.StatusOK, `{"Volumes":[{"Name":"media-data"}]}`
		case "/system/df":
			return http.StatusOK, `{"Volumes":[{"Id":"media-data","Names":["media-data"],"UsageData":{"Size":4096}}]}`
		case "/containers/container-1/logs":
			payload := []byte("2026-09-13T12:00:00.000000000Z warning message\n")
			frame := make([]byte, 8+len(payload))
			frame[0] = 2
			binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
			copy(frame[8:], payload)
			return http.StatusOK, string(frame)
		default:
			return http.StatusNotFound, "not found"
		}
	})
	service := Service{engine: client}
	if !service.Available(context.Background()) {
		t.Fatal("Engine API should report the test daemon as available")
	}
	containers, err := service.Containers(context.Background())
	if err != nil || len(containers) != 1 {
		t.Fatalf("containers = %#v, err=%v", containers, err)
	}
	if containers[0].Name != "media" || containers[0].StackID != "media" || containers[0].State != "unhealthy" || len(containers[0].Ports) != 1 || containers[0].Ports[0].Host != 8096 || containers[0].Restarts != 3 || containers[0].RAMUsedBytes != 1048576 || containers[0].CPUPercent != 40 || containers[0].StartedAt == nil || containers[0].StartedAt.Format(time.RFC3339) != "2026-09-13T10:00:00Z" {
		t.Fatalf("unexpected container mapping: %#v", containers[0])
	}
	images, err := service.Images(context.Background())
	if err != nil || len(images) != 3 || images[0].SizeBytes != 1234 || images[0].CreatedDaysAgo < 1 || !images[0].InUse || !images[1].InUse || images[2].InUse {
		t.Fatalf("unexpected image mapping: %#v, err=%v", images, err)
	}
	volumes, err := service.Volumes(context.Background())
	if err != nil || len(volumes) != 1 || volumes[0].ID != "media-data" || volumes[0].UsedBytes != 4096 {
		t.Fatalf("unexpected volume mapping: %#v, err=%v", volumes, err)
	}
	logs, err := service.Logs(context.Background(), "container-1", 25)
	if err != nil || len(logs) != 1 || logs[0].Level != "warn" || logs[0].Message != "warning message" {
		t.Fatalf("unexpected log mapping: %#v, err=%v", logs, err)
	}
}

func TestImageUsageFallsBackToContainerReference(t *testing.T) {
	client := testEngineClient(func(path string) (int, string) {
		switch path {
		case "/images/json":
			return http.StatusOK, `[{"Id":"sha256:image-1","RepoTags":["example/media:latest"]}]`
		case "/containers/json":
			return http.StatusOK, `[{"Image":"example/media:latest"}]`
		default:
			return http.StatusNotFound, "not found"
		}
	})
	images, err := (Service{engine: client}).Images(context.Background())
	if err != nil || len(images) != 1 || !images[0].InUse {
		t.Fatalf("reference fallback did not mark image in use: %#v, err=%v", images, err)
	}
}

func TestImageIDNormalization(t *testing.T) {
	if !imageInUse("sha256:image-1", "example/media:latest", map[string]struct{}{"image-1": {}}, nil) {
		t.Fatal("sha256 image ID was not normalized")
	}
	if imageInUse("sha256:image-2", "example/media:latest", nil, map[string]struct{}{"other:latest": {}}) {
		t.Fatal("unrelated image reference was marked in use")
	}
}

func TestValidateReadOnlyEnginePath(t *testing.T) {
	allowed := []string{
		"/version",
		"/containers/json?all=true",
		"/containers/container-1/json",
		"/containers/container-1/stats?stream=false",
		"/containers/container-1/logs?stdout=1&stderr=1&timestamps=1&tail=25",
		"/images/json",
		"/volumes",
		"/system/df",
	}
	for _, path := range allowed {
		if err := ValidateReadOnlyEnginePath(path); err != nil {
			t.Errorf("allowed path %q rejected: %v", path, err)
		}
	}
	for _, path := range []string{
		"/containers/json",
		"/containers/container-1/stats?stream=true",
		"/containers/container-1/logs?stdout=1&stderr=1&timestamps=1&tail=5000",
		"/containers/container-1/exec",
		"http://127.0.0.1/containers/json?all=true",
	} {
		if err := ValidateReadOnlyEnginePath(path); err == nil {
			t.Errorf("unsafe path %q was accepted", path)
		}
	}
}

func TestVolumeUsageFailureKeepsInventory(t *testing.T) {
	client := testEngineClient(func(path string) (int, string) {
		if path == "/volumes" {
			return http.StatusOK, `{"Volumes":[{"Name":"media-data"}]}`
		}
		return http.StatusNotFound, "not found"
	})
	volumes, err := (Service{engine: client}).Volumes(context.Background())
	if err != nil || len(volumes) != 1 || volumes[0].Name != "media-data" || volumes[0].UsedBytes != 0 {
		t.Fatalf("volume inventory was not preserved after accounting failure: %#v, err=%v", volumes, err)
	}
}

func TestEngineAPIRejectsEngineErrors(t *testing.T) {
	client := testEngineClient(func(path string) (int, string) {
		if path == "/version" {
			return http.StatusInternalServerError, strings.Repeat("x", 600)
		}
		return http.StatusNotFound, "not found"
	})
	if client.available(context.Background()) {
		t.Fatal("HTTP 500 must not report Docker as available")
	}
	service := Service{engine: client}
	if _, err := service.Containers(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("Engine HTTP errors must remain visible to callers, got %v", err)
	}
	if err := client.get(context.Background(), "/version", nil); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected bounded Engine HTTP error, got %v", err)
	}
}

func TestContainerCPUPercent(t *testing.T) {
	stats := containerStatsResponse{}
	stats.CPUStats.CPUUsage.TotalUsage = 200
	stats.CPUStats.SystemCPUUsage = 1000
	stats.CPUStats.OnlineCPUs = 2
	stats.PreCPUStats.CPUUsage.TotalUsage = 100
	stats.PreCPUStats.SystemCPUUsage = 500
	if got := containerCPUPercent(stats); got != 40 {
		t.Fatalf("containerCPUPercent() = %v, want 40", got)
	}

	stats.CPUStats.SystemCPUUsage = stats.PreCPUStats.SystemCPUUsage
	if got := containerCPUPercent(stats); got != 0 {
		t.Fatalf("containerCPUPercent() with no system delta = %v, want 0", got)
	}
}

func testEngineClient(handler func(path string) (int, string)) *engineClient {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		status, body := handler(request.URL.Path)
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})
	return &engineClient{client: &http.Client{Transport: transport, Timeout: time.Second}}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSplitImageReference(t *testing.T) {
	tests := map[string][2]string{
		"nginx":                {"nginx", "latest"},
		"registry:5000/app":    {"registry:5000/app", "latest"},
		"registry:5000/app:v1": {"registry:5000/app", "v1"},
	}
	for input, expected := range tests {
		repo, tag := splitImageReference(input)
		if repo != expected[0] || tag != expected[1] {
			t.Fatalf("splitImageReference(%q) = %q:%q, want %q:%q", input, repo, tag, expected[0], expected[1])
		}
	}
}

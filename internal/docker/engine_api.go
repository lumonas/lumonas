package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDockerSocket = "/var/run/docker.sock"
	maxEngineResponse   = 16 << 20
)

// engineClient is the production read-only Docker integration. Compose
// remains a separate CLI concern, but normal status collection must not parse
// human-oriented docker ps/images/volume output.
type engineClient struct {
	client *http.Client
}

func newEngineClient(socket string) *engineClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
		},
		DisableKeepAlives: true,
	}
	return &engineClient{client: &http.Client{Transport: transport, Timeout: 30 * time.Second}}
}

func dockerSocket() string {
	if socket := strings.TrimSpace(os.Getenv("LUMONAS_DOCKER_SOCKET")); socket != "" {
		return socket
	}
	return defaultDockerSocket
}

func (c *engineClient) get(ctx context.Context, path string, destination any) error {
	body, err := c.request(ctx, http.MethodGet, path)
	if err != nil {
		return err
	}
	if destination == nil {
		return nil
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("decode Docker Engine response: %w", err)
	}
	return nil
}

func (c *engineClient) request(ctx context.Context, method, path string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Docker Engine request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxEngineResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read Docker Engine response: %w", err)
	}
	if len(body) > maxEngineResponse {
		return nil, errors.New("Docker Engine response exceeded size limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(body))
		if len(message) > 512 {
			message = message[:512]
		}
		return nil, fmt.Errorf("Docker Engine returned HTTP %d: %s", response.StatusCode, message)
	}
	return body, nil
}

func (c *engineClient) available(ctx context.Context) bool {
	var response struct {
		Version string `json:"Version"`
	}
	return c.get(ctx, "/version", &response) == nil && response.Version != ""
}

func (c *engineClient) containers(ctx context.Context) ([]Container, error) {
	var rows []struct {
		ID     string            `json:"Id"`
		Names  []string          `json:"Names"`
		Image  string            `json:"Image"`
		State  string            `json:"State"`
		Status string            `json:"Status"`
		Labels map[string]string `json:"Labels"`
		Ports  []struct {
			PublicPort  uint16 `json:"PublicPort"`
			PrivatePort uint16 `json:"PrivatePort"`
			Type        string `json:"Type"`
		} `json:"Ports"`
	}
	if err := c.get(ctx, "/containers/json?all=true", &rows); err != nil {
		return nil, err
	}
	result := make([]Container, 0, len(rows))
	for _, row := range rows {
		name := row.ID
		if len(row.Names) > 0 {
			name = strings.TrimPrefix(row.Names[0], "/")
		}
		ports := make([]Port, 0, len(row.Ports))
		for _, port := range row.Ports {
			ports = append(ports, Port{Host: int(port.PublicPort), Container: int(port.PrivatePort), Label: port.Type})
		}
		container := Container{
			ID: row.ID, Name: name, StackID: row.Labels["com.docker.compose.project"],
			Image: row.Image, State: normalizeEngineState(row.State, row.Status), Ports: ports,
		}
		c.enrichContainer(ctx, &container)
		result = append(result, container)
	}
	return result, nil
}

type containerInspectResponse struct {
	RestartCount int `json:"RestartCount"`
	State        struct {
		StartedAt string `json:"StartedAt"`
	} `json:"State"`
}

type containerStatsResponse struct {
	MemoryStats struct {
		Usage uint64 `json:"usage"`
	} `json:"memory_stats"`
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64   `json:"total_usage"`
			PerCPU     []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint64 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
}

// enrichContainer uses the Engine's machine-readable inspect and one-shot
// stats endpoints. Inventory remains useful when either optional endpoint is
// unavailable, so enrichment errors are intentionally best-effort and never
// make the entire read-only container list fail.
func (c *engineClient) enrichContainer(ctx context.Context, container *Container) {
	if container == nil || container.ID == "" {
		return
	}
	path := "/containers/" + url.PathEscape(container.ID)
	var inspect containerInspectResponse
	if err := c.get(ctx, path+"/json", &inspect); err == nil {
		container.Restarts = maxInt(inspect.RestartCount, 0)
		if startedAt, err := time.Parse(time.RFC3339Nano, inspect.State.StartedAt); err == nil && !startedAt.IsZero() {
			container.StartedAt = &startedAt
		}
	}
	var stats containerStatsResponse
	if err := c.get(ctx, path+"/stats?stream=false", &stats); err == nil {
		container.RAMUsedBytes = stats.MemoryStats.Usage
		container.CPUPercent = containerCPUPercent(stats)
	}
}

func maxInt(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}

func containerCPUPercent(stats containerStatsResponse) float64 {
	if stats.CPUStats.SystemCPUUsage <= stats.PreCPUStats.SystemCPUUsage || stats.CPUStats.CPUUsage.TotalUsage < stats.PreCPUStats.CPUUsage.TotalUsage {
		return 0
	}
	cpuDelta := stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage
	systemDelta := stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage
	if cpuDelta == 0 || systemDelta == 0 {
		return 0
	}
	onlineCPUs := stats.CPUStats.OnlineCPUs
	if onlineCPUs == 0 {
		onlineCPUs = uint64(len(stats.CPUStats.CPUUsage.PerCPU))
	}
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}
	return float64(cpuDelta) / float64(systemDelta) * float64(onlineCPUs) * 100
}

func (c *engineClient) images(ctx context.Context) ([]Image, error) {
	var rows []struct {
		ID       string   `json:"Id"`
		RepoTags []string `json:"RepoTags"`
		Size     uint64   `json:"Size"`
		Created  int64    `json:"Created"`
	}
	if err := c.get(ctx, "/images/json", &rows); err != nil {
		return nil, err
	}
	result := make([]Image, 0, len(rows))
	for _, row := range rows {
		tags := row.RepoTags
		if len(tags) == 0 {
			tags = []string{"<none>:<none>"}
		}
		for _, reference := range tags {
			repo, tag := splitImageReference(reference)
			result = append(result, Image{ID: row.ID, Repo: repo, Tag: tag, SizeBytes: row.Size, CreatedDaysAgo: createdDaysAgo(row.Created)})
		}
	}
	return result, nil
}

func (c *engineClient) volumes(ctx context.Context) ([]Volume, error) {
	var response struct {
		Volumes []struct {
			Name string `json:"Name"`
		} `json:"Volumes"`
	}
	if err := c.get(ctx, "/volumes", &response); err != nil {
		return nil, err
	}
	result := make([]Volume, 0, len(response.Volumes))
	for _, volume := range response.Volumes {
		result = append(result, Volume{ID: volume.Name, Name: volume.Name})
	}
	return result, nil
}

func (c *engineClient) logs(ctx context.Context, container string, tail int) ([]LogLine, error) {
	path := "/containers/" + url.PathEscape(container) + "/logs?stdout=1&stderr=1&timestamps=1&tail=" + strconv.Itoa(tail)
	body, err := c.request(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	body = decodeDockerLogFrames(body)
	result := make([]LogLine, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		line := scanner.Text()
		timestamp := time.Now().UTC()
		message := line
		if space := strings.IndexByte(line, ' '); space > 0 {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, line[:space]); parseErr == nil {
				timestamp, message = parsed, line[space+1:]
			}
		}
		level := "info"
		lower := strings.ToLower(message)
		if strings.Contains(lower, "error") {
			level = "error"
		} else if strings.Contains(lower, "warn") {
			level = "warn"
		}
		result = append(result, LogLine{Container: container, Timestamp: timestamp, Level: level, Message: message})
	}
	return result, scanner.Err()
}

func normalizeEngineState(state, status string) string {
	if strings.Contains(strings.ToLower(status), "unhealthy") {
		return "unhealthy"
	}
	switch strings.ToLower(state) {
	case "running":
		return "running"
	case "restarting":
		return "restarting"
	case "exited", "dead":
		return "exited"
	default:
		return "created"
	}
}

func splitImageReference(reference string) (string, string) {
	last := strings.LastIndexByte(reference, ':')
	if last <= strings.LastIndexByte(reference, '/') {
		return reference, "latest"
	}
	return reference[:last], reference[last+1:]
}

func createdDaysAgo(created int64) int {
	if created <= 0 {
		return 0
	}
	days := int(time.Since(time.Unix(created, 0)) / (24 * time.Hour))
	if days < 0 {
		return 0
	}
	return days
}

func decodeDockerLogFrames(body []byte) []byte {
	if len(body) < 8 || (body[0] != 1 && body[0] != 2) {
		return body
	}
	var decoded []byte
	for offset := 0; offset+8 <= len(body); {
		length := int(binary.BigEndian.Uint32(body[offset+4 : offset+8]))
		if offset+8+length > len(body) {
			return body
		}
		decoded = append(decoded, body[offset+8:offset+8+length]...)
		offset += 8 + length
	}
	if len(decoded) == 0 {
		return body
	}
	return decoded
}

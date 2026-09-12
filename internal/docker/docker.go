package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/runner"
)

type Runner func(context.Context, string, ...string) ([]byte, error)

type Service struct {
	Root string
	Run  Runner
}

type Stack struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	CatalogID        string            `json:"catalogId,omitempty"`
	Category         string            `json:"category"`
	Status           string            `json:"status"`
	State            string            `json:"state"`
	Images           []string          `json:"images"`
	ComposeYAML      string            `json:"composeYaml"`
	Env              []EnvVar          `json:"env"`
	Storage          []StorageMapping  `json:"storage"`
	Ports            []Port            `json:"ports"`
	Risks            []string          `json:"risks"`
	CPUPercent       float64           `json:"cpuPercent"`
	RAMUsedBytes     uint64            `json:"ramUsedBytes"`
	Restarts         int               `json:"restarts"`
	LastDeploy       time.Time         `json:"lastDeploy"`
	Backup           BackupInfo        `json:"backup"`
	Recovery         *RecoveryContract `json:"recovery,omitempty"`
	RecoveryCoverage float64           `json:"recoveryCoverage"`
}
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Scope string `json:"scope"`
}
type StorageMapping struct {
	ContainerPath string `json:"containerPath"`
	ResourceID    string `json:"resourceId"`
	ResourceLabel string `json:"resourceLabel"`
}
type Port struct {
	Host      int    `json:"host"`
	Container int    `json:"container"`
	Label     string `json:"label,omitempty"`
}
type BackupInfo struct {
	Strategy         string     `json:"strategy"`
	LastBackupAt     *time.Time `json:"lastBackupAt,omitempty"`
	AppdataSizeBytes uint64     `json:"appdataSizeBytes"`
}
type Container struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	StackID      string     `json:"stackId,omitempty"`
	Image        string     `json:"image"`
	State        string     `json:"state"`
	CPUPercent   float64    `json:"cpuPercent"`
	RAMUsedBytes uint64     `json:"ramUsedBytes"`
	Restarts     int        `json:"restarts"`
	Ports        []Port     `json:"ports"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
}
type Image struct {
	ID              string `json:"id"`
	Repo            string `json:"repo"`
	Tag             string `json:"tag"`
	SizeBytes       uint64 `json:"sizeBytes"`
	CreatedDaysAgo  int    `json:"createdDaysAgo"`
	UpdateAvailable bool   `json:"updateAvailable"`
	InUse           bool   `json:"inUse"`
}
type Volume struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StackID   string `json:"stackId,omitempty"`
	StackName string `json:"stackName,omitempty"`
	UsedBytes uint64 `json:"usedBytes"`
	BindPath  string `json:"bindPath,omitempty"`
}
type LogLine struct {
	Container string    `json:"container"`
	Timestamp time.Time `json:"ts"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

func New(root string, run Runner) Service {
	if run == nil {
		run = commandRunner
	}
	return Service{Root: root, Run: run}
}

func commandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return runner.OutputContext(ctx, name, args...)
}

func (s Service) Available(ctx context.Context) bool {
	_, err := s.Run(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	return err == nil
}

func (s Service) Stacks(ctx context.Context) ([]Stack, error) {
	entries, err := os.ReadDir(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return []Stack{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]Stack, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		composePath := filepath.Join(s.Root, entry.Name(), "compose.yaml")
		data, err := os.ReadFile(composePath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, parseStack(entry.Name(), composePath, string(data)))
	}
	return result, nil
}

func (s Service) Containers(ctx context.Context) ([]Container, error) {
	out, err := s.Run(ctx, "docker", "ps", "-a", "--format", "{{json .}}")
	if err != nil {
		if isUnavailable(err) {
			return []Container{}, nil
		}
		return nil, err
	}
	var result []Container
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var row struct {
			ID     string `json:"ID"`
			Names  string `json:"Names"`
			Image  string `json:"Image"`
			State  string `json:"State"`
			Ports  string `json:"Ports"`
			Labels string `json:"Labels"`
		}
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		state := normalizeState(row.State)
		stackID := ""
		for _, label := range strings.Split(row.Labels, ",") {
			if strings.HasPrefix(label, "com.docker.compose.project=") {
				stackID = strings.TrimPrefix(label, "com.docker.compose.project=")
			}
		}
		result = append(result, Container{ID: row.ID, Name: row.Names, StackID: stackID, Image: row.Image, State: state, Ports: parsePorts(row.Ports)})
	}
	return result, nil
}

func (s Service) Images(ctx context.Context) ([]Image, error) {
	out, err := s.Run(ctx, "docker", "images", "--format", "{{json .}}")
	if err != nil {
		if isUnavailable(err) {
			return []Image{}, nil
		}
		return nil, err
	}
	var result []Image
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var row struct {
			ID           string `json:"ID"`
			Repository   string `json:"Repository"`
			Tag          string `json:"Tag"`
			Size         string `json:"Size"`
			CreatedSince string `json:"CreatedSince"`
		}
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		result = append(result, Image{ID: row.ID, Repo: row.Repository, Tag: row.Tag, SizeBytes: parseSize(row.Size), CreatedDaysAgo: parseDays(row.CreatedSince)})
	}
	return result, nil
}

func (s Service) Volumes(ctx context.Context) ([]Volume, error) {
	out, err := s.Run(ctx, "docker", "volume", "ls", "--format", "{{json .}}")
	if err != nil {
		if isUnavailable(err) {
			return []Volume{}, nil
		}
		return nil, err
	}
	var result []Volume
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var row struct {
			ID     string `json:"ID"`
			Name   string `json:"Name"`
			Driver string `json:"Driver"`
		}
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		result = append(result, Volume{ID: row.ID, Name: row.Name})
	}
	return result, nil
}

func (s Service) Logs(ctx context.Context, container string, tail int) ([]LogLine, error) {
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(container) {
		return nil, errors.New("invalid container name")
	}
	if tail < 1 || tail > 1000 {
		tail = 200
	}
	out, err := s.Run(ctx, "docker", "logs", "--timestamps", "--tail", strconv.Itoa(tail), container)
	if err != nil {
		return nil, err
	}
	result := make([]LogLine, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
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

func (s Service) Action(ctx context.Context, stack Stack, action string) error {
	if action != "start" && action != "stop" && action != "restart" && action != "deploy" && action != "update" {
		return fmt.Errorf("unsupported stack action %q", action)
	}
	composePath := filepath.Join(s.Root, stack.Name, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		return err
	}
	composeAction := action
	if action == "deploy" || action == "update" {
		composeAction = "up"
	}
	args := []string{"docker", "compose", "-f", composePath}
	if action == "update" {
		args = append(args, "pull")
		if _, err := s.Run(ctx, args[0], args[1:]...); err != nil {
			return err
		}
		args = []string{"docker", "compose", "-f", composePath}
	}
	args = append(args, composeAction)
	if composeAction == "up" {
		args = append(args, "-d", "--remove-orphans")
	}
	_, err := s.Run(ctx, args[0], args[1:]...)
	return err
}

func (s Service) ContainerAction(ctx context.Context, id, action string) error {
	if action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("unsupported container action %q", action)
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(id) {
		return errors.New("invalid container id")
	}
	_, err := s.Run(ctx, "docker", action, id)
	return err
}

func (s Service) UpdateImage(ctx context.Context, image Image) error {
	if !regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`).MatchString(image.Repo) || image.Repo == "" || image.Tag == "<none>" || image.Tag == "" {
		return errors.New("invalid image reference")
	}
	_, err := s.Run(ctx, "docker", "pull", image.Repo+":"+image.Tag)
	return err
}

func (s Service) UpdateCompose(name, compose string) (Stack, error) {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`).MatchString(name) || !strings.Contains(compose, "services:") {
		return Stack{}, errors.New("invalid compose update")
	}
	if err := s.ValidateCompose(context.Background(), compose); err != nil {
		return Stack{}, err
	}
	stackDir := filepath.Join(s.Root, name)
	if filepath.Dir(stackDir) != filepath.Clean(s.Root) {
		return Stack{}, errors.New("invalid stack path")
	}
	composePath := filepath.Join(stackDir, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		return Stack{}, err
	}
	temporary, err := os.CreateTemp(stackDir, ".compose-*.yaml")
	if err != nil {
		return Stack{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(compose); err != nil {
		temporary.Close()
		return Stack{}, err
	}
	if err := temporary.Chmod(0o640); err != nil {
		temporary.Close()
		return Stack{}, err
	}
	if err := temporary.Close(); err != nil {
		return Stack{}, err
	}
	if err := os.Rename(temporaryPath, composePath); err != nil {
		return Stack{}, err
	}
	return parseStack(name, composePath, compose), nil
}

func (s Service) CreateStack(name, compose string) (Stack, error) {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`).MatchString(name) {
		return Stack{}, errors.New("stack name must contain lowercase letters, numbers, and hyphens")
	}
	if !strings.Contains(compose, "services:") {
		return Stack{}, errors.New("compose must define services")
	}
	if err := s.ValidateCompose(context.Background(), compose); err != nil {
		return Stack{}, err
	}
	stackDir := filepath.Join(s.Root, name)
	if filepath.Dir(stackDir) != filepath.Clean(s.Root) {
		return Stack{}, errors.New("invalid stack path")
	}
	if _, err := os.Stat(stackDir); err == nil {
		return Stack{}, errors.New("stack already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Stack{}, err
	}
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		return Stack{}, err
	}
	composePath := filepath.Join(stackDir, "compose.yaml")
	temporary, err := os.CreateTemp(stackDir, ".compose-*.yaml")
	if err != nil {
		return Stack{}, err
	}
	tempPath := temporary.Name()
	defer os.Remove(tempPath)
	if _, err := temporary.WriteString(compose); err != nil {
		temporary.Close()
		return Stack{}, err
	}
	if err := temporary.Chmod(0o640); err != nil {
		temporary.Close()
		return Stack{}, err
	}
	if err := temporary.Close(); err != nil {
		return Stack{}, err
	}
	if err := os.Rename(tempPath, composePath); err != nil {
		return Stack{}, err
	}
	return parseStack(name, composePath, compose), nil
}

func (s Service) ValidateCompose(ctx context.Context, compose string) error {
	if err := validateComposeStructure(compose); err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "lumonas-compose-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "compose.yaml")
	if err := os.WriteFile(path, []byte(compose), 0o600); err != nil {
		return err
	}
	_, err = s.Run(ctx, "docker", "compose", "-f", path, "config", "--quiet")
	if err != nil && !isUnavailable(err) {
		return fmt.Errorf("compose validation failed: %w", err)
	}
	return nil
}

func validateComposeStructure(compose string) error {
	if strings.ContainsRune(compose, '\x00') || strings.Contains(compose, "\t") {
		return errors.New("compose contains unsupported control characters")
	}
	lines := strings.Split(strings.ReplaceAll(compose, "\r\n", "\n"), "\n")
	servicesLine := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == "services:" && len(line) == len(strings.TrimLeft(line, " ")) {
			servicesLine = index
			break
		}
	}
	if servicesLine < 0 {
		return errors.New("compose must define a top-level services mapping")
	}
	for _, line := range lines[servicesLine+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if len(line)-len(strings.TrimLeft(line, " ")) >= 2 && strings.HasSuffix(trimmed, ":") {
			return nil
		}
		if !strings.HasPrefix(line, " ") {
			break
		}
	}
	return errors.New("compose services mapping is empty")
}

func parseStack(name, composePath, content string) Stack {
	stack := Stack{ID: "stack-" + name, Name: name, Category: "Custom", Status: "attention", State: "stopped", ComposeYAML: content, LastDeploy: time.Time{}, Backup: BackupInfo{Strategy: "stop-backup"}}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "image:") {
			stack.Images = append(stack.Images, strings.TrimSpace(strings.TrimPrefix(trimmed, "image:")))
		}
		if strings.Contains(trimmed, "privileged: true") {
			stack.Risks = append(stack.Risks, "privileged")
		}
		if strings.Contains(trimmed, "/var/run/docker.sock") {
			stack.Risks = append(stack.Risks, "docker_socket")
		}
		if strings.Contains(trimmed, "network_mode: host") {
			stack.Risks = append(stack.Risks, "host_network")
		}
		if strings.HasPrefix(trimmed, "- ") && strings.Contains(trimmed, ":") {
			parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")), ":")
			if len(parts) == 2 {
				if host, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
					if container, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
						stack.Ports = append(stack.Ports, Port{Host: host, Container: container})
					}
				}
			}
		}
	}
	_ = composePath
	return stack
}

func normalizeState(value string) string {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "unhealthy") {
		return "unhealthy"
	}
	if strings.HasPrefix(lower, "up") {
		return "running"
	}
	if strings.Contains(lower, "restart") {
		return "restarting"
	}
	if strings.Contains(lower, "exited") {
		return "exited"
	}
	return "created"
}
func parsePorts(value string) []Port {
	result := []Port{}
	for _, part := range strings.Split(value, ", ") {
		match := regexp.MustCompile(`(?:127\.0\.0\.1:)?(\d+)->(\d+)`).FindStringSubmatch(part)
		if len(match) == 3 {
			host, _ := strconv.Atoi(match[1])
			container, _ := strconv.Atoi(match[2])
			result = append(result, Port{Host: host, Container: container})
		}
	}
	return result
}
func parseSize(value string) uint64 {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return 0
	}
	number, _ := strconv.ParseFloat(fields[0], 64)
	multiplier := map[string]float64{"B": 1, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12}
	return uint64(number * multiplier[strings.ToUpper(fields[1])])
}
func parseDays(value string) int {
	fields := strings.Fields(value)
	if len(fields) < 2 {
		return 0
	}
	number, _ := strconv.Atoi(fields[0])
	if strings.HasPrefix(strings.ToLower(fields[1]), "day") {
		return number
	}
	return 0
}
func isUnavailable(err error) bool {
	message := strings.ToLower(err.Error())
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		message += " " + strings.ToLower(string(exitErr.Stderr))
	}
	return errors.Is(err, os.ErrNotExist) || strings.Contains(message, "executable file not found") || strings.Contains(message, "cannot connect") || strings.Contains(message, "failed to connect")
}

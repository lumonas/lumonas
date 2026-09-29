package docker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RehearsalResult struct {
	Stack       string   `json:"stack"`
	Project     string   `json:"project"`
	Services    []string `json:"services"`
	Containers  int      `json:"containers"`
	Healthy     bool     `json:"healthy"`
	NetworkMode string   `json:"networkMode"`
}

// RehearseStack starts a restored Compose project under a unique project name,
// strips host port publication, rewrites bind mounts into the offline restore
// root, and uses a project-private internal network. The project is always
// stopped and its temporary volumes removed before returning.
func (s Service) RehearseStack(ctx context.Context, restoredRoot, stackName string, timeout time.Duration) (result RehearsalResult, returnErr error) {
	if s.Run == nil || !validStackName(stackName) || !filepath.IsAbs(restoredRoot) {
		return result, errors.New("service rehearsal configuration is invalid")
	}
	if timeout <= 0 || timeout > 10*time.Minute {
		timeout = 90 * time.Second
	}
	composePath := filepath.Join(restoredRoot, "srv", "lumonas", "docker", "stacks", stackName, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		return result, err
	}
	configCtx, configCancel := context.WithTimeout(ctx, 15*time.Second)
	configBytes, err := s.Run(configCtx, "docker", "compose", "--project-directory", filepath.Dir(composePath), "-f", composePath, "config", "--format", "json")
	configCancel()
	if err != nil {
		return result, fmt.Errorf("normalize restored Compose project: %w", err)
	}
	sanitized, services, err := sanitizeRehearsalCompose(configBytes, restoredRoot)
	if err != nil {
		return result, err
	}
	if len(services) == 0 {
		return result, errors.New("restored Compose project contains no services")
	}
	temporary, err := os.MkdirTemp("", "lumonas-compose-rehearsal-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(temporary)
	isolatedCompose := filepath.Join(temporary, "compose.yaml")
	if err := os.WriteFile(isolatedCompose, sanitized, 0o600); err != nil {
		return result, err
	}
	var projectBytes [8]byte
	if _, err := rand.Read(projectBytes[:]); err != nil {
		return result, err
	}
	project := "lumonas-drill-" + hex.EncodeToString(projectBytes[:])
	args := []string{"compose", "-p", project, "-f", isolatedCompose}
	cleanup := func() error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := s.Run(cleanupCtx, "docker", append(append([]string(nil), args...), "down", "--volumes", "--remove-orphans")...)
		return err
	}
	defer func() {
		if err := cleanup(); err != nil {
			if returnErr == nil {
				returnErr = fmt.Errorf("stop isolated rehearsal project: %w", err)
			} else {
				returnErr = fmt.Errorf("%v; stop isolated rehearsal project: %w", returnErr, err)
			}
		}
	}()
	upCtx, cancel := context.WithTimeout(ctx, timeout)
	_, err = s.Run(upCtx, "docker", append(append([]string(nil), args...), "up", "--detach", "--no-build", "--pull", "never", "--wait", "--wait-timeout", fmt.Sprintf("%d", int(timeout.Seconds())))...)
	cancel()
	if err != nil {
		return result, fmt.Errorf("start or health-check isolated services: %w", err)
	}
	psCtx, psCancel := context.WithTimeout(ctx, 15*time.Second)
	psBytes, err := s.Run(psCtx, "docker", append(append([]string(nil), args...), "ps", "--format", "json")...)
	psCancel()
	if err != nil {
		return result, fmt.Errorf("inspect isolated service health: %w", err)
	}
	containers, err := parseRehearsalContainers(psBytes)
	if err != nil {
		return result, err
	}
	started := make(map[string]bool, len(containers))
	for _, container := range containers {
		started[container.Service] = true
		if !strings.EqualFold(container.State, "running") || strings.EqualFold(container.Health, "unhealthy") || strings.EqualFold(container.Health, "starting") {
			return result, fmt.Errorf("service %s is not healthy (state=%s health=%s)", container.Service, container.State, container.Health)
		}
	}
	for _, service := range services {
		if !started[service] {
			return result, fmt.Errorf("isolated rehearsal did not start service %q", service)
		}
	}
	return RehearsalResult{Stack: stackName, Project: project, Services: services, Containers: len(containers), Healthy: true, NetworkMode: "private-internal"}, nil
}

func sanitizeRehearsalCompose(raw []byte, restoredRoot string) ([]byte, []string, error) {
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, nil, fmt.Errorf("normalized Compose config is invalid: %w", err)
	}
	services, ok := config["services"].(map[string]any)
	if !ok {
		return nil, nil, errors.New("normalized Compose config has no services")
	}
	serviceNames := make([]string, 0, len(services))
	for name, rawService := range services {
		service, ok := rawService.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("Compose service %q is invalid", name)
		}
		for _, key := range []string{"privileged", "network_mode", "pid", "ipc", "uts", "cgroup", "userns_mode", "runtime", "gpus", "security_opt", "devices", "cap_add", "volumes_from", "configs", "secrets", "extends", "build"} {
			if value, exists := service[key]; exists && value != nil && value != false && value != "" {
				return nil, nil, fmt.Errorf("Compose service %q uses unsupported isolated rehearsal option %q", name, key)
			}
		}
		delete(service, "container_name")
		delete(service, "ports")
		serviceNames = append(serviceNames, name)
		volumes, ok := service["volumes"].([]any)
		if !ok {
			continue
		}
		for _, rawVolume := range volumes {
			volume, ok := rawVolume.(map[string]any)
			if !ok {
				return nil, nil, fmt.Errorf("Compose service %q has an unsupported volume form", name)
			}
			typeName, _ := volume["type"].(string)
			source, _ := volume["source"].(string)
			switch typeName {
			case "bind":
				if !filepath.IsAbs(source) || filepath.Clean(source) != source || source == "/" {
					return nil, nil, fmt.Errorf("Compose service %q has an unsafe bind source", name)
				}
				target := filepath.Join(restoredRoot, strings.TrimPrefix(source, string(filepath.Separator)))
				if !pathWithin(restoredRoot, target) {
					return nil, nil, fmt.Errorf("Compose service %q bind source escapes the restore root", name)
				}
				if err := os.MkdirAll(target, 0o750); err != nil {
					return nil, nil, err
				}
				volume["source"] = target
			case "volume":
				if source == "" || filepath.Base(source) != source {
					return nil, nil, fmt.Errorf("Compose service %q has an unsafe named volume", name)
				}
				volume["source"] = "drill-" + source
			case "tmpfs":
			default:
				return nil, nil, fmt.Errorf("Compose service %q has an unsupported volume type %q", name, typeName)
			}
		}
	}
	volumes, _ := config["volumes"].(map[string]any)
	isolatedVolumes := make(map[string]any, len(volumes))
	for name, rawVolume := range volumes {
		volume, ok := rawVolume.(map[string]any)
		if !ok {
			volume = map[string]any{}
		}
		if external, _ := volume["external"].(bool); external {
			return nil, nil, fmt.Errorf("external volume %q cannot be used in an isolated rehearsal", name)
		}
		isolatedVolumes["drill-"+name] = volume
	}
	if len(isolatedVolumes) > 0 {
		config["volumes"] = isolatedVolumes
	}
	networks, _ := config["networks"].(map[string]any)
	if networks == nil {
		networks = map[string]any{"default": map[string]any{}}
	}
	for name, rawNetwork := range networks {
		network, ok := rawNetwork.(map[string]any)
		if !ok {
			network = map[string]any{}
		}
		if external, _ := network["external"].(bool); external {
			return nil, nil, fmt.Errorf("external network %q cannot be used in an isolated rehearsal", name)
		}
		delete(network, "name")
		delete(network, "driver_opts")
		network["driver"] = "bridge"
		network["internal"] = true
		networks[name] = network
	}
	config["networks"] = networks
	sort.Strings(serviceNames)
	encoded, err := json.Marshal(config)
	return encoded, serviceNames, err
}

func parseRehearsalContainers(raw []byte) ([]struct{ Service, State, Health string }, error) {
	type container struct {
		Service string `json:"Service"`
		State   string `json:"State"`
		Health  string `json:"Health"`
	}
	var array []container
	if json.Unmarshal(raw, &array) == nil {
		result := make([]struct{ Service, State, Health string }, 0, len(array))
		for _, value := range array {
			result = append(result, struct{ Service, State, Health string }{value.Service, value.State, value.Health})
		}
		return result, nil
	}
	result := make([]struct{ Service, State, Health string }, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var value container
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			return nil, fmt.Errorf("isolated service health output is invalid: %w", err)
		}
		result = append(result, struct{ Service, State, Health string }{value.Service, value.State, value.Health})
	}
	return result, nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

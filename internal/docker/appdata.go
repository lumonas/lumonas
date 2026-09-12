package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AppdataSource struct {
	StackName     string
	ServiceName   string
	ContainerPath string
	HostPath      string
	ReadOnly      bool
}

type composeVolume struct {
	Type     string `json:"type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

type composeService struct {
	Volumes []composeVolume `json:"volumes"`
}

type composeConfig struct {
	Services map[string]composeService `json:"services"`
}

// AppdataSources resolves catalog container paths to host paths using the
// normalized Docker Compose model. Named volumes are resolved through Docker's
// volume metadata rather than guessed from the Compose directory.
func (s Service) AppdataSources(ctx context.Context, stack Stack) ([]AppdataSource, error) {
	if stack.Recovery == nil || len(stack.Recovery.AppdataPaths) == 0 {
		return []AppdataSource{}, nil
	}
	if !validStackName(stack.Name) {
		return nil, fmt.Errorf("invalid stack name %q", stack.Name)
	}
	composePath := filepath.Join(s.Root, stack.Name, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		return nil, err
	}
	out, err := s.Run(ctx, "docker", "compose", "-f", composePath, "config", "--format", "json")
	if err != nil {
		return nil, err
	}
	var config composeConfig
	if err := json.Unmarshal(out, &config); err != nil {
		return nil, fmt.Errorf("docker compose config is invalid: %w", err)
	}
	result := make([]AppdataSource, 0, len(stack.Recovery.AppdataPaths))
	for _, containerPath := range stack.Recovery.AppdataPaths {
		volume, serviceName, found, findErr := findComposeVolume(config.Services, containerPath)
		if findErr != nil {
			return nil, findErr
		}
		if !found {
			return nil, fmt.Errorf("appdata path %s is not mounted by stack %s", containerPath, stack.Name)
		}
		hostPath := volume.Source
		if volume.Type == "volume" || (volume.Type == "" && !filepath.IsAbs(hostPath)) {
			if strings.TrimSpace(hostPath) == "" {
				return nil, fmt.Errorf("named appdata volume for %s has no name", containerPath)
			}
			volumeOut, inspectErr := s.Run(ctx, "docker", "volume", "inspect", "--format", "{{json .Mountpoint}}", hostPath)
			if inspectErr != nil {
				return nil, fmt.Errorf("inspect appdata volume %s: %w", hostPath, inspectErr)
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(string(volumeOut))), &hostPath); err != nil {
				return nil, fmt.Errorf("appdata volume %s returned invalid mountpoint", volume.Source)
			}
		}
		if !approvedHostPath(hostPath) {
			return nil, fmt.Errorf("appdata source %s is not an approved host path", hostPath)
		}
		result = append(result, AppdataSource{StackName: stack.Name, ServiceName: serviceName, ContainerPath: filepath.Clean(containerPath), HostPath: filepath.Clean(hostPath), ReadOnly: volume.ReadOnly})
	}
	return result, nil
}

func findComposeVolume(services map[string]composeService, target string) (composeVolume, string, bool, error) {
	target = filepath.Clean(target)
	var match composeVolume
	matchedService := ""
	for serviceName, service := range services {
		for _, volume := range service.Volumes {
			if filepath.Clean(volume.Target) == target {
				if matchedService != "" {
					return composeVolume{}, "", false, fmt.Errorf("appdata path %s is mounted by multiple services", target)
				}
				match = volume
				matchedService = serviceName
			}
		}
	}
	if matchedService == "" {
		return composeVolume{}, "", false, nil
	}
	return match, matchedService, true, nil
}

func approvedHostPath(value string) bool {
	if value == "" || !filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return false
	}
	clean := filepath.Clean(value)
	if clean == "/" || strings.Contains(clean, "..") {
		return false
	}
	for _, prefix := range []string{"/srv/", "/mnt/", "/opt/", "/var/lib/"} {
		if strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

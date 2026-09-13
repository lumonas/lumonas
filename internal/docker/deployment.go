package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DeployStack brings a freshly staged stack up for the first time and probes
// its health for a bounded window. Unlike UpdateStack there is no previous
// release to fall back to, so a failed health gate tears the partial
// deployment down again and leaves the stack staged but stopped.
func (s Service) DeployStack(ctx context.Context, stack Stack, probe func(context.Context) error, options UpdateOptions) (UpdateResult, error) {
	if probe == nil {
		return UpdateResult{}, errors.New("health probe is required")
	}
	if !validStackName(stack.Name) {
		return UpdateResult{}, errors.New("invalid stack name")
	}
	composePath := filepath.Join(s.Root, stack.Name, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		return UpdateResult{}, err
	}
	options = options.withDefaults()
	if _, err := s.Run(ctx, "docker", "compose", "-f", composePath, "up", "-d", "--remove-orphans"); err != nil {
		return UpdateResult{}, fmt.Errorf("stack deployment failed: %w", err)
	}
	if err := s.probeUntilHealthy(ctx, probe, options); err != nil {
		downErr := s.DownStack(ctx, stack.Name)
		result := UpdateResult{RolledBack: downErr == nil, Reason: err.Error()}
		if downErr != nil {
			result.Reason = err.Error() + "; cleanup failed: " + downErr.Error()
		}
		return result, fmt.Errorf("stack deployment failed its health gate: %w", err)
	}
	return UpdateResult{Updated: true}, nil
}

// DownStack stops and removes a stack's containers, networks, and orphans
// without touching its staged compose file or volumes.
func (s Service) DownStack(ctx context.Context, name string) error {
	if !validStackName(name) {
		return errors.New("invalid stack name")
	}
	composePath := filepath.Join(s.Root, name, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		return err
	}
	_, err := s.Run(ctx, "docker", "compose", "-f", composePath, "down", "--remove-orphans")
	return err
}

// HasStack reports whether a stack directory with a compose file exists.
func (s Service) HasStack(name string) bool {
	if !validStackName(name) {
		return false
	}
	_, err := os.Stat(filepath.Join(s.Root, name, "compose.yaml"))
	return err == nil
}

// RestoreCompose writes previously deployed compose content back to a stack
// without revalidation. It is the rollback path for interrupted or failed
// deployments, so the exact bytes that last ran successfully are restored
// even if validation rules have changed since they were accepted.
func (s Service) RestoreCompose(name, compose string) error {
	if !validStackName(name) || !strings.Contains(compose, "services:") {
		return errors.New("invalid compose restore")
	}
	stackDir := filepath.Join(s.Root, name)
	if filepath.Dir(stackDir) != filepath.Clean(s.Root) {
		return errors.New("invalid stack path")
	}
	composePath := filepath.Join(stackDir, "compose.yaml")
	temporary, err := os.CreateTemp(stackDir, ".compose-*.yaml")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(compose); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o640); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, composePath)
}

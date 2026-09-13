package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UpdateOptions bound the post-update health gate. Zero fields fall back to
// production defaults; tests use short windows.
type UpdateOptions struct {
	GracePeriod time.Duration // wait after `up -d` before the first probe
	Interval    time.Duration // delay between probes
	Timeout     time.Duration // total budget for the health gate
}

func (o UpdateOptions) withDefaults() UpdateOptions {
	if o.GracePeriod <= 0 {
		o.GracePeriod = 10 * time.Second
	}
	if o.Interval <= 0 {
		o.Interval = 10 * time.Second
	}
	if o.Timeout <= 0 {
		o.Timeout = 120 * time.Second
	}
	return o
}

// UpdateResult reports the outcome of a health-gated stack update.
type UpdateResult struct {
	Updated    bool     `json:"updated"`
	RolledBack bool     `json:"rolledBack"`
	Reason     string   `json:"reason,omitempty"`
	Images     []string `json:"images,omitempty"` // repo:tag values that participated
}

// UpdateFailure marks an error raised by the health-gated update flow and
// carries the outcome so callers can distinguish staging failures (nothing
// was touched) from gate failures that may have rolled the stack back.
type UpdateFailure struct {
	Result UpdateResult
	err    error
}

func (f *UpdateFailure) Error() string { return f.err.Error() }

func (f *UpdateFailure) Unwrap() error { return f.err }

// ImageSnapshot records the image ID a tag pointed at before an update so the
// tag can be re-pointed back if the update fails its health gate or the daemon
// restarts before the transaction reaches a terminal state.
type ImageSnapshot struct {
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	ID         string `json:"ID"`
}

func (i ImageSnapshot) ref() string {
	return i.Repository + ":" + i.Tag
}

// UpdateStack pulls newer images for a stack, recreates its containers, and
// probes stack health for a bounded window. When the probe keeps failing, the
// previous image IDs are restored with `docker tag` and the stack is
// recreated from them, so a bad upstream release cannot strand the stack.
// The probe callback is supplied by the caller so health semantics (running
// state, restart counters, app-level checks) stay with the caller.
func (s Service) UpdateStack(ctx context.Context, stack Stack, probe func(context.Context) error, options UpdateOptions) (UpdateResult, error) {
	before, err := s.SnapshotStackImages(ctx, stack)
	if err != nil {
		return UpdateResult{}, &UpdateFailure{Result: UpdateResult{Reason: err.Error()}, err: err}
	}
	return s.updateStackWithSnapshot(ctx, stack, probe, options, before)
}

// UpdateStackWithSnapshot is the crash-safe variant used by the daemon
// transaction layer. The snapshot is captured before Compose staging, so an
// update that also changes image references can still roll back to the old
// stack after the new file has been written.
func (s Service) UpdateStackWithSnapshot(ctx context.Context, stack Stack, probe func(context.Context) error, options UpdateOptions, before []ImageSnapshot) (UpdateResult, error) {
	return s.updateStackWithSnapshot(ctx, stack, probe, options, before)
}

func (s Service) updateStackWithSnapshot(ctx context.Context, stack Stack, probe func(context.Context) error, options UpdateOptions, before []ImageSnapshot) (UpdateResult, error) {
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
	refs := make([]string, 0, len(before))
	for _, image := range before {
		refs = append(refs, image.ref())
	}

	if _, err := s.Run(ctx, "docker", "compose", "-f", composePath, "pull"); err != nil {
		return UpdateResult{}, &UpdateFailure{Result: UpdateResult{Reason: "image pull failed: " + err.Error()}, err: fmt.Errorf("image pull failed: %w", err)}
	}
	if _, err := s.Run(ctx, "docker", "compose", "-f", composePath, "up", "-d", "--remove-orphans"); err != nil {
		return UpdateResult{}, &UpdateFailure{Result: UpdateResult{Reason: "stack recreation failed: " + err.Error()}, err: fmt.Errorf("stack recreation failed: %w", err)}
	}

	if err := s.probeUntilHealthy(ctx, probe, options); err != nil {
		rollbackErr := s.rollbackStack(ctx, composePath, before)
		result := UpdateResult{RolledBack: rollbackErr == nil, Reason: err.Error(), Images: refs}
		if rollbackErr != nil {
			result.Reason = err.Error() + "; rollback failed: " + rollbackErr.Error()
		}
		return result, &UpdateFailure{Result: result, err: fmt.Errorf("stack update failed its health gate: %w", err)}
	}
	return UpdateResult{Updated: true, Images: refs}, nil
}

// probeUntilHealthy waits out the grace period, then polls the probe until it
// passes or the timeout budget is spent.
func (s Service) probeUntilHealthy(ctx context.Context, probe func(context.Context) error, options UpdateOptions) error {
	deadline := time.Now().Add(options.Timeout)
	select {
	case <-time.After(options.GracePeriod):
	case <-ctx.Done():
		return ctx.Err()
	}
	var lastErr error
	for {
		lastErr = probe(ctx)
		if lastErr == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			break
		}
		select {
		case <-time.After(options.Interval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return lastErr
}

// rollbackStack re-points each previously current tag at its old image ID and
// recreates the stack from those images.
// SnapshotStackImages records the current image IDs for a stack before a
// mutating update. Callers can persist the returned values for crash recovery.
func (s Service) SnapshotStackImages(ctx context.Context, stack Stack) ([]ImageSnapshot, error) {
	if !validStackName(stack.Name) {
		return nil, errors.New("invalid stack name")
	}
	composePath := filepath.Join(s.Root, stack.Name, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		return nil, err
	}
	return s.composeImages(ctx, composePath)
}

// RestoreStackImages re-points the recorded tags to their previous image IDs
// and recreates the stack from the restored references.
func (s Service) RestoreStackImages(ctx context.Context, stackName string, before []ImageSnapshot) error {
	if !validStackName(stackName) {
		return errors.New("invalid stack name")
	}
	composePath := filepath.Join(s.Root, stackName, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		return err
	}
	return s.rollbackStack(ctx, composePath, before)
}

func (s Service) rollbackStack(ctx context.Context, composePath string, before []ImageSnapshot) error {
	for _, image := range before {
		if image.Repository == "" || image.Tag == "" || image.ID == "" {
			continue
		}
		if _, err := s.Run(ctx, "docker", "tag", image.ID, image.ref()); err != nil {
			return fmt.Errorf("restore %s: %w", image.ref(), err)
		}
	}
	_, err := s.Run(ctx, "docker", "compose", "-f", composePath, "up", "-d", "--force-recreate")
	return err
}

func (s Service) composeImages(ctx context.Context, composePath string) ([]ImageSnapshot, error) {
	out, err := s.Run(ctx, "docker", "compose", "-f", composePath, "images", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("compose images failed: %w", err)
	}
	snapshot := make([]ImageSnapshot, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var image ImageSnapshot
		if json.Unmarshal([]byte(line), &image) != nil {
			continue
		}
		snapshot = append(snapshot, image)
	}
	return snapshot, nil
}

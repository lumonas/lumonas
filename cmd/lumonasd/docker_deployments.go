package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/model"
)

// deploymentOptions overrides the health-gate windows used by stack
// deployments. Zero keeps the production defaults; tests shorten them.
type deploymentOptions struct {
	GracePeriod time.Duration
	Interval    time.Duration
	Timeout     time.Duration
}

func (o deploymentOptions) runtime() dockerruntime.UpdateOptions {
	return dockerruntime.UpdateOptions{GracePeriod: o.GracePeriod, Interval: o.Interval, Timeout: o.Timeout}
}

// runStackInstallDeployment persists an install transaction for a freshly
// staged stack, brings it up behind the health gate, and records the outcome.
// A failed gate tears the partial deployment down; the stack stays staged but
// stopped so the operator can fix configuration and retry.
func (s *apiServer) runStackInstallDeployment(ctx context.Context, stack dockerruntime.Stack, probe func(context.Context) error) (dockerruntime.Stack, error) {
	composeAfter, err := s.currentCompose(stack.Name)
	if err != nil {
		return stack, err
	}
	deployment := model.DockerDeployment{ID: newID("deploy"), StackName: stack.Name, Kind: "install", ComposeAfter: composeAfter}
	if err := s.store.CreateDockerDeployment(deployment); err != nil {
		return stack, err
	}
	result, deployErr := s.dockerService.DeployStack(ctx, stack, probe, s.deploymentOptions.runtime())
	if deployErr != nil {
		state := "failed"
		if result.RolledBack {
			state = "rolled_back"
		}
		reason := result.Reason
		if reason == "" {
			reason = deployErr.Error()
		}
		if updateErr := s.store.UpdateDockerDeployment(deployment.ID, state, reason); updateErr != nil {
			if s.log != nil {
				s.log.Warn("deployment state could not be recorded", "deployment", deployment.ID, "error", updateErr)
			}
		}
		s.publish("docker.deployment.failed", "warning", &model.ResourceRef{Type: "stack", ID: stack.ID}, map[string]any{"stackId": stack.ID, "kind": "install", "reason": reason, "rolledBack": result.RolledBack})
		return stack, deployErr
	}
	if err := s.store.UpdateDockerDeployment(deployment.ID, "committed", ""); err != nil {
		if s.log != nil {
			s.log.Warn("deployment state could not be recorded", "deployment", deployment.ID, "error", err)
		}
	}
	s.publish("docker.deployment.committed", "info", &model.ResourceRef{Type: "stack", ID: stack.ID}, map[string]any{"stackId": stack.ID, "kind": "install"})
	return stack, nil
}

// runStackUpdateDeployment persists an update transaction around the staged
// compose change and the health-gated image update. When the gate fails, the
// previously deployed compose content is restored so the stack can be
// recreated exactly as it ran before, not merely re-pointed at old images.
func (s *apiServer) runStackUpdateDeployment(ctx context.Context, stack dockerruntime.Stack, composeYAML string, probe func(context.Context) error) (dockerruntime.Stack, error) {
	composeBefore, beforeErr := s.currentCompose(stack.Name)
	composeChanged := composeYAML != "" && (beforeErr != nil || composeYAML != composeBefore)
	var before *string
	if beforeErr == nil {
		value := composeBefore
		before = &value
	}
	composeAfter := composeYAML
	if composeAfter == "" && beforeErr == nil {
		composeAfter = composeBefore
	}
	imageBefore, imageErr := s.dockerService.SnapshotStackImages(ctx, stack)
	if imageErr != nil {
		return stack, imageErr
	}
	imageBeforeJSON, imageErr := json.Marshal(imageBefore)
	if imageErr != nil {
		return stack, imageErr
	}
	deployment := model.DockerDeployment{ID: newID("deploy"), StackName: stack.Name, Kind: "update", ComposeBefore: before, ComposeAfter: composeAfter, ImageBeforeJSON: string(imageBeforeJSON)}
	if err := s.store.CreateDockerDeployment(deployment); err != nil {
		return stack, err
	}
	if composeChanged {
		updated, updateErr := s.dockerService.UpdateCompose(stack.Name, composeYAML)
		if updateErr != nil {
			if stateErr := s.store.UpdateDockerDeployment(deployment.ID, "failed", updateErr.Error()); stateErr != nil {
				if s.log != nil {
					s.log.Warn("deployment state could not be recorded", "deployment", deployment.ID, "error", stateErr)
				}
			}
			return stack, updateErr
		}
		stack = updated
		s.advanceGeneration("docker.stack.compose.update")
	}
	result, updateErr := s.dockerService.UpdateStackWithSnapshot(ctx, stack, probe, s.deploymentOptions.runtime(), imageBefore)
	if updateErr != nil {
		state := "failed"
		reason := result.Reason
		if reason == "" {
			reason = updateErr.Error()
		}
		if result.RolledBack {
			state = "rolled_back"
			if composeChanged && before != nil {
				if restoreErr := s.dockerService.RestoreCompose(stack.Name, *before); restoreErr != nil {
					reason += "; compose restore failed: " + restoreErr.Error()
					state = "failed"
				}
			}
		}
		if stateErr := s.store.UpdateDockerDeployment(deployment.ID, state, reason); stateErr != nil {
			if s.log != nil {
				s.log.Warn("deployment state could not be recorded", "deployment", deployment.ID, "error", stateErr)
			}
		}
		s.publish("docker.deployment.failed", "warning", &model.ResourceRef{Type: "stack", ID: stack.ID}, map[string]any{"stackId": stack.ID, "kind": "update", "reason": reason, "rolledBack": result.RolledBack})
		return stack, updateErr
	}
	if err := s.store.UpdateDockerDeployment(deployment.ID, "committed", ""); err != nil {
		if s.log != nil {
			s.log.Warn("deployment state could not be recorded", "deployment", deployment.ID, "error", err)
		}
	}
	s.publish("docker.deployment.committed", "info", &model.ResourceRef{Type: "stack", ID: stack.ID}, map[string]any{"stackId": stack.ID, "kind": "update"})
	return stack, nil
}

// reconcilePendingDeployments closes deployment transactions the daemon left
// open when it stopped mid-flight. Interrupted installs are torn down;
// interrupted updates have their previous compose restored. A transaction is
// marked rolled_back only after cleanup succeeds; failed cleanup stays visible
// as failed so operators do not mistake an unsafe state for a recovery.
func (s *apiServer) reconcilePendingDeployments(ctx context.Context) {
	deployments, err := s.store.PendingDockerDeployments()
	if err != nil {
		if s.log != nil {
			s.log.Warn("pending deployments could not be listed", "error", err)
		}
		return
	}
	for _, deployment := range deployments {
		reason := "daemon restarted mid-deployment; rolled back at boot"
		state := "rolled_back"
		switch deployment.Kind {
		case "install":
			if err := s.dockerService.DownStack(ctx, deployment.StackName); err != nil {
				if s.log != nil {
					s.log.Warn("interrupted install could not be torn down", "stack", deployment.StackName, "error", err)
				}
				reason += "; teardown failed: " + err.Error()
				state = "failed"
			}
		case "update", "rollback":
			if deployment.ComposeBefore == nil || *deployment.ComposeBefore == "" {
				reason += "; previous compose snapshot is missing"
				state = "failed"
			} else if err := s.dockerService.RestoreCompose(deployment.StackName, *deployment.ComposeBefore); err != nil {
				if s.log != nil {
					s.log.Warn("interrupted update could not be restored", "stack", deployment.StackName, "error", err)
				}
				reason += "; compose restore failed: " + err.Error()
				state = "failed"
			} else {
				var images []dockerruntime.ImageSnapshot
				if err := json.Unmarshal([]byte(deployment.ImageBeforeJSON), &images); err != nil {
					reason += "; image snapshot is invalid"
					state = "failed"
				} else if err := s.dockerService.RestoreStackImages(ctx, deployment.StackName, images); err != nil {
					reason += "; image restore failed: " + err.Error()
					state = "failed"
				}
			}
		default:
			reason += "; unknown deployment kind " + deployment.Kind
			state = "failed"
		}
		if err := s.store.UpdateDockerDeployment(deployment.ID, state, reason); err != nil {
			if s.log != nil {
				s.log.Warn("pending deployment could not be closed", "deployment", deployment.ID, "error", err)
			}
			continue
		}
		eventType := "docker.deployment.rolled_back"
		if state == "failed" {
			eventType = "docker.deployment.recovery_failed"
		}
		s.publish(eventType, "warning", &model.ResourceRef{Type: "stack", ID: "stack-" + deployment.StackName}, map[string]any{"stackId": "stack-" + deployment.StackName, "kind": deployment.Kind, "reason": reason})
		if s.log != nil {
			s.log.Warn("interrupted deployment reconciled", "deployment", deployment.ID, "stack", deployment.StackName, "kind", deployment.Kind, "state", state)
		}
	}
}

// dockerDeploymentsHandler lists recent stack deployments, optionally scoped
// with ?stack=<name>, newest first.
func (s *apiServer) dockerDeploymentsHandler(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
			return
		}
		limit = value
	}
	deployments, err := s.store.DockerDeployments(limit, r.URL.Query().Get("stack"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, deployments)
}

func (s *apiServer) currentCompose(stackName string) (string, error) {
	if !s.dockerService.HasStack(stackName) {
		return "", errors.New("stack compose file not found")
	}
	data, err := os.ReadFile(filepath.Join(s.dockerService.Root, stackName, "compose.yaml"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

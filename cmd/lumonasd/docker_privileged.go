package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/trace"
)

func (s *apiServer) dockerEngineRequest(ctx context.Context, path string) ([]byte, error) {
	result, err := s.executePrivileged(ctx, privileged.Request{
		Operation:      "docker.read",
		OperationID:    newID("docker-read"),
		CorrelationID:  trace.CorrelationID(ctx),
		PlanHash:       "docker-read",
		RequestedState: map[string]any{"path": path},
		ExpiresAt:      time.Now().UTC().Add(30 * time.Second),
		Confirmed:      true,
	})
	if err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, errors.New(result.Error)
	}
	body, err := json.Marshal(result.Data)
	if err != nil {
		return nil, fmt.Errorf("decode Docker Engine broker response: %w", err)
	}
	return body, nil
}

func (s *apiServer) dockerCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name != "docker" {
		return nil, fmt.Errorf("Docker command %q is not allow-listed", name)
	}
	request := privileged.Request{
		Operation:      "docker.command",
		OperationID:    newID("docker"),
		CorrelationID:  trace.CorrelationID(ctx),
		PlanHash:       "docker-command",
		RequestedState: map[string]any{"args": append([]string(nil), args...)},
		ExpiresAt:      time.Now().UTC().Add(5 * time.Minute),
		Confirmed:      true,
	}
	result, err := s.executePrivileged(ctx, request)
	if err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, errors.New(result.Error)
	}
	output, ok := result.Data.(string)
	if !ok {
		return nil, errors.New("Docker command broker returned an invalid output")
	}
	return []byte(output), nil
}

func (s *apiServer) dockerServiceWithBroker(root string) dockerruntime.Service {
	return dockerruntime.NewWithEngineRequester(root, s.dockerCommandRunner, s.dockerEngineRequest)
}

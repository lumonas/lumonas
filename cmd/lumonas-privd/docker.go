package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
)

const maxDockerBrokerResponse = 1 << 20

var readDockerEngine = dockerruntime.ReadOnlyEngineRequest

func executeDockerRead(req request) response {
	path := requestedString(req.RequestedState, "path")
	if err := dockerruntime.ValidateReadOnlyEnginePath(path); err != nil {
		return response{Error: err.Error()}
	}
	body, err := readDockerEngine(context.Background(), os.Getenv("LUMONAS_DOCKER_SOCKET"), path)
	if err != nil {
		return response{Error: "Docker Engine read failed"}
	}
	if err := validateDockerReadResponse(body); err != nil {
		return response{Error: err.Error()}
	}
	return response{OK: true, Data: jsonRawMessage(body)}
}

func validateDockerReadResponse(body []byte) error {
	if len(body) > maxDockerBrokerResponse {
		return errors.New("Docker Engine response exceeded privileged IPC size limit")
	}
	if !json.Valid(body) {
		return errors.New("Docker Engine response was not valid JSON")
	}
	return nil
}

func executeDockerCommand(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	args := requestedStrings(req.RequestedState, "args")
	if err := validateDockerCommand(args); err != nil {
		return response{Error: err.Error()}
	}
	output, err := run("docker", args...)
	if err != nil {
		return response{Error: "Docker command failed"}
	}
	return response{OK: true, Data: string(output)}
}

// jsonRawMessage lets the broker preserve the Engine's object/array payload
// while response.Data remains an intentionally narrow JSON value.
type jsonRawMessage []byte

func (value jsonRawMessage) MarshalJSON() ([]byte, error) {
	if len(value) == 0 {
		return []byte("null"), nil
	}
	return value, nil
}

func validateDockerCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("Docker command arguments are required")
	}
	switch args[0] {
	case "compose":
		return validateDockerComposeCommand(args[1:])
	case "start", "stop", "restart":
		if len(args) != 2 || !privilegedDockerIdentifierPattern.MatchString(args[1]) {
			return errors.New("Docker container action is not allow-listed")
		}
	case "pull":
		if len(args) != 2 || !dockerImageReferencePattern.MatchString(args[1]) {
			return errors.New("Docker pull reference is not allow-listed")
		}
	case "volume":
		if len(args) != 5 || args[1] != "inspect" || args[2] != "--format" || args[3] != "{{json .Mountpoint}}" || !privilegedDockerIdentifierPattern.MatchString(args[4]) {
			return errors.New("Docker volume inspection is not allow-listed")
		}
	case "image":
		if len(args) != 5 || args[1] != "inspect" || args[2] != "--format" || args[3] != "{{index .RepoDigests 0}}" || !dockerImageReferencePattern.MatchString(args[4]) {
			return errors.New("Docker image inspection is not allow-listed")
		}
	case "manifest":
		if len(args) != 4 || args[1] != "inspect" || args[2] != "--verbose" || !dockerImageReferencePattern.MatchString(args[3]) {
			return errors.New("Docker manifest inspection is not allow-listed")
		}
	case "load":
		if len(args) != 3 || args[1] != "--input" || !managedDockerPath(args[2], os.Getenv("LUMONAS_DOCKER_IMPORT_DIR"), true) {
			return errors.New("Docker image import path is not allow-listed")
		}
	case "tag":
		if len(args) != 3 || !dockerImageIDPattern.MatchString(args[1]) || !dockerImageReferencePattern.MatchString(args[2]) {
			return errors.New("Docker tag arguments are not allow-listed")
		}
	default:
		return fmt.Errorf("Docker command %q is not allow-listed", args[0])
	}
	return nil
}

func validateDockerComposeCommand(args []string) error {
	if len(args) < 3 || args[0] != "-f" || !managedComposePath(args[1]) {
		return errors.New("Docker Compose path is not allow-listed")
	}
	remaining := args[2:]
	switch remaining[0] {
	case "config":
		if len(remaining) == 2 && remaining[1] == "--quiet" {
			return nil
		}
		if len(remaining) != 3 || remaining[1] != "--format" || remaining[2] != "json" {
			return errors.New("Docker Compose config arguments are not allow-listed")
		}
	case "images":
		if len(remaining) != 3 || remaining[1] != "--format" || remaining[2] != "json" {
			return errors.New("Docker Compose inspection arguments are not allow-listed")
		}
	case "pull":
		if len(remaining) != 1 {
			return errors.New("Docker Compose pull arguments are not allow-listed")
		}
	case "up":
		if !sameStrings(remaining[1:], "-d", "--remove-orphans") && !sameStrings(remaining[1:], "-d", "--force-recreate") {
			return errors.New("Docker Compose up arguments are not allow-listed")
		}
	case "down":
		if !sameStrings(remaining[1:], "--remove-orphans") {
			return errors.New("Docker Compose down arguments are not allow-listed")
		}
	default:
		return fmt.Errorf("Docker Compose action %q is not allow-listed", remaining[0])
	}
	return nil
}

func sameStrings(actual []string, expected ...string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}

func managedComposePath(path string) bool {
	root := os.Getenv("LUMONAS_STACK_ROOT")
	if root == "" {
		root = "/srv/lumonas/docker/stacks"
	}
	cleanRoot, cleanPath := filepath.Clean(root), filepath.Clean(path)
	relative, err := filepath.Rel(cleanRoot, cleanPath)
	return err == nil && relative != "." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".." && filepath.Base(cleanPath) == "compose.yaml"
}

func managedDockerPath(path, configuredRoot string, requireRegularFile bool) bool {
	if configuredRoot == "" {
		configuredRoot = "/var/lib/lumonas/imports"
	}
	cleanRoot, cleanPath := filepath.Clean(configuredRoot), filepath.Clean(path)
	relative, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return false
	}
	if requireRegularFile {
		info, statErr := os.Stat(cleanPath)
		return statErr == nil && info.Mode().IsRegular()
	}
	return true
}

var dockerImageReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/@:-]{0,255}$`)
var dockerImageIDPattern = regexp.MustCompile(`^(?:sha256:)?[a-fA-F0-9]{12,128}$`)
var privilegedDockerIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

package docker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// EngineRequester is the narrow read-only transport used by the daemon when
// Docker's root-owned socket is hidden behind lumonas-privd.
type EngineRequester func(context.Context, string) ([]byte, error)

// NewWithEngineRequester keeps the Docker socket out of the lumonas service
// user. Read-only Engine calls use requester; Docker CLI mutations use run and
// are expected to be brokered separately by the caller.
func NewWithEngineRequester(root string, run Runner, requester EngineRequester) Service {
	if run == nil {
		run = commandRunner
	}
	if requester == nil {
		return Service{Root: root, Run: run, engine: newEngineClient(dockerSocket())}
	}
	return Service{Root: root, Run: run, engine: newEngineClientWithRequester(requester)}
}

// ReadOnlyEngineRequest performs one allow-listed GET against the local Docker
// Engine. It is intended for the root-owned privileged broker, never for an
// arbitrary API caller.
func ReadOnlyEngineRequest(ctx context.Context, socket, path string) ([]byte, error) {
	if err := ValidateReadOnlyEnginePath(path); err != nil {
		return nil, err
	}
	if strings.TrimSpace(socket) == "" {
		socket = defaultDockerSocket
	}
	return newEngineClient(socket).request(ctx, http.MethodGet, path)
}

// ValidateReadOnlyEnginePath prevents the broker from becoming a generic
// Docker socket tunnel. Every production inventory endpoint is enumerated and
// container identifiers/tail values are bounded before the request reaches
// the root-owned socket.
func ValidateReadOnlyEnginePath(path string) error {
	parsed, err := url.ParseRequestURI(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path == "" {
		return errors.New("Docker Engine path must be a relative API path")
	}
	query := parsed.Query()
	switch parsed.Path {
	case "/version", "/images/json", "/volumes", "/system/df":
		if len(query) != 0 {
			return errors.New("Docker Engine endpoint does not accept query parameters")
		}
		return nil
	case "/containers/json":
		if !singleQueryValue(query, "all", "true") {
			return errors.New("Docker container inventory requires all=true")
		}
		return nil
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "containers" || !dockerIdentifierPattern.MatchString(parts[1]) {
		return errors.New("Docker Engine container path is not allow-listed")
	}
	switch parts[2] {
	case "json":
		if len(query) != 0 {
			return errors.New("Docker inspect endpoint does not accept query parameters")
		}
	case "stats":
		if !singleQueryValue(query, "stream", "false") {
			return errors.New("Docker stats endpoint requires stream=false")
		}
	case "logs":
		if !singleQueryValue(query, "stdout", "1") || !singleQueryValue(query, "stderr", "1") || !singleQueryValue(query, "timestamps", "1") {
			return errors.New("Docker logs endpoint requires bounded stdout/stderr timestamps")
		}
		tail := query.Get("tail")
		value, parseErr := strconv.Atoi(tail)
		if parseErr != nil || value < 0 || value > 1000 || len(query) != 4 {
			return errors.New("Docker logs tail must be between 0 and 1000")
		}
	default:
		return fmt.Errorf("Docker Engine container endpoint %q is not allow-listed", parts[2])
	}
	return nil
}

var dockerIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

func singleQueryValue(values url.Values, key, expected string) bool {
	items, ok := values[key]
	return ok && len(items) == 1 && items[0] == expected
}

// Package runner centralizes bounded execution of host integration commands.
package runner

import (
	"context"
	"io"
	"os/exec"
	"time"
)

const DefaultTimeout = 30 * time.Second

// Context derives a bounded context while preserving an earlier parent
// deadline. Every integration command should use this before invoking an OS
// binary so a broken daemon cannot hold an API or privileged worker forever.
func Context(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, DefaultTimeout)
}

func Output(name string, args ...string) ([]byte, error) {
	return OutputContext(context.Background(), name, args...)
}

func OutputContext(parent context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := Context(parent)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

func CombinedOutput(name string, args ...string) ([]byte, error) {
	return CombinedOutputContext(context.Background(), name, args...)
}

func CombinedOutputContext(parent context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := Context(parent)
	defer cancel()
	return combinedOutputContext(ctx, nil, name, args...)
}

// CombinedOutputContextWithStdin is the stdin-aware counterpart to
// CombinedOutputContext. It uses the same bounded process-group execution so
// commands that spawn descendants cannot keep pipes open after cancellation.
func CombinedOutputContextWithStdin(parent context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	ctx, cancel := Context(parent)
	defer cancel()
	return combinedOutputContext(ctx, stdin, name, args...)
}

func combinedOutputContext(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	configureProcessGroup(command)
	if stdin != nil {
		command.Stdin = stdin
	}

	if err := command.Start(); err != nil {
		return nil, err
	}

	output, done := collectCombinedOutput(command)
	select {
	case err := <-done:
		return output(), err
	case <-ctx.Done():
		killProcessGroup(command)
		<-done
		return output(), ctx.Err()
	}
}

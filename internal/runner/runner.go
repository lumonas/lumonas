// Package runner centralizes bounded execution of host integration commands.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

const DefaultTimeout = 30 * time.Second

// MaxOutputBytes prevents a broken or hostile host utility from turning a
// bounded command into an unbounded memory allocation. Callers receive
// ErrOutputLimit rather than silently consuming partial command output.
const MaxOutputBytes = 1 << 20

var ErrOutputLimit = errors.New("command output exceeded limit")

// ConfigureProcessGroup makes an interactive command and its descendants
// share a killable process group. It is exported for privileged operations
// that must keep a stdin pipe open while still enforcing a deadline.
func ConfigureProcessGroup(command *exec.Cmd) {
	configureProcessGroup(command)
}

// KillProcessGroup terminates an interactive command and all descendants.
func KillProcessGroup(command *exec.Cmd) {
	killProcessGroup(command)
}

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
	return OutputContextLimit(parent, MaxOutputBytes, name, args...)
}

// OutputContextLimit executes a command while retaining at most maxBytes of
// stdout. It is intended for callers that already enforce a domain-specific
// hard limit, such as a recovery-bundle entry limit.
func OutputContextLimit(parent context.Context, maxBytes int64, name string, args ...string) ([]byte, error) {
	return OutputContextLimitTimeout(parent, DefaultTimeout, maxBytes, name, args...)
}

// OutputContextLimitTimeout is like OutputContextLimit with an explicit
// maximum runtime. A parent deadline, when earlier, remains authoritative.
func OutputContextLimitTimeout(parent context.Context, timeout time.Duration, maxBytes int64, name string, args ...string) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("command output limit must be positive")
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	command := exec.Command(name, args...)
	configureProcessGroup(command)
	stdout := boundedBuffer{limit: maxBytes}
	stderr := boundedBuffer{limit: MaxOutputBytes}
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if stdout.Exceeded() || stderr.Exceeded() {
			return stdout.Bytes(), fmt.Errorf("%w: %s", ErrOutputLimit, name)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitErr.Stderr = stderr.Bytes()
		}
		return stdout.Bytes(), err
	case <-ctx.Done():
		killProcessGroup(command)
		<-done
		return stdout.Bytes(), ctx.Err()
	}
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
	var output boundedBuffer
	command.Stdout = &output
	command.Stderr = &output

	if err := command.Start(); err != nil {
		return nil, err
	}

	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if output.Exceeded() {
			return output.Bytes(), fmt.Errorf("%w: %s", ErrOutputLimit, name)
		}
		return output.Bytes(), err
	case <-ctx.Done():
		killProcessGroup(command)
		<-done
		return output.Bytes(), ctx.Err()
	}
}

type boundedBuffer struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	exceeded bool
	limit    int64
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	originalLength := len(p)
	limit := b.limit
	if limit <= 0 {
		limit = MaxOutputBytes
	}
	remaining := limit - int64(b.buf.Len())
	if remaining <= 0 {
		b.exceeded = true
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		b.exceeded = true
		p = p[:remaining]
	}
	_, _ = b.buf.Write(p)
	return originalLength, nil
}

func (b *boundedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes())
}

func (b *boundedBuffer) Exceeded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exceeded
}

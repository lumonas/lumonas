#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

require_line() {
	file=$1
	pattern=$2
	if ! grep -F "$pattern" "$file" >/dev/null 2>&1; then
		echo "missing '$pattern' in $file" >&2
		exit 1
	fi
}

# The shared runner is the only ordinary production owner of exec.Command.
# The privileged broker has one explicit exception for the long-lived,
# confirmation-backed NetworkManager checkpoint; keep that exception visible
# and verify that it still has a process-group deadline.
matches=$(grep -R -n --include='*.go' --exclude='*_test.go' -F 'exec.Command(' "$ROOT/internal" "$ROOT/cmd" 2>/dev/null || true)
unexpected=$(printf '%s\n' "$matches" | grep -v '/internal/runner/runner.go:' | grep -v '/cmd/lumonas-privd/main.go:' || true)
if [ -n "$unexpected" ]; then
	printf '%s\n' "$unexpected"
	echo "production command execution bypasses the shared runner" >&2
	exit 1
fi

require_line "$ROOT/internal/runner/runner.go" 'const DefaultTimeout = 30 * time.Second'
require_line "$ROOT/internal/runner/runner.go" 'const MaxOutputBytes = 1 << 20'
require_line "$ROOT/internal/runner/runner.go" 'ErrOutputLimit'
require_line "$ROOT/internal/runner/runner.go" 'configureProcessGroup(command)'
require_line "$ROOT/internal/runner/runner.go" 'killProcessGroup(command)'
require_line "$ROOT/cmd/lumonas-privd/main.go" 'var networkCheckpointCommand = exec.Command'
require_line "$ROOT/cmd/lumonas-privd/main.go" 'commandrunner.ConfigureProcessGroup(command)'
require_line "$ROOT/cmd/lumonas-privd/main.go" 'waitProcessGroup(checkpointContext, command)'
require_line "$ROOT/cmd/lumonas-privd/main.go" 'context.WithTimeout(context.Background(), time.Duration(timeout+10)*time.Second)'

# Reject the common unsafe escape hatch in production Go code. Shell command
# strings belong in tests or explicitly reviewed appliance scripts, never in
# service code handling user input.
if grep -R -n --include='*.go' --exclude='*_test.go' -E 'exec\.(Command|CommandContext).*"(sh|bash|zsh)"|/bin/(sh|bash|zsh)' "$ROOT/internal" "$ROOT/cmd" 2>/dev/null; then
	echo "production service code constructs a shell command" >&2
	exit 1
fi

echo "LumoNAS command-boundary policy checks passed"

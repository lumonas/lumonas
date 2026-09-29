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
# There are two explicit exceptions, both for long-lived interactive processes
# that the bounded runner cannot express; keep each one visible and verify that
# its compensating controls are still in place.
#
#  1. cmd/lumonas-privd/main.go            - NetworkManager checkpoint
#  2. internal/virtualization/console_linux.go - libvirt serial console
#
# The second one needs a PTY slave as the child's controlling terminal, which
# requires the child to be a session leader (Setsid + Setctty). That is
# fundamentally incompatible with the runner's Setpgid process-group handling
# and with its output/timeout bounds, so it is confined to a dedicated runtime
# and constrained by the assertions below instead.
matches=$(grep -R -n --include='*.go' --exclude='*_test.go' -F 'exec.Command(' "$ROOT/internal" "$ROOT/cmd" 2>/dev/null || true)
unexpected=$(printf '%s\n' "$matches" | grep -v '/internal/runner/runner.go:' | grep -v '/cmd/lumonas-privd/main.go:' | grep -v '/internal/virtualization/console_linux.go:' || true)
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

# The VM serial console is the second reviewed exception. Assert the controls
# that make a raw exec.Command acceptable here: the domain name is validated,
# the spawned process is a fixed argument vector with no shell, the number of
# concurrent sessions is capped, output is ring-buffered to a fixed bound, and
# idle/finished sessions are reaped and killed.
require_line "$ROOT/internal/virtualization/console_linux.go" 'domainNamePattern.MatchString(domain)'
require_line "$ROOT/internal/virtualization/console_linux.go" 'exec.Command(virsh, "console", domain, "--force")'
require_line "$ROOT/internal/virtualization/console_linux.go" 'len(runtime.sessions) >= consoleSessionLimit'
require_line "$ROOT/internal/virtualization/console_linux.go" 'consoleBufferBytes'
require_line "$ROOT/internal/virtualization/console_linux.go" 'consoleReadBytes'
require_line "$ROOT/internal/virtualization/console_linux.go" 'reapIdleSessions'
require_line "$ROOT/internal/virtualization/console_linux.go" 'session.command.Process.Kill()'

# Reject the common unsafe escape hatch in production Go code. Shell command
# strings belong in tests or explicitly reviewed appliance scripts, never in
# service code handling user input.
#
# Launching a shell from service code is never acceptable and has no exception.
if grep -R -n --include='*.go' --exclude='*_test.go' -E 'exec\.(Command|CommandContext)[^\n]*"(sh|bash|zsh)"' "$ROOT/internal" "$ROOT/cmd" 2>/dev/null; then
	echo "production service code constructs a shell command" >&2
	exit 1
fi

# Writing a shell script is a different operation and has exactly one reviewed
# production site: the certbot deploy hook. Certbot itself executes the file,
# so the broker never spawns a shell. The hook body must stay a constant
# literal that calls back into the broker by absolute path, with no
# interpolation of request or environment data, and must be guarded by the
# renewal lineage it is responsible for.
hook_files=$(grep -R -l --include='*.go' --exclude='*_test.go' -E '/bin/(sh|bash|zsh)' "$ROOT/internal" "$ROOT/cmd" 2>/dev/null || true)
unexpected_hooks=$(printf '%s\n' "$hook_files" | grep -v '^$' | grep -v '/cmd/lumonas-privd/tls_certificate.go$' || true)
if [ -n "$unexpected_hooks" ]; then
	printf '%s\n' "$unexpected_hooks"
	echo "production service code writes a shell script outside the reviewed ACME deploy hook" >&2
	exit 1
fi
require_line "$ROOT/cmd/lumonas-privd/tls_certificate.go" 'func installACMEDeployHook(path string) error {'
require_line "$ROOT/cmd/lumonas-privd/tls_certificate.go" 'RENEWED_LINEAGE:-'
require_line "$ROOT/cmd/lumonas-privd/tls_certificate.go" 'exec /usr/lib/lumonas/lumonas-privd --activate-acme-certificate'
# The hook body must remain a constant: interpolation would turn this into a
# command-construction sink driven by renewal configuration.
if sed -n '/func installACMEDeployHook/,/^}/p' "$ROOT/cmd/lumonas-privd/tls_certificate.go" | grep -E 'Sprintf|\+\s*"|"[[:space:]]*\+' >/dev/null 2>&1; then
	echo "the ACME deploy hook body must be a constant literal" >&2
	exit 1
fi

echo "LumoNAS command-boundary policy checks passed"

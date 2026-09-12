package network

import (
	"context"
	"fmt"

	"github.com/lumonas/lumonas/internal/runner"
)

// DiagnosticCommandRunner is injectable so API tests never execute host
// networking utilities or a shell.
type DiagnosticCommandRunner func(context.Context, string, ...string) ([]byte, error)

// RunCommandDiagnostic executes only the bounded command diagnostics
// supported by the API. Arguments are passed directly to exec.CommandContext;
// no shell interpolation is involved.
func RunCommandDiagnostic(ctx context.Context, kind, target string, _ int, run DiagnosticCommandRunner) (map[string]any, error) {
	if run == nil {
		run = runner.CombinedOutputContext
	}
	command := "ping"
	args := []string{"-c", "1", "-W", "2", target}
	if kind == "traceroute" {
		command = "traceroute"
		args = []string{"-m", "8", "-w", "2", target}
	} else if kind != "ping" {
		return nil, fmt.Errorf("unsupported diagnostic kind")
	}
	output, err := run(ctx, command, args...)
	if err != nil {
		return nil, fmt.Errorf("%s failed", kind)
	}
	if len(output) > 4096 {
		output = output[:4096]
	}
	return map[string]any{"target": target, "output": string(output)}, nil
}

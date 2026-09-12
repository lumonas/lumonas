package network

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunCommandDiagnosticUsesAllowListedArguments(t *testing.T) {
	for _, test := range []struct {
		kind    string
		command string
		args    []string
	}{
		{kind: "ping", command: "ping", args: []string{"-c", "1", "-W", "2", "host;touch /tmp/no"}},
		{kind: "traceroute", command: "traceroute", args: []string{"-m", "8", "-w", "2", "host"}},
	} {
		t.Run(test.kind, func(t *testing.T) {
			var gotCommand string
			var gotArgs []string
			result, err := RunCommandDiagnostic(context.Background(), test.kind, test.args[len(test.args)-1], 0, func(_ context.Context, command string, args ...string) ([]byte, error) {
				gotCommand, gotArgs = command, args
				return []byte("diagnostic output"), nil
			})
			if err != nil || gotCommand != test.command || strings.Join(gotArgs, "\x00") != strings.Join(test.args, "\x00") || result["output"] != "diagnostic output" {
				t.Fatalf("unexpected diagnostic execution: command=%q args=%v result=%#v err=%v", gotCommand, gotArgs, result, err)
			}
		})
	}
}

func TestRunCommandDiagnosticBoundsOutputAndFailures(t *testing.T) {
	result, err := RunCommandDiagnostic(context.Background(), "ping", "host", 0, func(context.Context, string, ...string) ([]byte, error) {
		return []byte(strings.Repeat("x", 5000)), nil
	})
	if err != nil || len(result["output"].(string)) != 4096 {
		t.Fatalf("diagnostic output was not bounded: len=%d err=%v", len(result["output"].(string)), err)
	}
	if _, err := RunCommandDiagnostic(context.Background(), "ping", "host", 0, func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("command failed")
	}); err == nil {
		t.Fatal("failed diagnostic command unexpectedly succeeded")
	}
	if _, err := RunCommandDiagnostic(context.Background(), "shell", "host", 0, nil); err == nil {
		t.Fatal("unsupported diagnostic kind unexpectedly succeeded")
	}
}

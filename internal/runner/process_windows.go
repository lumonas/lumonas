//go:build windows

package runner

import "os/exec"

func configureProcessGroup(command *exec.Cmd) {}

func killProcessGroup(command *exec.Cmd) {
	if command.Process != nil {
		_ = command.Process.Kill()
	}
}
